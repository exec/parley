package voice

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"parley/internal/httputil"
	ws "parley/internal/websocket"
)

// activityHub is the slice of hub methods the activity handler needs.
type activityHub interface {
	BroadcastToChannel(channelID, eventType string, payload []byte)
}

type ActivityHandler struct {
	svc *Service
	hub activityHub
}

func NewActivityHandler(svc *Service, hub activityHub) *ActivityHandler {
	return &ActivityHandler{svc: svc, hub: hub}
}

// Start records the active activity for a virtual channel and broadcasts
// ACTIVITY_START. Caller must be a participant in voice:{vc}.
func (h *ActivityHandler) Start(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(w, r)
	if !ok {
		return
	}
	_, vcStr, ok := parseVCFromPath(w, r)
	if !ok {
		return
	}
	if isPart, err := h.svc.IsParticipant(r.Context(), vcStr, strconv.FormatInt(userID, 10)); err != nil || !isPart {
		httputil.JSONError(w, "forbidden: not a participant", http.StatusForbidden)
		return
	}
	var body struct {
		Type   string          `json:"type"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Type == "" {
		httputil.JSONError(w, "type required", http.StatusBadRequest)
		return
	}
	if err := h.svc.StartActivity(r.Context(), vcStr, body.Type, userID, body.Params); err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	a, err := h.svc.GetActivity(r.Context(), vcStr)
	if err != nil || a == nil {
		// Race: another caller raced an EndActivity between our StartActivity
		// and this read-back. Fall back to local state so the broadcast still
		// fires; the canonical Activity record will be whatever wins the race
		// in Redis. Log so this is visible if it ever fires.
		log.Printf("activity handler: read-back missed in Start for vc=%s err=%v; falling back to local state", vcStr, err)
		a = &Activity{
			Type:        body.Type,
			StartedBy:   userID,
			StartedAtMs: time.Now().UnixMilli(),
			Params:      body.Params,
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"vc":            vcStr,
		"type":          a.Type,
		"started_by":    strconv.FormatInt(a.StartedBy, 10),
		"started_at_ms": a.StartedAtMs,
		"params":        a.Params,
	})
	h.hub.BroadcastToChannel(vcStr, ws.EventActivityStart, payload)
	w.WriteHeader(http.StatusNoContent)
}

// End clears the active activity and broadcasts ACTIVITY_END.
// Caller must be a participant.
func (h *ActivityHandler) End(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(w, r)
	if !ok {
		return
	}
	_, vcStr, ok := parseVCFromPath(w, r)
	if !ok {
		return
	}
	if isPart, err := h.svc.IsParticipant(r.Context(), vcStr, strconv.FormatInt(userID, 10)); err != nil || !isPart {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := h.svc.EndActivity(r.Context(), vcStr); err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	payload, _ := json.Marshal(map[string]any{"vc": vcStr})
	h.hub.BroadcastToChannel(vcStr, ws.EventActivityEnd, payload)
	w.WriteHeader(http.StatusNoContent)
}

// Get returns the active activity for a virtual channel, or 204 if none.
func (h *ActivityHandler) Get(w http.ResponseWriter, r *http.Request) {
	_, vcStr, ok := parseVCFromPath(w, r)
	if !ok {
		return
	}
	a, err := h.svc.GetActivity(r.Context(), vcStr)
	if err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if a == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

// parseVCFromPath mirrors voice.Handler.parseVC for handlers that don't have
// a Handler receiver (RingHandler / ActivityHandler).
func parseVCFromPath(w http.ResponseWriter, r *http.Request) (VirtualChannel, string, bool) {
	raw := r.PathValue("vc")
	vc, err := ParseVirtualChannel(raw)
	if err != nil {
		httputil.JSONError(w, "invalid virtual channel id", http.StatusBadRequest)
		return VirtualChannel{}, "", false
	}
	return vc, raw, true
}
