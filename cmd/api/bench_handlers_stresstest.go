//go:build stresstest

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // registers pprof routes on http.DefaultServeMux
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"parley/internal/auth"
	"parley/internal/db"
)

func init() {
	log.Println("WARNING: stresstest build — /internal/bench/* routes are active. Never deploy this binary to production.")
}

// registerBenchRoutes registers the stresstest-only provisioning and cleanup endpoints,
// and exposes pprof on /debug/pprof/*.
func registerBenchRoutes(r chi.Router, repo *db.Repository, authService *auth.AuthService) {
	benchSecret := os.Getenv("BENCH_SECRET")
	if benchSecret == "" {
		log.Println("WARNING: BENCH_SECRET is not set — bench endpoints are unprotected")
	}

	r.With(benchSecretMiddleware(benchSecret)).Mount("/debug", http.DefaultServeMux)

	r.Route("/internal/bench", func(r chi.Router) {
		r.Use(benchSecretMiddleware(benchSecret))
		r.Post("/provision", handleBenchProvision(repo, authService))
		r.Delete("/cleanup", handleBenchCleanup(repo))
	})
}

func benchSecretMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret != "" && r.Header.Get("X-Bench-Secret") != secret {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type provisionRequest struct {
	Count  int    `json:"count"`
	Prefix string `json:"prefix"`
}

type provisionedUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Token    string `json:"token"`
}

type provisionResponse struct {
	Users     []provisionedUser `json:"users"`
	ServerID  int64             `json:"server_id"`
	ChannelID int64             `json:"channel_id"`
}

// benchPasswordHash is computed once at startup with MinCost for speed.
// All bench users share the same password "benchtest".
var benchPasswordHash string

func init() {
	h, err := bcrypt.GenerateFromPassword([]byte("benchtest"), bcrypt.MinCost)
	if err != nil {
		log.Fatalf("failed to generate bench password hash: %v", err)
	}
	benchPasswordHash = string(h)
}

func handleBenchProvision(repo *db.Repository, authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req provisionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Count <= 0 || req.Count > 2000 {
			http.Error(w, "count must be 1–2000", http.StatusBadRequest)
			return
		}
		if req.Prefix == "" {
			req.Prefix = "bench_"
		}

		ctx := r.Context()
		users := make([]provisionedUser, 0, req.Count)

		// Create users directly — no email verification, no rate limit, MinCost bcrypt.
		for i := 0; i < req.Count; i++ {
			username := fmt.Sprintf("%s%d_%d", req.Prefix, i, time.Now().UnixNano()%1_000_000)
			email := fmt.Sprintf("%s@bench.invalid", username)

			u := &db.User{
				Username:     username,
				Email:        email,
				PasswordHash: benchPasswordHash,
			}
			if err := repo.CreateUser(ctx, u); err != nil {
				http.Error(w, fmt.Sprintf("create user %d: %v", i, err), http.StatusInternalServerError)
				return
			}
			// Mark email verified so bench users pass any email-verification middleware.
			if err := repo.SetEmailVerified(ctx, u.ID); err != nil {
				http.Error(w, fmt.Sprintf("set email verified for user %d: %v", i, err), http.StatusInternalServerError)
				return
			}
			if err := repo.CreateUserPreferences(ctx, u.ID); err != nil {
				// Non-fatal — preferences are optional
				log.Printf("bench provision: create preferences for user %d: %v", u.ID, err)
			}

			token, err := authService.GenerateTokenForUser(strconv.FormatInt(u.ID, 10))
			if err != nil {
				http.Error(w, fmt.Sprintf("generate token for user %d: %v", i, err), http.StatusInternalServerError)
				return
			}
			users = append(users, provisionedUser{ID: u.ID, Username: username, Token: token})
		}

		// Create a shared test server owned by the first user.
		srv := &db.Server{
			Name:    req.Prefix + "server",
			OwnerID: users[0].ID,
		}
		if err := repo.CreateServer(ctx, srv); err != nil {
			http.Error(w, fmt.Sprintf("create server: %v", err), http.StatusInternalServerError)
			return
		}

		// Create the @everyone role for this server (required for permission checks).
		// Must use CreateEveryoneRole — it sets is_everyone=TRUE and the default permission bits.
		if err := repo.CreateEveryoneRole(ctx, srv.ID); err != nil {
			http.Error(w, fmt.Sprintf("create @everyone role: %v", err), http.StatusInternalServerError)
			return
		}

		// Create a text channel.
		ch := &db.Channel{
			ServerID:    srv.ID,
			Name:        "bench-general",
			ChannelType: db.ChannelTypeText,
			Position:    0,
		}
		if err := repo.CreateChannel(ctx, ch); err != nil {
			http.Error(w, fmt.Sprintf("create channel: %v", err), http.StatusInternalServerError)
			return
		}

		// Add all users as server members.
		for _, u := range users {
			member := &db.ServerMember{
				ServerID: srv.ID,
				UserID:   u.ID,
			}
			if err := repo.AddMember(ctx, member); err != nil {
				// Ignore duplicate member errors.
				log.Printf("bench provision: add member %d: %v", u.ID, err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(provisionResponse{
			Users:     users,
			ServerID:  srv.ID,
			ChannelID: ch.ID,
		})
	}
}

type cleanupRequest struct {
	Prefix string `json:"prefix"`
}

type cleanupResponse struct {
	Deleted int64 `json:"deleted"`
}

func handleBenchCleanup(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req cleanupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Prefix == "" {
			req.Prefix = "bench_"
		}

		ctx := r.Context()
		// DELETE FROM users WHERE username LIKE 'bench_%'
		// All related data (servers, channels, messages, members) cascades via FK ON DELETE CASCADE.
		result, err := repo.DB().ExecContext(ctx,
			`DELETE FROM users WHERE username LIKE $1`,
			req.Prefix+"%",
		)
		if err != nil {
			http.Error(w, fmt.Sprintf("cleanup: %v", err), http.StatusInternalServerError)
			return
		}
		n, _ := result.RowsAffected()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cleanupResponse{Deleted: n})
	}
}
