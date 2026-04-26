package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"parley/internal/auth"
	"parley/internal/desktopauth"
)

func handleDesktopAuthIssue(svc *desktopauth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := auth.GetUserIDFromContext(r)
		if userID == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			State string `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.State == "" {
			jsonError(w, "state is required", http.StatusBadRequest)
			return
		}
		code, err := svc.Issue(r.Context(), userID, req.State)
		if err != nil {
			jsonError(w, "failed to issue code", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"code": code})
	}
}

func handleDesktopAuthExchange(svc *desktopauth.Service, authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// CSRF / login-fixation gate. The exchange endpoint is the only
		// place an unauthenticated POST mints a session — that made it the
		// natural target for a cross-origin form auto-submit (text/plain is
		// CORS-safe so no preflight fires) that landed an attacker's session
		// cookie on the victim's browser. Require Origin to be on the CORS
		// allowlist (the legitimate caller is the Tauri webview, which
		// always sends Origin: tauri://localhost on cross-origin POSTs).
		// Reject missing / null / unknown origins outright.
		origin := r.Header.Get("Origin")
		if origin == "" || origin == "null" || !allowedOrigins[origin] {
			jsonError(w, "forbidden origin", http.StatusForbidden)
			return
		}

		var req struct {
			Code  string `json:"code"`
			State string `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" || req.State == "" {
			jsonError(w, "code and state are required", http.StatusBadRequest)
			return
		}
		userID, err := svc.Exchange(r.Context(), req.Code, req.State)
		if err != nil {
			if errors.Is(err, desktopauth.ErrNotFound) || errors.Is(err, desktopauth.ErrStateMismatch) {
				jsonError(w, "invalid or expired code", http.StatusUnauthorized)
				return
			}
			jsonError(w, "failed to exchange code", http.StatusInternalServerError)
			return
		}
		user, err := authService.GetUserByID(r.Context(), userID)
		if err != nil {
			jsonError(w, "user not found", http.StatusUnauthorized)
			return
		}
		token, err := authService.GenerateTokenForUser(userID)
		if err != nil {
			jsonError(w, "failed to generate token", http.StatusInternalServerError)
			return
		}
		// Intentionally NO Set-Cookie here. The desktop client is the only
		// legitimate caller; it lives at tauri://localhost (cross-origin to
		// parley.byexec.com), so a SameSite=Lax cookie wouldn't ship on its
		// subsequent requests anyway — desktop authenticates via the JSON
		// `token` field returned below, attached as Authorization: Bearer.
		// Keeping the Set-Cookie was the primitive that turned this endpoint
		// into a one-shot session-fixation gadget for any attacker who could
		// trick a victim's browser into POSTing a code they minted.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{User: user, Token: token})
	}
}
