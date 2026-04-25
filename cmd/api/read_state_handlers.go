package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
	ws "parley/internal/websocket"
)

type markReadRequest struct {
	MessageID string `json:"message_id"`
}

type setNotificationsRequest struct {
	Setting string `json:"setting"`
}

// handleMarkChannelRead — POST /api/channels/{channelID}/read
func handleMarkChannelRead(repo *db.Repository, hub *ws.Hub) http.HandlerFunc {
	return markReadHandler(repo, hub, db.ChannelKindServer, channelMembershipCheck(repo))
}

// handleMarkDmRead — POST /api/dms/{channelID}/read
func handleMarkDmRead(repo *db.Repository, hub *ws.Hub) http.HandlerFunc {
	return markReadHandler(repo, hub, db.ChannelKindDM, dmMembershipCheck(repo))
}

func markReadHandler(repo *db.Repository, hub *ws.Hub, kind db.ChannelKind, ensureMember func(ctx context.Context, userID, channelID int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		userID, _ := strconv.ParseInt(userIDStr, 10, 64)

		channelID, err := strconv.ParseInt(chi.URLParam(r, "channelID"), 10, 64)
		if err != nil {
			httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
			return
		}

		if err := ensureMember(r.Context(), userID, channelID); err != nil {
			httputil.JSONError(w, "forbidden", http.StatusForbidden)
			return
		}

		var req markReadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		msgID, err := strconv.ParseInt(req.MessageID, 10, 64)
		if err != nil {
			httputil.JSONError(w, "invalid message id", http.StatusBadRequest)
			return
		}

		if err := repo.UpsertReadMarker(r.Context(), userID, kind, channelID, msgID); err != nil {
			httputil.InternalError(w, err)
			return
		}

		// Multi-tab sync — broadcast to user's own sessions only.
		if hub != nil {
			payload, perr := json.Marshal(map[string]any{
				"channel_kind":         int16(kind),
				"channel_id":           strconv.FormatInt(channelID, 10),
				"last_read_message_id": req.MessageID,
			})
			if perr == nil {
				hub.SendToUser(userIDStr, ws.EventChannelReadStateUpdate, payload)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func handleSetChannelNotifications(repo *db.Repository, hub *ws.Hub) http.HandlerFunc {
	return notificationsHandler(repo, hub, db.ChannelKindServer, channelMembershipCheck(repo))
}

func handleSetDmNotifications(repo *db.Repository, hub *ws.Hub) http.HandlerFunc {
	return notificationsHandler(repo, hub, db.ChannelKindDM, dmMembershipCheck(repo))
}

func notificationsHandler(repo *db.Repository, hub *ws.Hub, kind db.ChannelKind, ensureMember func(ctx context.Context, userID, channelID int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		userID, _ := strconv.ParseInt(userIDStr, 10, 64)

		channelID, err := strconv.ParseInt(chi.URLParam(r, "channelID"), 10, 64)
		if err != nil {
			httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
			return
		}

		if err := ensureMember(r.Context(), userID, channelID); err != nil {
			httputil.JSONError(w, "forbidden", http.StatusForbidden)
			return
		}

		var req setNotificationsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		var setting db.NotificationSetting
		switch req.Setting {
		case "ALL":
			setting = db.NotificationAll
		case "MENTIONS_ONLY":
			setting = db.NotificationMentionsOnly
		case "MUTED":
			setting = db.NotificationMuted
		default:
			httputil.JSONError(w, "invalid setting", http.StatusBadRequest)
			return
		}

		if err := repo.UpsertNotificationSetting(r.Context(), userID, kind, channelID, setting); err != nil {
			httputil.InternalError(w, err)
			return
		}

		if hub != nil {
			payload, perr := json.Marshal(map[string]any{
				"channel_kind":         int16(kind),
				"channel_id":           strconv.FormatInt(channelID, 10),
				"notification_setting": int16(setting),
			})
			if perr == nil {
				hub.SendToUser(userIDStr, ws.EventChannelNotificationUpdate, payload)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// handleGetMyChannelState — GET /api/me/channel-state
// Returns every user_channel_state row for the authenticated user. The frontend
// hydrates its bulk read-state + notification-setting cache from this on app
// mount; subsequent updates flow via WS events.
func handleGetMyChannelState(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		userID, _ := strconv.ParseInt(userIDStr, 10, 64)

		rows, err := repo.BulkGetUserChannelState(r.Context(), userID)
		if err != nil {
			httputil.InternalError(w, err)
			return
		}
		if rows == nil {
			rows = []db.UserChannelState{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rows)
	}
}

// channelMembershipCheck returns "user is a member of the server containing this channel."
func channelMembershipCheck(repo *db.Repository) func(ctx context.Context, userID, channelID int64) error {
	return func(ctx context.Context, userID, channelID int64) error {
		ch, err := repo.GetChannelByID(ctx, channelID)
		if err != nil {
			return err
		}
		member, err := repo.GetMember(ctx, ch.ServerID, userID)
		if err != nil || member == nil {
			return errors.New("not a member")
		}
		return nil
	}
}

// dmMembershipCheck returns "user is in the dm_channel_members for this DM channel."
// Migration #65 backfilled dm_channel_members for all 1:1 channels, so a single
// IsDmMember lookup serves both legacy 1:1 and group channels uniformly.
func dmMembershipCheck(repo *db.Repository) func(ctx context.Context, userID, channelID int64) error {
	return func(ctx context.Context, userID, channelID int64) error {
		isMember, err := repo.IsDmMember(ctx, channelID, userID)
		if err != nil {
			return err
		}
		if !isMember {
			return errors.New("not a member")
		}
		return nil
	}
}
