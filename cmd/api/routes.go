package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	gorillawebsocket "github.com/gorilla/websocket"

	"parley/internal/auth"
	"parley/internal/channel"
	"parley/internal/db"
	"parley/internal/message"
	"parley/internal/server"
	ws "parley/internal/websocket"
)

// registerRoutes registers all API routes
func registerRoutes(
	router *chi.Mux,
	repo *db.Repository,
	authService *auth.AuthService,
	serverService *server.ServerService,
	channelService *channel.ChannelService,
	messageService *message.MessageService,
	hub *ws.Hub,
) {
	// Health check endpoint
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Mount API routes
	router.Route("/api", func(r chi.Router) {
		// Auth routes (no auth required)
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", handleAuthRegister(authService))
			r.Post("/login", handleAuthLogin(authService))
		})

		// Protected routes - require authentication
		r.Group(func(r chi.Router) {
			r.Use(auth.AuthMiddleware)

			// Server routes
			serverHandler := server.NewHandler(serverService)
			r.Mount("/servers", serverHandler.Router())

			// Channel routes
			channelHandler := channel.NewHandler(channelService)
			r.Mount("/channels", channelHandler.ChannelRoutes())

			// Message routes
			messageHandler := message.NewHandler(messageService)
			r.Mount("/messages", messageHandler.Routes())
		})
	})

	// WebSocket route
	router.Get("/ws", handleWebSocket(hub))
}

// Auth handlers

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	User  auth.User `json:"user"`
	Token string     `json:"token"`
}

func handleAuthRegister(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		user, token, err := authService.Register(r.Context(), req.Username, req.Email, req.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(AuthResponse{User: user, Token: token})
	}
}

func handleAuthLogin(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		user, token, err := authService.Login(r.Context(), req.Email, req.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{User: user, Token: token})
	}
}

// WebSocket handler

func handleWebSocket(hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get user ID from query parameter or Authorization header
		userID := r.URL.Query().Get("user_id")
		if userID == "" {
			// Try to get from Authorization header
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) > 7 {
				userID = authHeader[7:] // Remove "Bearer " prefix
			}
		}

		if userID == "" {
			http.Error(w, "user_id is required", http.StatusBadRequest)
			return
		}

		// Upgrade HTTP connection to WebSocket using gorilla/websocket
		upgrader := gorillawebsocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
			return
		}

		// Create client using internal websocket package and register with hub
		wsClient := ws.NewClient(hub, conn, userID)
		hub.RegisterClient(wsClient)

		// Start read and write pumps
		go wsClient.WritePump()
		go wsClient.ReadPump()
	}
}