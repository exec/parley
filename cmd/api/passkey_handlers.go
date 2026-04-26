package main

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"parley/internal/auth"
	"parley/internal/passkey"
)

func handleRemovePassword(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// Collapse the "has at least one passkey" check and the password
		// clear into one SQL statement. A concurrent DELETE against the
		// last passkey that ran between a prior List() and the UPDATE
		// would otherwise leave the account with neither a password nor
		// a passkey (F-auth-removepw-race). RowsAffected distinguishes
		// the no-passkeys case from a successful clear.
		cleared, err := authService.RemovePasswordIfPasskeyExists(r.Context(), userIDStr)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !cleared {
			jsonError(w, "cannot remove password without at least one passkey set up", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Password removed"})
	}
}

func handlePasskeyRegisterBegin(svc *passkey.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		creation, sessionID, err := svc.RegisterBegin(r.Context(), userIDStr)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"options":    creation,
			"session_id": sessionID,
		})
	}
}

func handlePasskeyRegisterFinish(svc *passkey.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var meta struct {
			SessionID  string          `json:"session_id"`
			Name       string          `json:"name"`
			Credential json.RawMessage `json:"credential"`
		}
		if err := json.NewDecoder(r.Body).Decode(&meta); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := svc.RegisterFinish(r.Context(), userIDStr, meta.SessionID, meta.Name, meta.Credential); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Passkey registered successfully"})
	}
}

func handlePasskeyLoginBegin(svc *passkey.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		assertion, sessionID, err := svc.LoginBegin(r.Context())
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"options":    assertion,
			"session_id": sessionID,
		})
	}
}

func handlePasskeyLoginFinish(svc *passkey.Service, authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			jsonError(w, "failed to read body", http.StatusBadRequest)
			return
		}
		var meta struct {
			SessionID  string          `json:"session_id"`
			Credential json.RawMessage `json:"credential"`
		}
		if err := json.Unmarshal(body, &meta); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		userIDStr, err := svc.LoginFinish(r.Context(), meta.SessionID, meta.Credential)
		if err != nil {
			jsonError(w, "invalid passkey", http.StatusUnauthorized)
			return
		}
		dbUser, err := authService.GetUserByID(r.Context(), userIDStr)
		if err != nil {
			jsonError(w, "user not found", http.StatusUnauthorized)
			return
		}
		token, err := authService.GenerateTokenForUser(userIDStr)
		if err != nil {
			jsonError(w, "failed to generate token", http.StatusInternalServerError)
			return
		}
		auth.SetSessionCookie(w, token, int(authService.TokenTTL().Seconds()))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{User: dbUser, Token: token})
	}
}

func handleListPasskeys(svc *passkey.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		passkeys, err := svc.List(r.Context(), userIDStr)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(passkeys)
	}
}

func handleDeletePasskey(svc *passkey.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		if err := svc.Delete(r.Context(), userIDStr, id); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleRenamePasskey(svc *passkey.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := chi.URLParam(r, "id")
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := svc.Rename(r.Context(), userIDStr, id, req.Name); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Passkey renamed"})
	}
}
