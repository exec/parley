package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	goredis "github.com/redis/go-redis/v9"

	"parley/internal/ai"
	"parley/internal/audit"
	"parley/internal/auth"
	"parley/internal/bin"
	"parley/internal/botcommands"
	"parley/internal/bots"
	"parley/internal/channel"
	"parley/internal/db"
	"parley/internal/desktopauth"
	"parley/internal/dm"
	"parley/internal/friend"
	"parley/internal/message"
	"parley/internal/notification"
	"parley/internal/passkey"
	"parley/internal/server"
	"parley/internal/soundboard"
	"parley/internal/spaces"
	"parley/internal/theme"
	"parley/internal/voice"
	ws "parley/internal/websocket"
)

// parseCDNHost extracts the hostname from a CDN URL string.
func parseCDNHost(cdnURL string) string {
	u, err := url.Parse(cdnURL)
	if err != nil || u.Host == "" {
		return cdnURL
	}
	return u.Host
}

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
	tickets ticketIssuer,
	passkeySvc *passkey.Service,
	redisHub     *ws.RedisHub,
	ollamaAPIURL string,
	ollamaAPIKey string,
	ollamaModel  string,
	cdnHost string,
	siteURL string,
	botsHandler *bots.Handler,
	auditSvc *audit.AuditService,
	botCommandsHandler *botcommands.Handler,
) {
	// Cap request bodies at 64 KB for all routes except /api/upload,
	// which applies its own 50 MB limit inside the handler.
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			isSoundboardUpload := r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/soundboard")
			if r.URL.Path != "/api/upload" && r.URL.Path != "/api/me/themes/generate" && !isSoundboardUpload {
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

	// Resolve the optional Redis client for shared cross-node rate limiting.
	var rdb *goredis.Client
	if redisHub != nil {
		rdb = redisHub.Client()
	}

	// Rate limiter for auth endpoints: 10 attempts per IP per minute.
	// Uses Redis in production so the limit is enforced across all API nodes.
	authLimiter := newRateLimiterFor(rdb, 10, time.Minute)
	// Rate limiter for invite code lookups: 30 per IP per minute.
	inviteLimiter := newRateLimiterFor(rdb, 30, time.Minute)
	// Rate limiter for message history reads: 120 per IP per minute.
	msgReadLimiter := newRateLimiterFor(rdb, 120, time.Minute)
	// Rate limiter for message writes: 5 messages/second per authenticated user (burst 10).
	// Keyed on user ID, not IP, to prevent cross-IP bypasses by the same account.
	msgWriteLimiter := newRateLimiterFor(rdb, 10, 2*time.Second)
	// Rate limiter for public discovery endpoints: 30 per IP per minute.
	discoverLimiter := newRateLimiterFor(rdb, 30, time.Minute)
	// Rate limiter for message search: 20 per authenticated user per minute.
	// Search uses ILIKE sequential scans — expensive without a full-text index.
	msgSearchLimiter := newRateLimiterFor(rdb, 20, time.Minute)
	// Rate limiter for bot slash-command registration (PUT/POST/DELETE): 50 per
	// authenticated bot per hour. Keyed on the authenticated user ID (bot user)
	// via userRateLimitMiddleware.
	botCmdRegLimiter := newRateLimiterFor(rdb, 50, time.Hour)

	router.Route("/api", func(r chi.Router) {
		// Auth routes
		r.Route("/auth", func(r chi.Router) {
			// Unauthenticated auth routes (rate-limited)
			r.Group(func(r chi.Router) {
				r.Use(rateLimitMiddleware(authLimiter))
				r.Post("/register", handleAuthRegister(authService))
				r.Post("/login", handleAuthLogin(authService))
				r.Get("/verify-email", handleVerifyEmail(authService))
				// Pre-registration invite-code probe. Cheap lookup gated by the
				// same auth limiter; we don't want unauthenticated users to be
				// able to enumerate codes.
				r.Get("/check-invite", handleCheckInvite(repo))
				r.Post("/forgot-password", handleForgotPassword(authService))
				r.Post("/reset-password", handleResetPassword(authService))
				if passkeySvc != nil {
					r.Post("/passkey/login/begin", handlePasskeyLoginBegin(passkeySvc))
					r.Post("/passkey/login/finish", handlePasskeyLoginFinish(passkeySvc, authService))
				}
				if redisHub != nil {
					r.Post("/desktop/exchange", handleDesktopAuthExchange(desktopauth.New(redisHub.Client()), authService))
				}
			})

			// Authenticated auth routes
			r.Group(func(r chi.Router) {
				r.Use(auth.AuthMiddlewareWith(authService))
				r.Get("/me", handleGetMe(repo))
				r.Get("/me/phone", handleGetMePhone(repo))
				r.Post("/ws-ticket", handleWsTicket(authService, tickets))
				r.Put("/profile", handleUpdateProfile(authService, repo, hub, cdnHost))
				r.Put("/email", handleChangeEmail(authService))
				r.Post("/resend-verification", handleResendVerification(authService))
				r.Post("/verify-phone", handleVerifyPhone(authService))
				r.Post("/resend-phone", handleResendPhone(authService))
				r.Put("/phone", handleChangePhone(authService))
				if passkeySvc != nil {
					r.Post("/passkey/register/begin", handlePasskeyRegisterBegin(passkeySvc))
					r.Post("/passkey/register/finish", handlePasskeyRegisterFinish(passkeySvc))
					r.Get("/passkeys", handleListPasskeys(passkeySvc))
					r.Delete("/passkeys/{id}", handleDeletePasskey(passkeySvc))
					r.Put("/passkeys/{id}", handleRenamePasskey(passkeySvc))
				r.Delete("/password", handleRemovePassword(passkeySvc, authService))
				}
				if redisHub != nil {
					r.Post("/desktop/issue", handleDesktopAuthIssue(desktopauth.New(redisHub.Client())))
				}
			})
		})

		// Theme handler — constructed once, used by both protected and public routes
		themeRepo := theme.NewRepository(repo.DB())
		themeSvc := theme.NewService(themeRepo, cdnHost, siteURL)
		themeHandler := theme.NewHandler(themeSvc)

		// AI theme generation handler — requires Redis and a configured Ollama key.
		var aiQueue *ai.AIQueue
		if redisHub != nil && ollamaAPIKey != "" {
			aiQueue = ai.NewAIQueue(redisHub.Client())
			ollamaClient := ai.NewOllamaClient(ollamaAPIURL, ollamaAPIKey, ollamaModel)
			go ai.StartWorker(context.Background(), aiQueue, ollamaClient)
		}
		aiHandler := theme.NewAIHandler(aiQueue)

		// Server handler — used by both authenticated and public routes
		serverHandler := server.NewHandler(serverService, auditSvc)

		// Protected routes — require authentication
		r.Group(func(r chi.Router) {
			r.Use(auth.AuthMiddlewareWith(authService))

			// Server routes
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
			r.Get("/servers/{id}/bans", serverHandler.ListBans)
			r.Delete("/servers/{id}/bans/{userID}", serverHandler.UnbanMember)

			// Role routes
			r.Get("/servers/{id}/roles", serverHandler.GetServerRoles)
			r.Post("/servers/{id}/roles", serverHandler.CreateServerRole)
			r.Delete("/servers/{id}/roles/{roleID}", serverHandler.DeleteServerRole)
			r.Patch("/servers/{id}/roles/positions", serverHandler.ReorderServerRoles)
			r.Patch("/servers/{id}/roles/{roleID}", serverHandler.UpdateServerRole)
			r.Get("/servers/{id}/members/{userID}/roles", serverHandler.GetMemberRoles)
			r.Post("/servers/{id}/members/{userID}/roles", serverHandler.AssignRoleToMember)
			r.Delete("/servers/{id}/members/{userID}/roles/{roleID}", serverHandler.RemoveRoleFromMember)
			r.Get("/servers/{id}/members-with-roles", serverHandler.GetMembersWithRoles)
			r.Get("/servers/{id}/my-permissions", serverHandler.GetMyPermissions)

			// Channel routes
			channelHandler := channel.NewHandler(channelService, auditSvc)
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
			messageHandler := message.NewHandler(messageService, cdnHost)
			r.With(userRateLimitMiddleware(msgSearchLimiter)).Get("/servers/{id}/messages/search", messageHandler.SearchMessages)
			r.With(rateLimitMiddleware(msgReadLimiter)).Get("/channels/{channelID}/messages", messageHandler.GetChannelMessages)
			r.With(userRateLimitMiddleware(msgWriteLimiter)).Post("/channels/{channelID}/messages", messageHandler.SendMessage)
			r.Put("/messages/{id}", messageHandler.EditMessage)
			r.Delete("/messages/{id}", messageHandler.DeleteMessage)
			r.Post("/messages/{id}/reactions", messageHandler.ToggleReaction)
			r.Get("/messages/{id}/versions", messageHandler.GetMessageVersions)
			r.Get("/channels/{channelID}/pins", messageHandler.GetChannelPins)
			r.Post("/channels/{channelID}/pins/{messageID}", messageHandler.PinMessage)
			r.Delete("/channels/{channelID}/pins/{messageID}", messageHandler.UnpinMessage)
			r.With(userRateLimitMiddleware(msgWriteLimiter)).Post("/channels/{channelID}/forward", messageHandler.ForwardToChannel)

			// Typing indicator
			r.Post("/channels/{channelId}/typing", handleChannelTyping(repo, hub))

			// Voice routes
			voiceHandler := voice.NewHandler(voiceSvc, repo, hub)
			r.Get("/channels/{channelId}/voice/token", voiceHandler.Token)
			r.Post("/channels/{channelId}/voice/join", voiceHandler.Join)
			r.Post("/channels/{channelId}/voice/leave", voiceHandler.Leave)
			r.Get("/channels/{channelId}/voice/participants", voiceHandler.Participants)
			r.Post("/channels/{channelId}/voice/participants/{targetUserId}/mute", voiceHandler.MuteParticipant)
			r.Post("/channels/{channelId}/voice/participants/{targetUserId}/kick", voiceHandler.KickParticipant)

			// Notification service — wired into DM, friend, and message flows
			notifSvc := notification.New(repo, hub)
			messageService.SetMentionNotify(notifSvc.NotifyMentions)
			notifHandler := notification.NewHandler(repo)
			r.Get("/notifications", notifHandler.GetNotifications)
			r.Patch("/notifications/read-all", notifHandler.MarkAllRead)
			r.Patch("/notifications/{id}/read", notifHandler.MarkRead)

			// DM routes
			dmHandler := dm.NewHandler(repo, hub)
			dmHandler.SetDmNotify(notifSvc.NotifyDM)
			r.Get("/dms", dmHandler.GetDmChannels)
			r.Post("/dms", dmHandler.OpenDmChannel)
			r.Get("/dms/{id}/messages", dmHandler.GetDmMessages)
			r.Post("/dms/{id}/messages", dmHandler.SendDmMessage)
			r.Delete("/dms/{id}/messages/{messageId}", dmHandler.DeleteDmMessage)
			r.Post("/dms/{id}/messages/{messageId}/reactions", dmHandler.ToggleDmReaction)
			r.With(userRateLimitMiddleware(msgWriteLimiter)).Post("/dms/{id}/forward", dmHandler.ForwardToDm)

			// Friend routes
			friendSvc := friend.NewService(repo, hub)
			friendSvc.SetNotifyFriendRequest(notifSvc.NotifyFriendRequest)
			friendSvc.SetNotifyFriendAccept(notifSvc.NotifyFriendAccept)
			friendHandler := friend.NewHandler(friendSvc)
			r.Get("/friends", friendHandler.GetFriends)
			r.Get("/friend-requests", friendHandler.GetRequests)
			r.Post("/friend-requests", friendHandler.SendRequest)
			r.Post("/friend-requests/{id}/accept", friendHandler.AcceptRequest)
			r.Delete("/friend-requests/{id}", friendHandler.DeclineOrCancel)
			r.Delete("/friends/{userId}", friendHandler.RemoveFriend)

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

			// Soundboard routes
			sbRepo := soundboard.NewRepository(repo.DB())
			sbSvc := soundboard.NewService(sbRepo, spacesClient)
			soundboardHandler := soundboard.NewHandler(sbRepo, sbSvc, repo, hub, voiceSvc)
			r.Get("/soundboard", soundboardHandler.ListAll)
			r.Get("/servers/{serverId}/soundboard", soundboardHandler.List)
			r.With(maxBodyMiddleware(1<<20 + 4096)).Post("/servers/{serverId}/soundboard", soundboardHandler.Upload)
			r.Patch("/servers/{serverId}/soundboard/{soundId}", soundboardHandler.UpdateSound)
			r.Delete("/servers/{serverId}/soundboard/{soundId}", soundboardHandler.DeleteSound)
			r.Post("/channels/{channelId}/soundboard/play", soundboardHandler.Play)

			// Invite routes
			r.Post("/servers/{id}/invites", serverHandler.CreateInvite)
			r.Get("/servers/{id}/invites", serverHandler.ListServerInvites)
			r.Delete("/servers/{id}/invites/{code}", serverHandler.RevokeInvite)
			r.Get("/servers/{id}/invites/{code}/members", serverHandler.GetInviteMembers)
			// GET previews invite info without joining; POST actually joins.
			r.With(rateLimitMiddleware(inviteLimiter)).Get("/invites/{code}", serverHandler.PreviewInvite)
			r.With(rateLimitMiddleware(inviteLimiter)).Post("/invites/{code}", serverHandler.JoinInvite)
			r.Get("/servers/{id}/audit-log", serverHandler.GetAuditLog)
			r.Put("/servers/{id}/vanity", serverHandler.SetVanityURL)
			r.Put("/servers/{id}/categories", serverHandler.SetServerCategories)
			r.Get("/servers/{id}/categories", serverHandler.GetServerCategoriesForServer)

			// Bot routes
			r.Get("/servers/{id}/bots", botsHandler.ListBots)
			r.Post("/servers/{id}/bots", botsHandler.AddBot)
			r.Delete("/servers/{id}/bots/{botId}", botsHandler.RemoveBot)
			r.Get("/servers/{id}/ai-config", botsHandler.GetAIConfig)
			r.Put("/servers/{id}/ai-config", botsHandler.SetAIConfig)
			r.Get("/servers/{id}/ai-usage", botsHandler.GetAIUsage)

			// Slash commands — bot-authenticated (api_key via plk_ prefix hits
			// the same AuthMiddlewareWith path). Writes (PUT/POST/DELETE) are
			// rate-limited to 50/hour per authenticated bot user.
			r.Get("/bots/@me/servers/{id}/commands", botCommandsHandler.ListMyCommands)
			r.With(userRateLimitMiddleware(botCmdRegLimiter)).Put("/bots/@me/servers/{id}/commands", botCommandsHandler.BulkReplaceMyCommands)
			r.With(userRateLimitMiddleware(botCmdRegLimiter)).Post("/bots/@me/servers/{id}/commands", botCommandsHandler.UpsertMyCommand)
			r.With(userRateLimitMiddleware(botCmdRegLimiter)).Delete("/bots/@me/servers/{id}/commands/{name}", botCommandsHandler.DeleteMyCommand)

			// Slash commands — user-authenticated list + invoke.
			r.Get("/servers/{id}/commands", botCommandsHandler.ListServerCommands)
			r.With(userRateLimitMiddleware(msgWriteLimiter)).Post("/channels/{channelID}/interactions", botCommandsHandler.Invoke)

			// Authenticated bot invite accept
			r.Post("/bots/invite/{token}/accept", botsHandler.AcceptInvite)

			// Bots owned by the current user
			r.Get("/bots/mine", botsHandler.GetMyBots)

			// Registration invite routes — users manage their own codes.
			r.Get("/invites", handleListMyInvites(repo))
			r.Post("/invites", handleCreateMyInvite(repo))

			// User routes
			r.Get("/users/search", handleUserSearch(repo))
			r.Get("/users/me", handleGetMeSelf(repo))
			r.Patch("/users/me", handlePatchMe(repo, hub, cdnHost))
			r.Get("/users/{id}", handleGetUser(repo))
			r.Patch("/users/@me/status", handleUpdateStatus(hub, repo))

			// Developer API key routes
			r.Get("/developer/keys", handleListAPIKeys(repo))
			r.Post("/developer/keys", handleCreateAPIKey(repo))
			r.Delete("/developer/keys/{id}", handleRevokeAPIKey(repo))
			r.Patch("/developer/bots/{botId}", handleRenameBotUser(repo))
			r.Patch("/developer/bots/{botId}/invite", botsHandler.UpdateInvitePermissions)

			// GIPHY proxy — keeps the API key server-side
			r.Get("/giphy/search", handleGiphySearch)
			r.Get("/giphy/trending", handleGiphyTrending)

			// File upload — 50 MB limit
			r.With(maxBodyMiddleware(50 * 1024 * 1024)).Post("/upload", handleUpload(spacesClient, repo.DB()))

			// Theme routes
			r.Get("/me/preferences", themeHandler.GetPreferences)
			r.Put("/me/preferences/theme", themeHandler.SetActiveTheme)
			r.Post("/me/themes", themeHandler.CreateTheme)
			r.Put("/me/themes/{id}", themeHandler.UpdateTheme)
			r.Delete("/me/themes/{id}", themeHandler.DeleteTheme)
			r.Post("/me/themes/{id}/share", themeHandler.ShareTheme)
			r.Post("/me/themes/{id}/publish", themeHandler.TogglePublish)
			r.Post("/me/themes/install/{token}", themeHandler.InstallTheme)
			r.Post("/me/themes/generate", aiHandler.Generate)
			r.Put("/themes/{id}/feature", themeHandler.ToggleFeature)
		})

		// Interaction-token auth — bearer credential is the token in the URL,
		// no JWT or API key required.
		r.With(botCommandsHandler.InteractionTokenAuth).Post("/interactions/{token}/respond", botCommandsHandler.Respond)

		// Public theme routes — no authentication required
		r.Get("/themes/repo", themeHandler.GetThemeRepo)
		r.Get("/themes/{token}", themeHandler.GetPublicTheme)

		// Public bot invite route (no auth required)
		r.Get("/bots/invite/{token}", botsHandler.ResolveInvite)

		// Public discovery routes — no authentication required
		r.With(rateLimitMiddleware(discoverLimiter)).Get("/discover", serverHandler.Discover)
		r.With(rateLimitMiddleware(discoverLimiter)).Get("/server-categories", serverHandler.ListServerCategories)
	})

	// WebSocket endpoint — prefers short-lived ticket, falls back to JWT query param
	router.Get("/ws", handleWebSocket(hub, authService, repo, tickets))

	// Bench provisioning endpoints — no-op in prod builds, active in stresstest builds.
	registerBenchRoutes(router, repo, authService)
}

