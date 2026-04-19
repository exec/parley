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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{User: user, Token: token})
	}
}
