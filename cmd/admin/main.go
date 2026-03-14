package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"parley/internal/db"
)

var repo *db.Repository
var adminJWTSecret string
var parleyJWTSecret string

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: parley-admin <command>")
		fmt.Println("Commands:")
		fmt.Println("  serve              Start the admin HTTP server")
		fmt.Println("  create-user <name> Create a new admin user (prompts for password)")
		fmt.Println("  activate <name>    Activate an admin user")
		fmt.Println("  deactivate <name>  Deactivate an admin user")
		fmt.Println("  list-users         List all admin users")
		os.Exit(1)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	var err error
	repo, err = db.NewRepositoryWithDSN(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := repo.RunMigrations(context.Background()); err != nil {
		log.Fatalf("Migrations failed: %v", err)
	}

	adminJWTSecret = os.Getenv("ADMIN_JWT_SECRET")
	if adminJWTSecret == "" {
		log.Fatal("ADMIN_JWT_SECRET is required")
	}
	parleyJWTSecret = os.Getenv("PARLEY_JWT_SECRET")

	switch os.Args[1] {
	case "serve":
		runServer()
	case "create-user":
		if len(os.Args) < 3 {
			fmt.Println("Usage: parley-admin create-user <username>")
			os.Exit(1)
		}
		cliCreateUser(os.Args[2])
	case "activate":
		if len(os.Args) < 3 {
			fmt.Println("Usage: parley-admin activate <username>")
			os.Exit(1)
		}
		cliSetActive(os.Args[2], true)
	case "deactivate":
		if len(os.Args) < 3 {
			fmt.Println("Usage: parley-admin deactivate <username>")
			os.Exit(1)
		}
		cliSetActive(os.Args[2], false)
	case "list-users":
		cliListUsers()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

// ─── CLI commands ──────────────────────────────────────────────────────────────

func cliCreateUser(username string) {
	fmt.Printf("Password for %s: ", username)
	reader := bufio.NewReader(os.Stdin)
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)
	if password == "" {
		fmt.Println("Password cannot be empty")
		os.Exit(1)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}
	u, err := repo.AdminCreateUser(context.Background(), username, string(hash))
	if err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}
	fmt.Printf("Created admin user '%s' (ID: %d) — inactive. Run 'activate %s' to enable login.\n", u.Username, u.ID, u.Username)
}

func cliSetActive(username string, active bool) {
	if err := repo.AdminSetActive(context.Background(), username, active); err != nil {
		if err == db.ErrNotFound {
			fmt.Printf("Admin user '%s' not found\n", username)
			os.Exit(1)
		}
		log.Fatal(err)
	}
	state := "activated"
	if !active {
		state = "deactivated"
	}
	fmt.Printf("Admin user '%s' %s.\n", username, state)
}

func cliListUsers() {
	users, err := repo.AdminListUsers(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%-5s %-20s %-10s %-25s %-25s\n", "ID", "Username", "Active", "Created", "Last Login")
	fmt.Println(strings.Repeat("-", 90))
	for _, u := range users {
		lastLogin := "never"
		if u.LastLoginAt != nil {
			lastLogin = u.LastLoginAt.Format("2006-01-02 15:04:05")
		}
		activeStr := "no"
		if u.Active {
			activeStr = "YES"
		}
		fmt.Printf("%-5d %-20s %-10s %-25s %-25s\n", u.ID, u.Username, activeStr, u.CreatedAt.Format("2006-01-02 15:04:05"), lastLogin)
	}
}

// ─── HTTP Server ───────────────────────────────────────────────────────────────

func runServer() {
	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		port = "8081"
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
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
		r.Post("/auth/login", handleLogin)

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

			// Servers
			r.Get("/servers", handleListServers)
			r.Delete("/servers/{id}", handleDisbandServer)
		})
	})

	// Serve SPA — must be last to avoid swallowing /api routes
	r.Handle("/*", http.FileServer(http.Dir("/var/www/parley-admin")))

	log.Printf("Admin server starting on :%s", port)
	http.ListenAndServe(":"+port, r)
}

// ─── Auth middleware ───────────────────────────────────────────────────────────

type contextKey string

const adminIDKey contextKey = "adminID"
const adminUsernameKey contextKey = "adminUsername"

func adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return []byte(adminJWTSecret), nil
		})
		if err != nil || !token.Valid {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		adminID := int64(claims["admin_id"].(float64))
		adminUsername := claims["username"].(string)
		ctx := r.Context()
		ctx = context.WithValue(ctx, adminIDKey, adminID)
		ctx = context.WithValue(ctx, adminUsernameKey, adminUsername)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getAdminID(r *http.Request) int64 {
	v, _ := r.Context().Value(adminIDKey).(int64)
	return v
}

// ─── Handlers ──────────────────────────────────────────────────────────────────

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	u, err := repo.AdminGetUser(r.Context(), req.Username)
	if err != nil || !u.Active {
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	repo.AdminUpdateLastLogin(r.Context(), u.ID)

	claims := jwt.MapClaims{
		"admin_id": u.ID,
		"username": u.Username,
		"exp":      time.Now().Add(12 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(adminJWTSecret))
	if err != nil {
		jsonError(w, "token generation failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{
		"token":    tokenStr,
		"username": u.Username,
	})
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := repo.GetAdminStats(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, stats)
}

func handleListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	users, err := repo.AdminSearchUsers(r.Context(), q, limit, offset)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if users == nil {
		users = []db.User{}
	}
	jsonOK(w, users)
}

func handleGetUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	u, err := repo.GetUserByID(r.Context(), id)
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	jsonOK(w, u)
}

func handleBanUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Reason == "" {
		req.Reason = "violation of Terms of Service"
	}
	if err := repo.BanUser(r.Context(), id, req.Reason); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	repo.ForceLogoutUser(r.Context(), id)
	jsonOK(w, map[string]string{"message": "user banned"})
}

func handleUnbanUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := repo.UnbanUser(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"message": "user unbanned"})
}

func handleForceLogout(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := repo.ForceLogoutUser(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"message": "user force logged out"})
}

func handleImpersonate(w http.ResponseWriter, r *http.Request) {
	if parleyJWTSecret == "" {
		jsonError(w, "impersonation not configured", http.StatusNotImplemented)
		return
	}
	userIDStr := chi.URLParam(r, "id")
	claims := jwt.MapClaims{
		"user_id":       userIDStr,
		"impersonation": true,
		"exp":           time.Now().Add(1 * time.Hour).Unix(),
		"iat":           time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(parleyJWTSecret))
	if err != nil {
		jsonError(w, "token generation failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"token": tokenStr})
}

func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := repo.AdminDeleteUser(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"message": "user deleted"})
}

func handleSearchMessages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	userID, _ := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	msgs, err := repo.SearchMessages(r.Context(), q, userID, limit, offset)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []db.Message{}
	}
	jsonOK(w, msgs)
}

func handleMessageContext(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	before := queryInt(r, "before", 10)
	after := queryInt(r, "after", 10)
	msgs, err := repo.GetMessageContext(r.Context(), id, before, after)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []db.Message{}
	}
	jsonOK(w, map[string]interface{}{
		"messages":   msgs,
		"message_id": id,
	})
}

func handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := repo.AdminDeleteMessage(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"message": "deleted"})
}

func handleListReports(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	reports, err := repo.GetReports(r.Context(), status, limit, offset)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if reports == nil {
		reports = []db.Report{}
	}
	jsonOK(w, reports)
}

func handleGetReport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	report, err := repo.GetReport(r.Context(), id)
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	result := map[string]interface{}{"report": report}
	if report.ReportedMessageID != nil {
		msgs, err := repo.GetMessageContext(r.Context(), *report.ReportedMessageID, 10, 10)
		if err == nil {
			result["context"] = msgs
			result["target_message_id"] = *report.ReportedMessageID
		}
	}
	jsonOK(w, result)
}

func handleResolveReport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Status == "" {
		req.Status = "resolved"
	}
	adminID := getAdminID(r)
	if err := repo.ResolveReport(r.Context(), id, adminID, req.Status, req.Note); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"message": "report updated"})
}

func handleListCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := repo.GetReportCategories(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if cats == nil {
		cats = []db.ReportCategory{}
	}
	jsonOK(w, cats)
}

func handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}
	cat, err := repo.CreateReportCategory(r.Context(), req.Name)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, cat)
}

func handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := repo.DeleteReportCategory(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"message": "deleted"})
}

func handleListServers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	servers, err := repo.AdminGetServers(r.Context(), q, limit, offset)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if servers == nil {
		servers = []db.Server{}
	}
	jsonOK(w, servers)
}

func handleDisbandServer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	// Get server name before deletion
	servers, _ := repo.AdminGetServers(r.Context(), "", 100, 0)
	serverName := fmt.Sprintf("server #%d", id)
	for _, s := range servers {
		if s.ID == id {
			serverName = s.Name
			break
		}
	}

	// Get all member user IDs
	memberIDs, err := repo.GetServerMemberUserIDs(r.Context(), id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get system user
	sysUser, err := repo.GetSystemUser(r.Context())
	if err != nil {
		log.Printf("Warning: could not get system user for DMs: %v", err)
	}

	// Delete server (CASCADE removes channels, members, messages)
	if _, err := repo.DB().ExecContext(r.Context(), `DELETE FROM servers WHERE id = $1`, id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send DMs to all former members
	if sysUser != nil {
		msg := fmt.Sprintf("The server **%s** was disbanded due to violations of Parley's Terms of Service. We apologize for any disruption.", serverName)
		for _, memberID := range memberIDs {
			if memberID == sysUser.ID {
				continue
			}
			if err := repo.SendSystemDM(r.Context(), sysUser.ID, memberID, msg); err != nil {
				log.Printf("Warning: failed to send DM to user %d: %v", memberID, err)
			}
		}
	}

	jsonOK(w, map[string]interface{}{
		"message":          "server disbanded",
		"members_notified": len(memberIDs),
	})
}

// ─── Helpers ───────────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func queryInt(r *http.Request, key string, def int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return def
	}
	return v
}
