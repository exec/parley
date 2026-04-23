package main

import (
	"context"
	"crypto/sha256"
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
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/lib/pq"

	"parley/internal/audit"
	"parley/internal/auth"
	"parley/internal/bin"
	"parley/internal/botcommands"
	"parley/internal/bots"
	"parley/internal/cache"
	"parley/internal/channel"
	"parley/internal/db"
	"parley/internal/email"
	"parley/internal/message"
	"parley/internal/passkey"
	"parley/internal/permissions"
	"parley/internal/server"
	"parley/internal/spaces"
	"parley/internal/voice"
	"parley/internal/websocket"
)

// Config holds application configuration
type Config struct {
	DatabaseURL  string
	JWTSecret    string
	Port         string
	OllamaAPIURL string // OLLAMA_API_URL — base URL for Ollama cloud API
	OllamaAPIKey string // OLLAMA_API_KEY — auth key; empty disables AI generation
	OllamaModel  string // OLLAMA_MODEL — model name, e.g. devstral-small-2:24b-cloud
	BotKeySecret []byte // BOT_KEY_SECRET — 32 raw bytes for AES-256 bot API key encryption
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ollamaAPIURL := os.Getenv("OLLAMA_API_URL")
	if ollamaAPIURL == "" {
		ollamaAPIURL = "https://ollama.com/api"
	}

	ollamaAPIKey := os.Getenv("OLLAMA_API_KEY")
	if ollamaAPIKey == "" {
		log.Println("WARNING: OLLAMA_API_KEY is not set — AI theme generation is disabled")
	}

	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "devstral-small-2:24b-cloud"
	}

	botKeySecret := os.Getenv("BOT_KEY_SECRET")
	if botKeySecret == "" {
		log.Fatal("BOT_KEY_SECRET is required (secret used to derive the AES-256 bot API key encryption key)")
	}
	// Derive a 32-byte AES key via SHA-256 so the full entropy of the secret is
	// used regardless of its length, and multi-byte characters are never truncated.
	botKeyHash := sha256.Sum256([]byte(botKeySecret))
	botKeyBytes := botKeyHash[:]

	return &Config{
		DatabaseURL:  databaseURL,
		JWTSecret:    jwtSecret,
		Port:         port,
		OllamaAPIURL: ollamaAPIURL,
		OllamaAPIKey: ollamaAPIKey,
		OllamaModel:  ollamaModel,
		BotKeySecret: botKeyBytes,
	}
}

