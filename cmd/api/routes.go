package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"parley/internal/auth"
	"parley/internal/bin"
	"parley/internal/channel"
	"parley/internal/db"
	"parley/internal/dm"
	"parley/internal/message"
	"parley/internal/passkey"
	"parley/internal/server"
	"parley/internal/spaces"
	"parley/internal/voice"
	ws "parley/internal/websocket"
)

// registerRoutes registers all API routes.
func registerRoutes(
	router *chi.Mux,
	repo *db.Repository,
	authService *auth.AuthService,
	serverService *server.ServerService,
	channelService *channel.ChannelService,
	messageService *message.MessageService,
	hub *ws.Hub,
	spacesClient *spaces.Client,
	voiceSvc *voice.Service,
	binService *bin.Service,
	tickets *ticketStore,
	passkeySvc *passkey.Service,
) {
	// Cap request bodies at 64 KB for all routes except /api/upload,
	// which applies its own 50 MB limit inside the handler.
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/upload" {
				r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
			}
			next.ServeHTTP(w, r)
		})
	})

	// Health check
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Rate limiter for auth endpoints: 10 attempts per IP per minute.
	authLimiter := newRateLimiter(10, time.Minute)
	// Rate limiter for invite code lookups: 30 per IP per minute.
	inviteLimiter := newRateLimiter(30, time.Minute)

	router.Route("/api", func(r chi.Router) {
		// Auth routes (no auth required)
		r.Route("/auth", func(r chi.Router) {
			r.Use(rateLimitMiddleware(authLimiter))
			r.Post("/register", handleAuthRegister(authService))
			r.Post("/login", handleAuthLogin(authService))
			r.Get("/verify-email", handleVerifyEmail(authService))
			r.Post("/forgot-password", handleForgotPassword(authService))
			r.Post("/reset-password", handleResetPassword(authService))
			if passkeySvc != nil {
				r.Post("/passkey/login/begin", handlePasskeyLoginBegin(passkeySvc))
				r.Post("/passkey/login/finish", handlePasskeyLoginFinish(passkeySvc, authService))
			}
		})

		// Protected routes — require authentication
		r.Group(func(r chi.Router) {
			r.Use(auth.AuthMiddlewareWith(authService))
			r.Use(bridgeUserIDMiddleware)

			// Server routes
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
			r.Patch("/servers/{serverID}/channels/reorder", channelHandler.ReorderChannels)
			r.Get("/servers/{serverID}/channels", channelHandler.GetServerChannels)
			r.Get("/channels/{id}", channelHandler.GetChannel)
			r.Put("/channels/{id}", channelHandler.UpdateChannel)
			r.Delete("/channels/{id}", channelHandler.DeleteChannel)
			r.Get("/channels/{id}/overwrites", channelHandler.GetOverwrites)
			r.Put("/channels/{id}/overwrites", channelHandler.UpsertOverwrite)
			r.Delete("/channels/{id}/overwrites/{overwriteId}", channelHandler.DeleteOverwrite)
			r.Get("/channels/{id}/my-permissions", channelHandler.GetMyChannelPermissions)

			// Message routes
			messageHandler := message.NewHandler(messageService)
			r.Get("/servers/{id}/messages/search", messageHandler.SearchMessages)
			r.Get("/channels/{channelID}/messages", messageHandler.GetChannelMessages)
			r.Post("/channels/{channelID}/messages", messageHandler.SendMessage)
			r.Put("/messages/{id}", messageHandler.EditMessage)
			r.Delete("/messages/{id}", messageHandler.DeleteMessage)
			r.Post("/messages/{id}/reactions", messageHandler.ToggleReaction)
			r.Get("/messages/{id}/versions", messageHandler.GetMessageVersions)

			// Voice routes
			voiceHandler := voice.NewHandler(voiceSvc, repo, hub)
			r.Get("/channels/{channelId}/voice/token", voiceHandler.Token)
			r.Post("/channels/{channelId}/voice/join", voiceHandler.Join)
			r.Post("/channels/{channelId}/voice/leave", voiceHandler.Leave)
			r.Get("/channels/{channelId}/voice/participants", voiceHandler.Participants)
			r.Post("/channels/{channelId}/voice/participants/{targetUserId}/mute", voiceHandler.MuteParticipant)
			r.Post("/channels/{channelId}/voice/participants/{targetUserId}/kick", voiceHandler.KickParticipant)

			// DM routes
			dmHandler := dm.NewHandler(repo, hub)
			r.Get("/dms", dmHandler.GetDmChannels)
			r.Post("/dms", dmHandler.OpenDmChannel)
			r.Get("/dms/{id}/messages", dmHandler.GetDmMessages)
			r.Post("/dms/{id}/messages", dmHandler.SendDmMessage)

			// Bin routes
			binHandler := bin.NewHandler(binService)
			r.Post("/channels/{channelID}/posts", binHandler.CreatePost)
			r.Get("/channels/{channelID}/posts", binHandler.ListPosts)
			r.Get("/posts/{postID}", binHandler.GetPost)
			r.Put("/posts/{postID}", binHandler.EditPost)
			r.Delete("/posts/{postID}", binHandler.DeletePost)
			r.Get("/posts/{postID}/versions", binHandler.GetVersions)
			r.Get("/posts/{postID}/versions/{versionID}", binHandler.GetVersion)
			r.Post("/posts/{postID}/line-comments", binHandler.CreateLineComment)
			r.Get("/posts/{postID}/line-comments", binHandler.GetLineComments)
			r.Put("/line-comments/{id}", binHandler.UpdateLineComment)
			r.Delete("/line-comments/{id}", binHandler.DeleteLineComment)
			r.Get("/channels/{channelID}/tags", binHandler.GetTags)
			r.Post("/channels/{channelID}/tags", binHandler.CreateTag)
			r.Delete("/channels/{channelID}/tags/{tagID}", binHandler.DeleteTag)

			// Invite routes
			r.Post("/servers/{id}/invites", serverHandler.CreateInvite)
			r.With(rateLimitMiddleware(inviteLimiter)).Get("/invites/{code}", serverHandler.GetInvite)
			r.Put("/servers/{id}/vanity", serverHandler.SetVanityURL)

			// Auth / user routes
			r.Get("/auth/me", handleGetMe(repo))
			r.Get("/auth/me/phone", handleGetMePhone(repo))
			r.Post("/auth/ws-ticket", handleWsTicket(authService, tickets))
			r.Post("/auth/impersonate-token", handleImpersonateToken(authService))
			r.Put("/auth/profile", handleUpdateProfile(authService, repo, hub))
			r.Put("/auth/email", handleChangeEmail(authService))
			r.Post("/auth/resend-verification", handleResendVerification(authService))
			r.Post("/auth/verify-phone", handleVerifyPhone(authService))
			r.Post("/auth/resend-phone", handleResendPhone(authService))
			r.Put("/auth/phone", handleChangePhone(authService))
			r.Get("/users/search", handleUserSearch(repo))
			r.Get("/users/{id}", handleGetUser(repo))

			// Passkey management routes (protected)
			if passkeySvc != nil {
				r.Post("/auth/passkey/register/begin", handlePasskeyRegisterBegin(passkeySvc))
				r.Post("/auth/passkey/register/finish", handlePasskeyRegisterFinish(passkeySvc))
				r.Get("/auth/passkeys", handleListPasskeys(passkeySvc))
				r.Delete("/auth/passkeys/{id}", handleDeletePasskey(passkeySvc))
				r.Put("/auth/passkeys/{id}", handleRenamePasskey(passkeySvc))
			}

			// Developer API key routes
			r.Get("/developer/keys", handleListAPIKeys(repo))
			r.Post("/developer/keys", handleCreateAPIKey(repo))
			r.Delete("/developer/keys/{id}", handleRevokeAPIKey(repo))
			r.Patch("/developer/bots/{botId}", handleRenameBotUser(repo))

			// File upload — 50 MB limit
			r.With(maxBodyMiddleware(50 * 1024 * 1024)).Post("/upload", handleUpload(spacesClient))
		})
	})

	// WebSocket endpoint — prefers short-lived ticket, falls back to JWT query param
	router.Get("/ws", handleWebSocket(hub, authService, repo, tickets))
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
