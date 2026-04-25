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
	// D4: aggregate per-owner cap — 10 msg-writes/second shared across a user
	// and every bot they own, regardless of bot count. Sized at 2× the
	// per-user burst so a legit owner with a helpful bot isn't penalised,
	// while the previous 1-owner + 10-bots = 55/s attack surface is gone.
	msgWriteOwnerLimiter := newRateLimiterFor(rdb, 20, 2*time.Second)
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
				// D3: the /api/auth/me reads expose the bot's own
				// session state (email, phone, badges). Treat as a
				// profile:write-level surface — a leaked read-only
				// token has no business listing verified contacts.
				r.With(auth.RequireScope(auth.ScopeProfileWrite)).Get("/me", handleGetMe(repo))
				r.With(auth.RequireScope(auth.ScopeProfileWrite)).Get("/me/phone", handleGetMePhone(repo))
				// Support sessions must not acquire WS tickets as the
				// target (would let an admin open a full WS session as
				// the user) nor pair a desktop client to them.
				r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Post("/ws-ticket", handleWsTicket(authService, tickets))
				// Everything that mutates the target's account / credentials
				// / verified contacts / passkeys is denied for impersonation
				// sessions — these are destructive from the user's POV and
				// have no support-workflow justification.
				r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Put("/profile", handleUpdateProfile(authService, repo, hub, cdnHost))
				r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Put("/email", handleChangeEmail(authService))
				r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Post("/resend-verification", handleResendVerification(authService))
				r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Post("/verify-phone", handleVerifyPhone(authService))
				r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Post("/resend-phone", handleResendPhone(authService))
				r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Put("/phone", handleChangePhone(authService))
				if passkeySvc != nil {
					r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Post("/passkey/register/begin", handlePasskeyRegisterBegin(passkeySvc))
					r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Post("/passkey/register/finish", handlePasskeyRegisterFinish(passkeySvc))
					r.With(auth.RequireScope(auth.ScopeProfileWrite)).Get("/passkeys", handleListPasskeys(passkeySvc))
					r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Delete("/passkeys/{id}", handleDeletePasskey(passkeySvc))
					r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Put("/passkeys/{id}", handleRenamePasskey(passkeySvc))
					r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Delete("/password", handleRemovePassword(authService))
				}
				if redisHub != nil {
					r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Post("/desktop/issue", handleDesktopAuthIssue(desktopauth.New(redisHub.Client())))
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

			// D3: per-endpoint scope gates for bot API keys. RequireScope is
			// a pass-through for non-API-key auth (user JWTs + impersonation
			// tokens), so mounting it here has no effect on user sessions.
			// Scope mapping rationale (server + role + channel routes):
			//   servers:read   → every read of server/role/channel metadata,
			//                    including per-channel overwrites and
			//                    "my-permissions" probes. Bots that only
			//                    surface server/channel state never need
			//                    write scopes.
			//   profile:write  → mutations that change the bot's own visible
			//                    state in a server (kick/ban actions target
			//                    other users; these are gated on the bot's
			//                    existing member perms, so the scope bar is
			//                    set at "can modify state" = profile:write —
			//                    kept narrow so a messages-only bot can't
			//                    nuke channels). Treat role mutations the
			//                    same: narrow bots shouldn't reshape
			//                    permissions silently.
			//
			// Server routes
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/servers", serverHandler.CreateServer)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers", serverHandler.GetUserServers)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}", serverHandler.GetServer)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Put("/servers/{id}", serverHandler.UpdateServer)
			r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Delete("/servers/{id}", serverHandler.DeleteServer)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/servers/{id}/members", serverHandler.AddMember)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Delete("/servers/{id}/members/{userID}", serverHandler.RemoveMember)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}/members", serverHandler.GetMembers)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Delete("/servers/{id}/leave", serverHandler.LeaveServer)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/servers/{id}/members/{userID}/kick", serverHandler.KickMember)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/servers/{id}/members/{userID}/ban", serverHandler.BanMember)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}/bans", serverHandler.ListBans)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Delete("/servers/{id}/bans/{userID}", serverHandler.UnbanMember)

			// Role routes
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}/roles", serverHandler.GetServerRoles)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/servers/{id}/roles", serverHandler.CreateServerRole)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Delete("/servers/{id}/roles/{roleID}", serverHandler.DeleteServerRole)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Patch("/servers/{id}/roles/positions", serverHandler.ReorderServerRoles)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Patch("/servers/{id}/roles/{roleID}", serverHandler.UpdateServerRole)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}/members/{userID}/roles", serverHandler.GetMemberRoles)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/servers/{id}/members/{userID}/roles", serverHandler.AssignRoleToMember)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Delete("/servers/{id}/members/{userID}/roles/{roleID}", serverHandler.RemoveRoleFromMember)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}/members-with-roles", serverHandler.GetMembersWithRoles)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}/my-permissions", serverHandler.GetMyPermissions)

			// Channel routes
			channelHandler := channel.NewHandler(channelService, auditSvc)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/servers/{serverID}/channels", channelHandler.CreateChannel)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Patch("/servers/{serverID}/channels/reorder", channelHandler.ReorderChannels)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{serverID}/channels", channelHandler.GetServerChannels)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/channels/{id}", channelHandler.GetChannel)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Put("/channels/{id}", channelHandler.UpdateChannel)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Delete("/channels/{id}", channelHandler.DeleteChannel)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/channels/{id}/overwrites", channelHandler.GetOverwrites)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Put("/channels/{id}/overwrites", channelHandler.UpsertOverwrite)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Delete("/channels/{id}/overwrites/{overwriteId}", channelHandler.DeleteOverwrite)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/channels/{id}/my-permissions", channelHandler.GetMyChannelPermissions)

			// Message routes — read endpoints gate on messages:read;
			// write endpoints (send/edit/delete/react/pin/forward/typing)
			// gate on messages:write. Search is a read.
			messageHandler := message.NewHandler(messageService, cdnHost)
			r.With(auth.RequireScope(auth.ScopeMessagesRead), userRateLimitMiddleware(msgSearchLimiter)).Get("/servers/{id}/messages/search", messageHandler.SearchMessages)
			r.With(auth.RequireScope(auth.ScopeMessagesRead), rateLimitMiddleware(msgReadLimiter)).Get("/channels/{channelID}/messages", messageHandler.GetChannelMessages)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite), userRateLimitMiddleware(msgWriteLimiter), ownerRateLimitMiddleware(msgWriteOwnerLimiter)).Post("/channels/{channelID}/messages", messageHandler.SendMessage)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Put("/messages/{id}", messageHandler.EditMessage)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Delete("/messages/{id}", messageHandler.DeleteMessage)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/messages/{id}/reactions", messageHandler.ToggleReaction)
			r.With(auth.RequireScope(auth.ScopeMessagesRead)).Get("/messages/{id}/versions", messageHandler.GetMessageVersions)
			r.With(auth.RequireScope(auth.ScopeMessagesRead)).Get("/channels/{channelID}/pins", messageHandler.GetChannelPins)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/channels/{channelID}/pins/{messageID}", messageHandler.PinMessage)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Delete("/channels/{channelID}/pins/{messageID}", messageHandler.UnpinMessage)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite), userRateLimitMiddleware(msgWriteLimiter), ownerRateLimitMiddleware(msgWriteOwnerLimiter)).Post("/channels/{channelID}/forward", messageHandler.ForwardToChannel)

			// Typing indicator — write (broadcasts presence into the channel)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/channels/{channelId}/typing", handleChannelTyping(repo, hub))

			// Voice routes — joining a voice channel as a bot is a presence
			// change akin to sending a typing indicator; treat as
			// messages:write. Participant reads are servers:read.
			voiceHandler := voice.NewHandler(voiceSvc, repo, hub)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Get("/channels/{channelId}/voice/token", voiceHandler.Token)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/channels/{channelId}/voice/join", voiceHandler.Join)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/channels/{channelId}/voice/leave", voiceHandler.Leave)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/channels/{channelId}/voice/participants", voiceHandler.Participants)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/channels/{channelId}/voice/participants/{targetUserId}/mute", voiceHandler.MuteParticipant)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/channels/{channelId}/voice/participants/{targetUserId}/kick", voiceHandler.KickParticipant)

			// Notification service — wired into DM, friend, and message flows.
			// Notifications are per-user inbox reads/marks; map to servers:read
			// (read the inbox) and profile:write (mutate read-state on the
			// bot's own account).
			notifSvc := notification.New(repo, hub)
			messageService.SetMentionNotify(notifSvc.NotifyMentions)
			notifHandler := notification.NewHandler(repo)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/notifications", notifHandler.GetNotifications)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Patch("/notifications/read-all", notifHandler.MarkAllRead)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Patch("/notifications/{id}/read", notifHandler.MarkRead)

			// Read-state + notification setting routes — per-user account state.
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/channels/{channelID}/read", handleMarkChannelRead(repo, hub))
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/dms/{channelID}/read", handleMarkDmRead(repo, hub))
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Patch("/channels/{channelID}/notifications", handleSetChannelNotifications(repo, hub))
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Patch("/dms/{channelID}/notifications", handleSetDmNotifications(repo, hub))
			// Bulk read-state hydration on app mount.
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/me/channel-state", handleGetMyChannelState(repo))

			// DM routes — scoped under messages:read (channel list + history)
			// and messages:write (open/send/delete/react/forward).
			dmHandler := dm.NewHandler(repo, hub)
			dmHandler.SetDmNotify(notifSvc.NotifyDM)
			r.With(auth.RequireScope(auth.ScopeMessagesRead)).Get("/dms", dmHandler.GetDmChannels)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/dms", dmHandler.OpenDmChannel)
			r.With(auth.RequireScope(auth.ScopeMessagesRead)).Get("/dms/{id}/messages", dmHandler.GetDmMessages)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/dms/{id}/messages", dmHandler.SendDmMessage)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Delete("/dms/{id}/messages/{messageId}", dmHandler.DeleteDmMessage)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/dms/{id}/messages/{messageId}/reactions", dmHandler.ToggleDmReaction)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite), userRateLimitMiddleware(msgWriteLimiter), ownerRateLimitMiddleware(msgWriteOwnerLimiter)).Post("/dms/{id}/forward", dmHandler.ForwardToDm)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/dms/{id}/members", dmHandler.AddMembers)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/dms/{id}/leave", dmHandler.Leave)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Delete("/dms/{id}/members/{userID}", dmHandler.RemoveMember)

			// Friend routes — profile-level state (the bot's friends list);
			// scoped on profile:write for all mutations, servers:read for reads.
			friendSvc := friend.NewService(repo, hub)
			friendSvc.SetNotifyFriendRequest(notifSvc.NotifyFriendRequest)
			friendSvc.SetNotifyFriendAccept(notifSvc.NotifyFriendAccept)
			friendHandler := friend.NewHandler(friendSvc)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/friends", friendHandler.GetFriends)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/friend-requests", friendHandler.GetRequests)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/friend-requests", friendHandler.SendRequest)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/friend-requests/{id}/accept", friendHandler.AcceptRequest)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Delete("/friend-requests/{id}", friendHandler.DeclineOrCancel)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Delete("/friends/{userId}", friendHandler.RemoveFriend)

			// Bin routes — posts and line-comments are first-class messages
			// (authored content that surfaces in a channel). Scope them on
			// the message scopes rather than adding a bin-specific pair.
			binHandler := bin.NewHandler(binService)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/channels/{channelID}/posts", binHandler.CreatePost)
			r.With(auth.RequireScope(auth.ScopeMessagesRead)).Get("/channels/{channelID}/posts", binHandler.ListPosts)
			r.With(auth.RequireScope(auth.ScopeMessagesRead)).Get("/posts/{postID}", binHandler.GetPost)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Put("/posts/{postID}", binHandler.EditPost)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Delete("/posts/{postID}", binHandler.DeletePost)
			r.With(auth.RequireScope(auth.ScopeMessagesRead)).Get("/posts/{postID}/versions", binHandler.GetVersions)
			r.With(auth.RequireScope(auth.ScopeMessagesRead)).Get("/posts/{postID}/versions/{versionID}", binHandler.GetVersion)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/posts/{postID}/line-comments", binHandler.CreateLineComment)
			r.With(auth.RequireScope(auth.ScopeMessagesRead)).Get("/posts/{postID}/line-comments", binHandler.GetLineComments)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Put("/line-comments/{id}", binHandler.UpdateLineComment)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Delete("/line-comments/{id}", binHandler.DeleteLineComment)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/channels/{channelID}/tags", binHandler.GetTags)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/channels/{channelID}/tags", binHandler.CreateTag)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Delete("/channels/{channelID}/tags/{tagID}", binHandler.DeleteTag)

			// Soundboard routes — upload/update/delete are server-admin state
			// (profile:write); list + play are read/write equivalents.
			sbRepo := soundboard.NewRepository(repo.DB())
			sbSvc := soundboard.NewService(sbRepo, spacesClient)
			soundboardHandler := soundboard.NewHandler(sbRepo, sbSvc, repo, hub, voiceSvc)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/soundboard", soundboardHandler.ListAll)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{serverId}/soundboard", soundboardHandler.List)
			r.With(auth.RequireScope(auth.ScopeProfileWrite), maxBodyMiddleware(1<<20 + 4096)).Post("/servers/{serverId}/soundboard", soundboardHandler.Upload)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Patch("/servers/{serverId}/soundboard/{soundId}", soundboardHandler.UpdateSound)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Delete("/servers/{serverId}/soundboard/{soundId}", soundboardHandler.DeleteSound)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/channels/{channelId}/soundboard/play", soundboardHandler.Play)

			// Invite routes — create/revoke are server-admin mutations
			// (profile:write); read/preview are servers:read.
			r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Post("/servers/{id}/invites", serverHandler.CreateInvite)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}/invites", serverHandler.ListServerInvites)
			r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Delete("/servers/{id}/invites/{code}", serverHandler.RevokeInvite)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}/invites/{code}/members", serverHandler.GetInviteMembers)
			// GET previews invite info without joining; POST actually joins.
			r.With(auth.RequireScope(auth.ScopeServersRead), rateLimitMiddleware(inviteLimiter)).Get("/invites/{code}", serverHandler.PreviewInvite)
			r.With(auth.RequireScope(auth.ScopeProfileWrite), rateLimitMiddleware(inviteLimiter)).Post("/invites/{code}", serverHandler.JoinInvite)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}/audit-log", serverHandler.GetAuditLog)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Put("/servers/{id}/vanity", serverHandler.SetVanityURL)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Put("/servers/{id}/categories", serverHandler.SetServerCategories)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}/categories", serverHandler.GetServerCategoriesForServer)

			// Bot routes — AI config is a server-admin concern, gated on
			// profile:write for bots acting via API key (a read-only bot
			// should not be able to reconfigure a server's AI provider).
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}/bots", botsHandler.ListBots)
			r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Post("/servers/{id}/bots", botsHandler.AddBot)
			r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Delete("/servers/{id}/bots/{botId}", botsHandler.RemoveBot)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}/ai-config", botsHandler.GetAIConfig)
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Put("/servers/{id}/ai-config", botsHandler.SetAIConfig)
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}/ai-usage", botsHandler.GetAIUsage)

			// Slash commands — bot-authenticated (api_key via plk_ prefix hits
			// the same AuthMiddlewareWith path). Writes (PUT/POST/DELETE) are
			// rate-limited to 50/hour per authenticated bot user and require
			// commands:write. The list endpoint is a read of the bot's own
			// command surface for that server, so servers:read is the right
			// bar.
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/bots/@me/servers/{id}/commands", botCommandsHandler.ListMyCommands)
			r.With(auth.RequireScope(auth.ScopeCommandsWrite), userRateLimitMiddleware(botCmdRegLimiter)).Put("/bots/@me/servers/{id}/commands", botCommandsHandler.BulkReplaceMyCommands)
			r.With(auth.RequireScope(auth.ScopeCommandsWrite), userRateLimitMiddleware(botCmdRegLimiter)).Post("/bots/@me/servers/{id}/commands", botCommandsHandler.UpsertMyCommand)
			r.With(auth.RequireScope(auth.ScopeCommandsWrite), userRateLimitMiddleware(botCmdRegLimiter)).Delete("/bots/@me/servers/{id}/commands/{name}", botCommandsHandler.DeleteMyCommand)

			// Slash commands — user-authenticated list + invoke. User JWTs
			// pass RequireScope unconditionally; API-key-authed bots listing
			// another bot's commands read the same metadata as "servers:read".
			// Invocation is a user action (not a bot action), so no scope
			// applies in practice — still gated to keep an API-key-bearing
			// attacker from using a narrow-scope key to invoke arbitrary
			// commands; messages:write covers the "fan-out writes a message"
			// consequence.
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/servers/{id}/commands", botCommandsHandler.ListServerCommands)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite), userRateLimitMiddleware(msgWriteLimiter), ownerRateLimitMiddleware(msgWriteOwnerLimiter)).Post("/channels/{channelID}/interactions", botCommandsHandler.Invoke)

			// Authenticated bot invite accept — a bot claiming a server via
			// its invite is a profile-level state change for the bot.
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/bots/invite/{token}/accept", botsHandler.AcceptInvite)

			// Bots owned by the current user — read of the owner's bot roster.
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/bots/mine", botsHandler.GetMyBots)

			// Registration invite routes — users manage their own codes.
			// These are owner-surface codes for letting new humans sign up;
			// bots have no legitimate reason to touch them, so gate on
			// profile:write so only broadly-scoped keys qualify.
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Get("/invites", handleListMyInvites(repo))
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/invites", handleCreateMyInvite(repo))

			// User routes — searches and reads gate on servers:read (they
			// surface other-user metadata used to build server views);
			// /users/me is a self-read and status is the bot's own profile.
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/users/search", handleUserSearch(repo))
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/users/me", handleGetMeSelf(repo))
			// PATCH /users/me mutates the profile (display name, avatar, etc.) —
			// admins have a separate tool for that and shouldn't change these
			// through an impersonation session.
			r.With(denyImpersonation, auth.RequireScope(auth.ScopeProfileWrite)).Patch("/users/me", handlePatchMe(repo, hub, cdnHost))
			r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/users/{id}", handleGetUser(repo))
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Patch("/users/@me/status", handleUpdateStatus(hub, repo))

			// Developer API key routes — entire group is off-limits for
			// impersonation. Minting a key as the user or renaming their
			// bots would give the admin post-session persistence. Bots
			// holding a `developer:manage` scope can manage their OWN keys
			// (revoke a compromised one, rename the bot user), but cannot
			// do so with any narrower scope — stops a leaked messages:write
			// token from being used to rotate the bot's credentials and
			// lock the owner out.
			r.Group(func(r chi.Router) {
				r.Use(denyImpersonation)
				r.Use(auth.RequireScope(auth.ScopeDeveloperManage))
				r.Get("/developer/keys", handleListAPIKeys(repo))
				r.Post("/developer/keys", handleCreateAPIKey(repo))
				r.Delete("/developer/keys/{id}", handleRevokeAPIKey(repo))
				r.Patch("/developer/bots/{botId}", handleRenameBotUser(repo))
				r.Patch("/developer/bots/{botId}/invite", botsHandler.UpdateInvitePermissions)
			})

			// GIPHY proxy — the search/trending endpoints exist so a bot can
			// paste a gif into a message; messages:write is the matching
			// scope.
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Get("/giphy/search", handleGiphySearch)
			r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Get("/giphy/trending", handleGiphyTrending)

			// File upload — 50 MB limit. Uploads almost always feed a
			// message attachment, so messages:write is the matching bar.
			r.With(auth.RequireScope(auth.ScopeMessagesWrite), maxBodyMiddleware(50*1024*1024)).Post("/upload", handleUpload(spacesClient, repo.DB()))

			// Theme routes — preferences + theme mutations are profile
			// state and have no business being touched by a support
			// session. GetPreferences is a read; still deny it so the
			// admin UI doesn't silently pull the user's preference blob
			// (covers every /me/preferences path). Public theme reads
			// under /themes are registered outside this group.
			//
			// D3: the entire /me/* surface modifies the bot's own profile
			// (even reading the preference blob would let a leaked token
			// exfiltrate it), so all of these gate on profile:write.
			r.Group(func(r chi.Router) {
				r.Use(denyImpersonation)
				r.Use(auth.RequireScope(auth.ScopeProfileWrite))
				r.Get("/me/preferences", themeHandler.GetPreferences)
				r.Put("/me/preferences/theme", themeHandler.SetActiveTheme)
				r.Post("/me/themes", themeHandler.CreateTheme)
				r.Put("/me/themes/{id}", themeHandler.UpdateTheme)
				r.Delete("/me/themes/{id}", themeHandler.DeleteTheme)
				r.Post("/me/themes/{id}/share", themeHandler.ShareTheme)
				r.Post("/me/themes/{id}/publish", themeHandler.TogglePublish)
				r.Post("/me/themes/install/{token}", themeHandler.InstallTheme)
				r.Post("/me/themes/generate", aiHandler.Generate)
			})
			r.With(auth.RequireScope(auth.ScopeProfileWrite)).Put("/themes/{id}/feature", themeHandler.ToggleFeature)
		})

		// Interaction-token auth — bearer credential is the token in the URL,
		// no JWT or API key required. A bot responds to a user-invoked slash
		// command with this token, NOT with its API key, so there is no
		// scope to enforce here; the `interactions:respond` scope is
		// defined for forward compatibility (if the interaction path ever
		// grows an API-key-authed variant) but is not gated on this route.
		// See docs/security/runbooks/d3-bot-scopes.md for context.
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

