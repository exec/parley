package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// adminOrigin is the allowed CORS origin for the admin frontend.
// Defaults to the production admin URL but can be overridden via ADMIN_ORIGIN env.
func adminOrigin() string {
	if o := os.Getenv("ADMIN_ORIGIN"); o != "" {
		return o
	}
	return "http://167.71.242.21"
}

// ----- simple IP-based rate limiter (reused from API pattern) -----

type adminRateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func newAdminRateLimiter(limit int, window time.Duration) *adminRateLimiter {
	rl := &adminRateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
	go rl.cleanup()
	return rl
}

func (rl *adminRateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-rl.window)
	prev := rl.requests[key]
	valid := prev[:0]
	for _, t := range prev {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	if len(valid) >= rl.limit {
		rl.requests[key] = valid
		return false
	}
	rl.requests[key] = append(valid, now)
	return true
}

func (rl *adminRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.window)
		for key, times := range rl.requests {
			valid := times[:0]
			for _, t := range times {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.requests, key)
			} else {
				rl.requests[key] = valid
			}
		}
		rl.mu.Unlock()
	}
}

// adminRateLimitMiddleware rejects requests that exceed the limiter's threshold.
// It keys on X-Real-IP when set by the DMZ nginx (which overwrites the header
// to $remote_addr after real_ip_header CF-Connecting-IP, so it reflects the
// trusted real client IP) and falls back to r.RemoteAddr otherwise. Mirrors
// cmd/api/middleware.go rateLimitMiddleware. X-Forwarded-For is never read:
// Cloudflare preserves client-supplied XFF as the leftmost token, making it
// attacker-controlled (see audit finding F6). Without this, post-F1 all
// CF-routed traffic hits r.RemoteAddr=127.0.0.1 and shares one global bucket
// (finding F7).
func adminRateLimitMiddleware(rl *adminRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := strings.TrimSpace(r.Header.Get("X-Real-IP"))
			if ip == "" {
				var err error
				ip, _, err = net.SplitHostPort(r.RemoteAddr)
				if err != nil {
					ip = r.RemoteAddr
				}
			}
			if !rl.Allow(ip) {
				jsonError(w, "too many requests, please slow down", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func runServer() {
	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		port = "8081"
	}

	// Rate limiter: 5 login attempts per IP per minute
	loginLimiter := newAdminRateLimiter(5, time.Minute)

	origin := adminOrigin()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqOrigin := r.Header.Get("Origin")
			if reqOrigin == origin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	// Serve admin frontend static files
	r.Handle("/assets/*", http.FileServer(http.Dir("/var/www/parley-admin")))

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.With(adminRateLimitMiddleware(loginLimiter)).Post("/auth/login", handleLogin)

		r.Group(func(r chi.Router) {
			r.Use(adminAuthMiddleware)

			// Dashboard
			r.Get("/stats", handleStats)

			// Users
			r.Get("/users", handleListUsers)
			r.Get("/users/{id}", handleGetUser)
			r.Post("/users/{id}/ban", handleBanUser)
			r.Post("/users/{id}/unban", handleUnbanUser)
			r.Post("/users/{id}/force-logout", handleForceLogout)
			r.Post("/users/{id}/impersonate", handleImpersonate)
			r.Patch("/users/{id}/badges", handleSetBadges)
			r.Post("/users/{id}/invites", handleAddUserInvites)
			r.Post("/invites/bulk", handleBulkAddInvites)
			r.Delete("/users/{id}", handleDeleteUser)

			// Messages
			r.Get("/messages", handleSearchMessages)
			r.Get("/messages/{id}/context", handleMessageContext)
			r.Delete("/messages/{id}", handleDeleteMessage)

			// Reports
			r.Get("/reports", handleListReports)
			r.Get("/reports/{id}", handleGetReport)
			r.Post("/reports/{id}/resolve", handleResolveReport)

			// Report categories
			r.Get("/categories", handleListCategories)
			r.Post("/categories", handleCreateCategory)
			r.Delete("/categories/{id}", handleDeleteCategory)

			// Server categories
			r.Get("/server-categories", handleListServerCategories)
			r.Post("/server-categories", handleCreateServerCategory)
			r.Delete("/server-categories/{id}", handleDeleteServerCategory)

			// Bots
			r.Get("/bots", handleListBots)

			// Servers
			r.Get("/servers", handleListServers)
			r.Delete("/servers/{id}", handleDisbandServer)
			r.Post("/servers/{id}/invite", handleGenerateInvite)
		})
	})

	// Serve SPA — must be last to avoid swallowing /api routes
	r.Handle("/*", http.FileServer(http.Dir("/var/www/parley-admin")))

	// F1: when ADMIN_BIND_LOCAL=1, bind to 127.0.0.1 only so the admin Go
	// server is not reachable directly from vmbr1. The container-local nginx
	// (which enforces the source-IP allow-list) reverse-proxies /api/ to
	// 127.0.0.1:<ADMIN_PORT>. Default (unset / "0") preserves the legacy
	// all-interfaces bind for dev / non-LXC deployments.
	addr := ":" + port
	if v := os.Getenv("ADMIN_BIND_LOCAL"); v == "1" || v == "true" || v == "yes" {
		addr = "127.0.0.1:" + port
	}
	log.Printf("Admin server starting on %s", addr)
	http.ListenAndServe(addr, r)
}
