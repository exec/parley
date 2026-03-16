package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"

	"parley/internal/auth"
	"parley/internal/bin"
	"parley/internal/channel"
	"parley/internal/db"
	"parley/internal/email"
	"parley/internal/message"
	"parley/internal/passkey"
	"parley/internal/server"
	"parley/internal/spaces"
	"parley/internal/voice"
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
		log.Fatal("JWT_SECRET environment variable is not set — refusing to start with an insecure default")
	}

	if os.Getenv("ADMIN_IMPERSONATE_SECRET") == "" {
		log.Println("WARNING: ADMIN_IMPERSONATE_SECRET is not set — the impersonation endpoint is disabled")
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

	// Run migrations individually so a transient error on one doesn't block later ones
	for i, sql := range db.Migrations {
		if _, merr := dbConn.Exec(sql); merr != nil {
			log.Printf("Warning: migration %d may already be applied: %v", i, merr)
		}
	}
	log.Println("Database migrations completed")

	// Initialize repository layer
	repo := db.NewRepository(dbConn)

	// Initialize email client for verification emails
	brevoAPIKey := os.Getenv("BREVO_API_KEY")
	brevoFromEmail := os.Getenv("BREVO_FROM_EMAIL")
	siteURL := os.Getenv("SITE_URL")
	if siteURL == "" {
		siteURL = "https://parley.x86-64.com"
	}
	var emailClient *email.Client
	if brevoAPIKey != "" && brevoFromEmail != "" {
		emailClient = email.NewClient(brevoAPIKey, brevoFromEmail, "Parley")
		log.Println("Email client initialized (Brevo)")
	} else {
		log.Println("Email client not configured (BREVO_API_KEY or BREVO_FROM_EMAIL missing) — email verification disabled")
	}

	// Initialize services
	authService := auth.NewAuthService(repo)
	authService.SetEmailClient(emailClient, siteURL)
	serverService := server.NewServerService(repo)
	channelService := channel.NewChannelService(repo)
	messageService := message.NewMessageService(repo)

	// Initialize WebSocket hub first
	hub := websocket.NewHub()

	// Enforce channel access: only server members may subscribe to a channel's events.
	hub.SetChannelAccessChecker(func(userID, channelID string) bool {
		uID, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			return false
		}
		ctx := context.Background()

		// "server:{serverID}" virtual channels: allow if user is a member of that server.
		if serverIDStr, ok := strings.CutPrefix(channelID, "server:"); ok {
			sID, err := strconv.ParseInt(serverIDStr, 10, 64)
			if err != nil {
				return false
			}
			member, err := repo.GetMember(ctx, sID, uID)
			return err == nil && member != nil
		}

		// Regular channel: check the user is a member of the channel's server.
		chID, err := strconv.ParseInt(channelID, 10, 64)
		if err != nil {
			return false
		}
		ch, err := repo.GetChannelByID(ctx, chID)
		if err != nil {
			return false
		}
		member, err := repo.GetMember(ctx, ch.ServerID, uID)
		return err == nil && member != nil
	})

	// Set up Redis pub/sub for cross-node broadcasting (graceful fallback if unavailable)
	redisHub := websocket.NewRedisHub(hub)
	if redisHub != nil {
		hub.SetPublisher(redisHub)
		redisHub.Subscribe()
		log.Println("Redis pub/sub enabled for cross-node WebSocket broadcasting")
	} else {
		log.Println("Running in local-only WebSocket mode (no Redis)")
	}

	// Initialize passkey service (requires Redis for session storage)
	var passkeySvc *passkey.Service
	if redisHub != nil {
		passkeySvc = passkey.New(repo, redisHub.Client(), siteURL)
	}

	// Set up hub broadcasting for message service
	hubBroadcaster := &HubBroadcaster{hub: hub}
	messageService.SetBroadcaster(hubBroadcaster)

	// Set up hub broadcasting for server service (for member join events)
	serverService.SetHub(hub)

	// Set up hub broadcasting for channel service
	channelService.SetHub(hub)

	// Initialize bin service
	binService := bin.NewService(repo)
	binService.SetHub(hub)

	// Start hub in a goroutine
	go hub.Run()
	log.Println("WebSocket hub started")

	// Initialize Spaces client
	var spacesClient *spaces.Client
	spacesAccessKey := os.Getenv("SPACES_ACCESS_KEY")
	spacesSecretKey := os.Getenv("SPACES_SECRET_KEY")
	spacesBucket    := os.Getenv("SPACES_BUCKET")
	spacesRegion    := os.Getenv("SPACES_REGION")
	spacesEndpoint  := os.Getenv("SPACES_ENDPOINT")
	spacesCDNURL    := os.Getenv("SPACES_CDN_URL")
	if spacesAccessKey != "" && spacesSecretKey != "" {
		sc, err := spaces.NewClient(spacesAccessKey, spacesSecretKey, spacesBucket, spacesRegion, spacesEndpoint, spacesCDNURL)
		if err != nil {
			log.Printf("Warning: failed to init Spaces client: %v", err)
		} else {
			spacesClient = sc
		}
	}

	// Initialize voice service (optional — requires LIVEKIT_* env vars and Redis)
	var voiceSvc *voice.Service
	if redisHub != nil {
		voiceSvc = voice.NewService(redisHub.Client())
	} else {
		voiceSvc = voice.NewService(nil)
	}
	if voiceSvc.Configured() {
		log.Println("LiveKit voice service enabled")
	} else {
		log.Println("LiveKit voice service not configured (LIVEKIT_API_KEY/LIVEKIT_API_SECRET/LIVEKIT_URL not set)")
	}

	// Setup chi router
	router := setupRouter(config, repo, authService, serverService, channelService, messageService, hub, spacesClient, voiceSvc, binService, passkeySvc)

	// Start version purge goroutine
	go func() {
		// Purge on startup
		if n, err := repo.PurgeOldMessageVersions(context.Background()); err != nil {
			log.Printf("version purge error: %v", err)
		} else if n > 0 {
			log.Printf("purged %d old message versions", n)
		}
		if n, err := repo.PurgeOldBinPostVersions(context.Background()); err != nil {
			log.Printf("bin version purge error: %v", err)
		} else if n > 0 {
			log.Printf("purged %d old bin post versions", n)
		}

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if n, err := repo.PurgeOldMessageVersions(context.Background()); err != nil {
				log.Printf("version purge error: %v", err)
			} else if n > 0 {
				log.Printf("purged %d old message versions", n)
			}
			if n, err := repo.PurgeOldBinPostVersions(context.Background()); err != nil {
				log.Printf("bin version purge error: %v", err)
			} else if n > 0 {
				log.Printf("purged %d old bin post versions", n)
			}
		}
	}()

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
	spacesClient *spaces.Client,
	voiceSvc *voice.Service,
	binService *bin.Service,
	passkeySvc *passkey.Service,
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
	tickets := newTicketStore()
	registerRoutes(router, repo, authService, serverService, channelService, messageService, hub, spacesClient, voiceSvc, binService, tickets, passkeySvc)

	return router
}

// allowedOrigins lists origins permitted to access the API
var allowedOrigins = map[string]bool{
	"https://parley.x86-64.com": true,
	"http://localhost:5173":      true, // Vite dev server
	"http://localhost:8080":      true, // local API dev
}

// corsMiddleware returns a CORS middleware handler
func corsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if allowedOrigins[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "3600")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
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