package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	gorillawebsocket "github.com/gorilla/websocket"

	"parley/internal/auth"
	"parley/internal/channel"
	"parley/internal/db"
	"parley/internal/dm"
	"parley/internal/message"
	"parley/internal/server"
	"parley/internal/spaces"
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
	spacesClient *spaces.Client,
) {
	// Cap request bodies at 64 KB for all routes except /api/upload,
	// which applies its own 25 MB limit inside the handler.
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/upload" {
				r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
			}
			next.ServeHTTP(w, r)
		})
	})

	// Health check endpoint
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Rate limiter for auth endpoints: 10 attempts per IP per minute.
	authLimiter := newRateLimiter(10, time.Minute)

	// Mount API routes
	router.Route("/api", func(r chi.Router) {
		// Auth routes (no auth required)
		r.Route("/auth", func(r chi.Router) {
			r.Use(rateLimitMiddleware(authLimiter))
			r.Post("/register", handleAuthRegister(authService))
			r.Post("/login", handleAuthLogin(authService))
			r.Get("/verify-email", handleVerifyEmail(authService))
		})

		// Protected routes - require authentication
		r.Group(func(r chi.Router) {
			r.Use(auth.AuthMiddlewareWith(authService))
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
			r.Delete("/servers/{id}/leave", serverHandler.LeaveServer)
			r.Post("/servers/{id}/members/{userID}/kick", serverHandler.KickMember)
			r.Post("/servers/{id}/members/{userID}/ban", serverHandler.BanMember)

			// Role routes
			r.Get("/servers/{id}/roles", serverHandler.GetServerRoles)
			r.Post("/servers/{id}/roles", serverHandler.CreateServerRole)
			r.Delete("/servers/{id}/roles/{roleId}", serverHandler.DeleteServerRole)
			r.Patch("/servers/{id}/roles/{roleId}", serverHandler.UpdateServerRole)
			r.Get("/servers/{id}/members/{userID}/roles", serverHandler.GetMemberRoles)
			r.Post("/servers/{id}/members/{userID}/roles", serverHandler.AssignRoleToMember)
			r.Delete("/servers/{id}/members/{userID}/roles/{roleId}", serverHandler.RemoveRoleFromMember)
			r.Get("/servers/{id}/members-with-roles", serverHandler.GetMembersWithRoles)
			r.Get("/servers/{id}/my-permissions", serverHandler.GetMyPermissions)

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
			r.Post("/messages/{id}/reactions", messageHandler.ToggleReaction)

			// DM routes
			dmHandler := dm.NewHandler(repo, hub)
			r.Get("/dms", dmHandler.GetDmChannels)
			r.Post("/dms", dmHandler.OpenDmChannel)
			r.Get("/dms/{id}/messages", dmHandler.GetDmMessages)
			r.Post("/dms/{id}/messages", dmHandler.SendDmMessage)

			// Invite routes
			r.Post("/servers/{id}/invites", serverHandler.CreateInvite)
			r.Get("/invites/{code}", serverHandler.GetInvite)
			r.Put("/servers/{id}/vanity", serverHandler.SetVanityURL)

			// User routes
			r.Get("/users/search", handleUserSearch(repo))
			r.Get("/users/{id}", handleGetUser(repo))
			r.Get("/auth/me", handleGetMe(repo))
			r.Post("/auth/impersonate-token", handleImpersonateToken(authService))
			r.Put("/auth/profile", handleUpdateProfile(authService, repo, hub))
			r.Put("/auth/email", handleChangeEmail(authService))
			r.Post("/auth/resend-verification", handleResendVerification(authService))
			r.Post("/auth/verify-phone", handleVerifyPhone(authService))
			r.Post("/auth/resend-phone", handleResendPhone(authService))
			r.Put("/auth/phone", handleChangePhone(authService))

			// File upload endpoint - 25MB limit (overrides global 64KB cap)
			r.With(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					r.Body = http.MaxBytesReader(w, r.Body, 25*1024*1024)
					next.ServeHTTP(w, r)
				})
			}).Post("/upload", func(w http.ResponseWriter, r *http.Request) {
				if spacesClient == nil {
					http.Error(w, "file upload not configured", http.StatusServiceUnavailable)
					return
				}

				if err := r.ParseMultipartForm(10 << 20); err != nil {
					http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
					return
				}

				file, header, err := r.FormFile("file")
				if err != nil {
					http.Error(w, "missing file field", http.StatusBadRequest)
					return
				}
				defer file.Close()

				// Buffer file so we can both NSFW-check and upload it
				data, err := io.ReadAll(file)
				if err != nil {
					http.Error(w, "failed to read file", http.StatusInternalServerError)
					return
				}

				// NSFW check disabled — sidecar moved to dedicated box (TODO)
				// contentType := header.Header.Get("Content-Type")
				// if strings.HasPrefix(contentType, "image/") { checkNSFW(...) }

				ext := filepath.Ext(header.Filename)
				key := fmt.Sprintf("uploads/%s%s", generateID(), ext)

				url, err := spacesClient.Upload(r.Context(), key, bytes.NewReader(data), int64(len(data)))
				if err != nil {
					log.Printf("upload error: %v", err)
					http.Error(w, "upload failed", http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"url": url})
			})
		})
	})

	// WebSocket route - accepts token via query param (browser WS can't set headers)
	router.Get("/ws", handleWebSocket(hub))
}

