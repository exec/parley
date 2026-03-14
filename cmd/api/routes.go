package main

import (
	"context"
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
			// Bridge auth.UserIDKey ("userID") to server.UserIDKey (contextKey "user_id")
			r.Use(bridgeUserIDMiddleware)

			// Server routes - registered directly to avoid double-prefix
			serverHandler := server.NewHandler(serverService)
			r.Post("/servers", serverHandler.CreateServer)
			r.Get("/servers", serverHandler.GetUserServers)
			r.Get("/servers/{id}", serverHandler.GetServer)
			r.Put("/servers/{id}", serverHandler.UpdateServer)
			r.Delete("/servers/{id}", serverHandler.DeleteServer)
			r.Post("/servers/{id}/members", serverHandler.AddMember)
			r.Delete("/servers/{id}/members/{userID}", serverHandler.RemoveMember)
			r.Get("/servers/{id}/members", serverHandler.GetMembers)

			// Channel routes
			channelHandler := channel.NewHandler(channelService)
			r.Post("/servers/{serverID}/channels", channelHandler.CreateChannel)
			r.Get("/servers/{serverID}/channels", channelHandler.GetServerChannels)
			r.Get("/channels/{id}", channelHandler.GetChannel)
			r.Put("/channels/{id}", channelHandler.UpdateChannel)
			r.Delete("/channels/{id}", channelHandler.DeleteChannel)

			// Message routes
			messageHandler := message.NewHandler(messageService)
			r.Get("/channels/{channelID}/messages", messageHandler.GetChannelMessages)
			r.Post("/channels/{channelID}/messages", messageHandler.SendMessage)
			r.Put("/messages/{id}", messageHandler.EditMessage)
			r.Delete("/messages/{id}", messageHandler.DeleteMessage)
		})
	})

	// WebSocket route - accepts token via query param (browser WS can't set headers)
	router.Get("/ws", handleWebSocket(hub))
}

// bridgeUserIDMiddleware copies the userID from auth.UserIDKey to server.UserIDKey
// so server handlers can read it with their own context key type.
func bridgeUserIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := auth.GetUserIDFromContext(r)
		if userID != "" {
			ctx := context.WithValue(r.Context(), server.UserIDKey, userID)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
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
	Token string    `json:"token"`
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
		// Accept token from Authorization header OR query param (browser WS can't set headers)
		tokenString := ""
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			tokenString = authHeader[7:]
		} else {
			tokenString = r.URL.Query().Get("token")
		}

		if tokenString == "" {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		authService := auth.NewAuthService(nil)
		userID, err := authService.ValidateToken(tokenString)
		if err != nil {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
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