func main() {
	// Load configuration
	config := DefaultConfig()

	// Connect to PostgreSQL
	dbConn, err := sql.Open("pgx", config.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbConn.Close()

	// Configure the connection pool.
	// API nodes connect via PgBouncer (port 6432, transaction pooling).
	// 25 open conns per node × 3 nodes = 75 PgBouncer client slots; well under
	// max_client_conn=1000. PgBouncer holds default_pool_size=25 backend conns
	// to PostgreSQL, which is configured with max_connections=150.
	dbConn.SetMaxOpenConns(25)
	dbConn.SetMaxIdleConns(5)
	dbConn.SetConnMaxLifetime(5 * time.Minute)
	dbConn.SetConnMaxIdleTime(2 * time.Minute)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := dbConn.PingContext(ctx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("Connected to PostgreSQL database")

	// Initialize repository layer
	repo := db.NewRepository(dbConn)

	// Run migrations via the tracked runner so each migration executes exactly once.
	if err := repo.RunMigrations(ctx); err != nil {
		log.Fatalf("Database migrations failed: %v", err)
	}
	log.Println("Database migrations completed")

	// Initialize email client for verification emails
	brevoAPIKey := os.Getenv("BREVO_API_KEY")
	brevoFromEmail := os.Getenv("BREVO_FROM_EMAIL")
	siteURL := os.Getenv("SITE_URL")
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
	auditSvc := audit.NewAuditService(repo)
	serverService := server.NewServerService(repo, auditSvc)
	channelService := channel.NewChannelService(repo)
	messageService := message.NewMessageService(repo)

	// Initialize WebSocket hub first
	hub := websocket.NewHub()

	// Short-lived membership cache to absorb the wave of CHANNEL_SUBSCRIBE DB
	// queries when many users connect simultaneously (e.g., server restart).
	memberCache := cache.NewMembershipCache(30 * time.Second)

	// Enforce channel access: only server members may subscribe to a channel's events.
	hub.SetChannelAccessChecker(func(userID, channelID string) bool {
		uID, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			return false
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// "server:{serverID}" virtual channels
		if serverIDStr, ok := strings.CutPrefix(channelID, "server:"); ok {
			sID, err := strconv.ParseInt(serverIDStr, 10, 64)
			if err != nil {
				return false
			}
			if isMember, ok := memberCache.GetMember(sID, uID); ok {
				return isMember
			}
			member, err := repo.GetMember(ctx, sID, uID)
			result := err == nil && member != nil
			memberCache.SetMember(sID, uID, result)
			return result
		}

		// "dm:{dmChannelID}" virtual channels
		if dmIDStr, ok := strings.CutPrefix(channelID, "dm:"); ok {
			dmID, err := strconv.ParseInt(dmIDStr, 10, 64)
			if err != nil {
				return false
			}
			ch, err := repo.GetDmChannelByID(ctx, dmID)
			return err == nil && (ch.User1ID == uID || ch.User2ID == uID)
		}

		// Regular channels: check channel→server mapping (cached) then membership (cached)
		chID, err := strconv.ParseInt(channelID, 10, 64)
		if err != nil {
			return false
		}

		var serverID int64
		if sID, ok := memberCache.GetChannelServer(chID); ok {
			serverID = sID
		} else {
			ch, err := repo.GetChannelByID(ctx, chID)
			if err != nil {
				return false
			}
			memberCache.SetChannelServer(chID, ch.ServerID)
			serverID = ch.ServerID
		}

		if isMember, ok := memberCache.GetMember(serverID, uID); ok {
			if !isMember {
				return false
			}
		} else {
			member, err := repo.GetMember(ctx, serverID, uID)
			result := err == nil && member != nil
			memberCache.SetMember(serverID, uID, result)
			if !result {
				return false
			}
		}

		// Verify ViewChannel permission — permission overwrites that deny
		// ViewChannel must be enforced on the real-time path too.
		srv, err := repo.GetServerByID(ctx, serverID)
		if err != nil {
			return false
		}
		canView, err := permissions.HasChannelPermission(ctx, repo, serverID, uID, srv.OwnerID, chID, permissions.PermViewChannel)
		if err != nil || !canView {
			return false
		}
		return true
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

	// Wire StatusWriter unconditionally (required for online/offline persistence)
	hub.SetStatusWriter(repo)

	// Initialize passkey service (requires Redis for session storage)
	var passkeySvc *passkey.Service
	if redisHub != nil {
		passkeySvc = passkey.New(repo, redisHub.Client(), siteURL)
	}

	messageService.SetMemberCache(memberCache)
	serverService.SetMemberCache(memberCache)

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

	// Initialize bots service
	botsRepo := bots.NewRepository(repo)
	botsSvc := bots.NewService(botsRepo, config.BotKeySecret)
	botsHandler := bots.NewHandler(botsSvc)

	// Wire hub into bots handler for SERVER_MEMBER_JOIN/LEAVE broadcasts
	// Must happen after hub is initialized (above) and before requests are served.
	botsHandler.SetHub(hub)

	// Initialize slash-command (bot_commands) service.
	botCmdRepo := botcommands.NewRepository(repo.DB())
	botCmdSvc := botcommands.NewService(botCmdRepo, repo)
	botCmdSvc.SetNotifier(botCommandsHubNotifier{hub: hub})
	botCmdSvc.SetBroadcaster(hubBroadcaster)
	botCommandsHandler := botcommands.NewHandler(botCmdSvc)

	// Background sweeper: transition timed-out pending interactions to 'expired'.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if _, err := botCmdRepo.ExpirePastInteractions(ctx, time.Now().UTC()); err != nil {
				log.Printf("bot interaction expire sweep: %v", err)
			}
			cancel()
		}
	}()

	// Cache bot user ID at startup (fatal if not found — migration must have run)
	botUserID, err := botsRepo.GetBotUserID(context.Background(), "polly")
	if err != nil {
		log.Fatalf("bot user 'polly' not found — run migrations: %v", err)
	}

	// Wire AI dispatch as a message trigger.
	// BuildTrigger is called ONCE here to produce a stable trigger function.
	dispatcher := bots.NewDispatcher(botsSvc, botsRepo, config.OllamaAPIURL, config.OllamaAPIKey, botUserID)
	dispatcher.SetHub(hub)
	postFn := func(ctx context.Context, chID, botUserIDStr, content, replyToMsgID string) error {
		_, err := messageService.SendMessage(ctx, chID, botUserIDStr, content, "", "", "", "", replyToMsgID)
		return err
	}
	botTriggerFn := dispatcher.BuildTrigger(postFn)
	messageService.SetBotTrigger(botTriggerFn)

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
			if corsErr := sc.ConfigureCORS(context.Background(), siteURL); corsErr != nil {
				log.Printf("Warning: failed to configure Spaces CORS: %v", corsErr)
			}
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

	// Register production origin in CORS allowlist from env
	if siteURL != "" {
		allowedOrigins[siteURL] = true
	}

	// Setup chi router
	router := setupRouter(config, repo, authService, serverService, channelService, messageService, hub, spacesClient, voiceSvc, binService, passkeySvc, redisHub, parseCDNHost(spacesCDNURL), siteURL, botsHandler, auditSvc, botCommandsHandler)

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
		// WriteTimeout covers header-to-body write time. WebSocket connections are
		// hijacked before this fires so they are unaffected. Increase to 30s if
		// slow clients serving large paginated histories become an issue.
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
	redisHub *websocket.RedisHub,
	cdnHost string,
	siteURL string,
	botsHandler *bots.Handler,
	auditSvc *audit.AuditService,
	botCommandsHandler *botcommands.Handler,
) *chi.Mux {
	router := chi.NewRouter()

	// Global middleware
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	// CORS middleware
	router.Use(corsMiddleware())

	// Mount routes — use Redis-backed ticket store in production (shared across nodes).
	// Fall back to in-memory only when Redis is unavailable (dev/single-node).
	var tickets ticketIssuer
	if redisHub != nil {
		tickets = newRedisTicketStore(redisHub.Client())
	} else {
		log.Println("WARNING: using in-memory ticket store — WebSocket tickets will NOT work across multiple API nodes")
		tickets = newTicketStore()
	}
	registerRoutes(router, repo, authService, serverService, channelService, messageService, hub, spacesClient, voiceSvc, binService, tickets, passkeySvc, redisHub, config.OllamaAPIURL, config.OllamaAPIKey, config.OllamaModel, cdnHost, siteURL, botsHandler, auditSvc, botCommandsHandler)

	return router
}

// allowedOrigins lists origins permitted to access the API
var allowedOrigins = map[string]bool{
	"http://localhost:5173":  true, // Vite dev server
	"http://localhost:8080":  true, // local API dev
	"tauri://localhost":      true, // Tauri v2 bundled webview (macOS/Linux)
	"http://tauri.localhost": true, // Tauri v2 bundled webview (Windows)
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
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
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

// botCommandsHubNotifier adapts the WebSocket hub to the narrower
// botcommands.Notifier interface. Used to push INTERACTION_CREATE to the bot
// user's connection(s). Hub.SendToUser already cross-publishes to Redis so bots
// connected to a different API node receive the event too.
type botCommandsHubNotifier struct {
	hub *websocket.Hub
}

// PublishToUser marshals data to JSON and sends it to every active connection
// of the target user. Marshal errors are logged and dropped.
func (n botCommandsHubNotifier) PublishToUser(userID int64, event string, data interface{}) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("botcommands: marshal %s payload: %v", event, err)
		return
	}
	if err := n.hub.SendToUser(strconv.FormatInt(userID, 10), event, payload); err != nil {
		log.Printf("botcommands: send %s to user %d: %v", event, userID, err)
	}
}