func handleImpersonateToken(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetUserID := r.Header.Get("X-Admin-Impersonate")
		adminSecret := r.Header.Get("X-Admin-Secret")

		expectedSecret := os.Getenv("ADMIN_IMPERSONATE_SECRET")
		if expectedSecret == "" || adminSecret != expectedSecret {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		token, err := authService.GenerateImpersonationToken(targetUserID)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"token": token})
	}
}

// generateID returns a unique string ID based on the current time in nanoseconds.
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// checkNSFW sends an image to the local NSFW sidecar and returns true if it should be blocked.
// Fails open (returns false) if the sidecar is unavailable, so uploads are never hard-blocked by infra issues.
func checkNSFW(ctx context.Context, data []byte, _ string) (bool, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "upload")
	if err != nil {
		return false, err
	}
	if _, err := part.Write(data); err != nil {
		return false, err
	}
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:8081/check", body)
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err // sidecar down — fail open
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("nsfw sidecar returned %d", resp.StatusCode)
	}

	var result struct {
		Predictions []struct {
			ClassName   string  `json:"className"`
			Probability float64 `json:"probability"`
		} `json:"predictions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	for _, p := range result.Predictions {
		if (p.ClassName == "Porn" || p.ClassName == "Hentai") && p.Probability > 0.6 {
			return true, nil
		}
	}
	return false, nil
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
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

type AuthResponse struct {
	User  auth.User `json:"user"`
	Token string    `json:"token"`
}

func jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"message": message})
}

func handleAuthRegister(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		user, token, err := authService.Register(r.Context(), req.Username, req.Email, req.Phone, req.Password)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
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
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		emailOrPhone := req.Email
		if emailOrPhone == "" {
			emailOrPhone = req.Phone
		}
		user, token, err := authService.Login(r.Context(), emailOrPhone, req.Password)
		if err != nil {
			jsonError(w, err.Error(), http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{User: user, Token: token})
	}
}

func handleGetMe(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		var id int64
		fmt.Sscan(userIDStr, &id)
		user, err := repo.GetUserByID(r.Context(), id)
		if err != nil {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(auth.User{
			ID:            fmt.Sprintf("%d", user.ID),
			Username:      user.Username,
			Email:         user.Email,
			AvatarURL:     user.AvatarURL,
			BannerURL:     user.BannerURL,
			Bio:           user.Bio,
			EmailVerified: user.EmailVerified,
			PhoneNumber:   user.PhoneNumber,
			PhoneVerified: user.PhoneVerified,
		})
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
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true // native clients / same-origin requests have no Origin
				}
				return allowedOrigins[origin]
			},
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

// publicUserResponse is a version of PublicUser with string IDs for frontend compatibility
type publicUserResponse struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	CreatedAt string `json:"created_at"`
}

func toPublicUserResponse(u db.PublicUser) publicUserResponse {
	return publicUserResponse{
		ID:        strconv.FormatInt(u.ID, 10),
		Username:  u.Username,
		AvatarURL: u.AvatarURL,
		CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// User search handler - GET /api/users/search?q=<query>
func handleUserSearch(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "user not authenticated", http.StatusUnauthorized)
			return
		}

		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			jsonError(w, "invalid user ID", http.StatusBadRequest)
			return
		}

		query := r.URL.Query().Get("q")
		if query == "" {
			jsonError(w, "query parameter 'q' is required", http.StatusBadRequest)
			return
		}

		users, err := repo.SearchUsers(r.Context(), query, userID)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		result := make([]publicUserResponse, len(users))
		for i, u := range users {
			result[i] = toPublicUserResponse(u)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// Get user handler - GET /api/users/{id}
func handleGetUser(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := chi.URLParam(r, "id")
		if userIDStr == "" {
			jsonError(w, "user ID is required", http.StatusBadRequest)
			return
		}

		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			jsonError(w, "invalid user ID", http.StatusBadRequest)
			return
		}

		user, err := repo.GetPublicUser(r.Context(), userID)
		if err != nil {
			if err == db.ErrNotFound {
				jsonError(w, "user not found", http.StatusNotFound)
				return
			}
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(toPublicUserResponse(*user))
	}
}

// Update profile handler - PUT /api/auth/profile
func handleUpdateProfile(authService *auth.AuthService, repo *db.Repository, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "user not authenticated", http.StatusUnauthorized)
			return
		}

		var req struct {
			Username        string `json:"username"`
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
			AvatarURL       string `json:"avatar_url"`
			BannerURL       string `json:"banner_url"`
			Bio             string `json:"bio"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		user, err := authService.UpdateProfile(r.Context(), userIDStr, req.Username, req.CurrentPassword, req.NewPassword, req.AvatarURL, req.BannerURL, req.Bio)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Broadcast USER_UPDATE to all servers the user is a member of
		if hub != nil {
			userID, parseErr := strconv.ParseInt(userIDStr, 10, 64)
			if parseErr == nil {
				servers, serversErr := repo.GetServersByUserID(r.Context(), userID)
				if serversErr == nil {
					payload, marshalErr := json.Marshal(map[string]string{
						"user_id":    userIDStr,
						"username":   user.Username,
						"avatar_url": user.AvatarURL,
					})
					if marshalErr == nil {
						for _, srv := range servers {
							hub.BroadcastToChannel(fmt.Sprintf("server:%d", srv.ID), ws.EventUserUpdate, payload)
						}
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}

// Verify email handler - GET /api/auth/verify-email?token=xxx
func handleVerifyEmail(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			jsonError(w, "token is required", http.StatusBadRequest)
			return
		}

		if err := authService.VerifyEmail(r.Context(), token); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Email verified successfully"})
	}
}

// Change email handler - PUT /api/auth/email
func handleChangeEmail(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "user not authenticated", http.StatusUnauthorized)
			return
		}

		var req struct {
			NewEmail string `json:"new_email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		user, err := authService.ChangeEmail(r.Context(), userIDStr, req.NewEmail, req.Password)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}

// Resend verification handler - POST /api/auth/resend-verification
func handleResendVerification(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "user not authenticated", http.StatusUnauthorized)
			return
		}

		if err := authService.ResendVerification(r.Context(), userIDStr); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Verification email sent"})
	}
}

func handleVerifyPhone(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "user not authenticated", http.StatusUnauthorized)
			return
		}
		var req struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
			jsonError(w, "code is required", http.StatusBadRequest)
			return
		}
		if err := authService.VerifyPhone(r.Context(), userIDStr, req.Code); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Phone verified successfully"})
	}
}

func handleResendPhone(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "user not authenticated", http.StatusUnauthorized)
			return
		}
		if err := authService.SendPhoneVerification(r.Context(), userIDStr); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Verification code sent"})
	}
}

func handleChangePhone(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "user not authenticated", http.StatusUnauthorized)
			return
		}
		var req struct {
			Phone    string `json:"phone"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		user, err := authService.ChangePhone(r.Context(), userIDStr, req.Phone, req.Password)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}
