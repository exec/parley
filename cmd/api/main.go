package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"

	"parley/internal/auth"
	"parley/internal/channel"
	"parley/internal/db"
	"parley/internal/message"
	"parley/internal/server"
	"parley/internal/websocket"
)

// Config holds application configuration
type Config struct {
	DatabaseURL string
	JWTSecret   string
	Port        string
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/parley?sslmode=disable"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "parley-secret-key-change-in-production"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return &Config{
		DatabaseURL: databaseURL,
		JWTSecret:   jwtSecret,
		Port:        port,
	}
}

func main() {
	// Load configuration
	config := DefaultConfig()

	// Connect to PostgreSQL
	dbConn, err := sql.Open("postgres", config.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbConn.Close()

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := dbConn.PingContext(ctx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("Connected to PostgreSQL database")

	// Run migrations
	migrationSQL := db.MigrationSQL()
	_, err = dbConn.Exec(migrationSQL)
	if err != nil {
		log.Printf("Warning: Migration error (may already exist): %v", err)
	} else {
		log.Println("Database migrations completed")
	}

	// Initialize repository layer
	repo := db.NewRepository(dbConn)

	// Initialize services
	authService := auth.NewAuthService(repo)
	serverService := server.NewServerService(repo)
	channelService := channel.NewChannelService(repo)
	messageService := message.NewMessageService(repo)

	// Initialize WebSocket hub
	hub := websocket.NewHub()

	// Set up the hub as the broadcaster for messages
	hubBroadcaster := &HubBroadcaster{hub: hub}
	messageService.SetBroadcaster(hubBroadcaster)

	// Start hub in a goroutine
	go hub.Run()
	log.Println("WebSocket hub started")

	// Setup chi router
	router := setupRouter(config, repo, authService, serverService, channelService, messageService, hub)

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting HTTP server on port %s", config.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Gracefully shutdown the server
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited properly")
}

// setupRouter configures the chi router with all routes and middleware
func setupRouter(
	config *Config,
	repo *db.Repository,
	authService *auth.AuthService,
	serverService *server.ServerService,
	channelService *channel.ChannelService,
	messageService *message.MessageService,
	hub *websocket.Hub,
) *chi.Mux {
	router := chi.NewRouter()

	// Global middleware
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	// CORS middleware
	router.Use(corsMiddleware())

	// Mount routes
	registerRoutes(router, repo, authService, serverService, channelService, messageService, hub)

	return router
}

// corsMiddleware returns a CORS middleware handler
func corsMiddleware() func(http.Handler) http.Handler {
	// Use go-chi/cors package
	// Configure CORS settings
	corsHandler := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set CORS headers
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "3600")

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
	return corsHandler
}

// HubBroadcaster implements the message.Broadcaster interface using the WebSocket hub
type HubBroadcaster struct {
	hub *websocket.Hub
}

// BroadcastToChannel sends a message to all subscribers of a channel
func (h *HubBroadcaster) BroadcastToChannel(channelID string, event string, data interface{}) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshaling broadcast message: %v", err)
		return
	}
	h.hub.BroadcastToChannel(channelID, event, payload)
}

// BroadcastToUser sends a message to a specific user
func (h *HubBroadcaster) BroadcastToUser(userID string, event string, data interface{}) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshaling user message: %v", err)
		return
	}
	h.hub.SendToUser(userID, event, payload)
}