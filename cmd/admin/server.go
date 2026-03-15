package main

import (
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

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
			r.Patch("/users/{id}/badges", handleSetBadges)
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
			r.Post("/servers/{id}/invite", handleGenerateInvite)
		})
	})

	// Serve SPA — must be last to avoid swallowing /api routes
	r.Handle("/*", http.FileServer(http.Dir("/var/www/parley-admin")))

	log.Printf("Admin server starting on :%s", port)
	http.ListenAndServe(":"+port, r)
}
