package main

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"parley/internal/auth"
)

// requestIP resolves the client IP. Behind the DMZ nginx, r.RemoteAddr is the
// DMZ's internal IP (10.10.10.5) — the real client comes in via X-Forwarded-For,
// which nginx populates from Cloudflare's CF-Connecting-IP. Shared helper in
// internal/auth so the middleware does the same thing.
func requestIP(r *http.Request) string {
	return auth.ClientIP(r)
}

func handleAuthRegister(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		user, token, err := authService.Register(r.Context(), req.Username, req.Email, req.Phone, req.Password, req.InviteCode, requestIP(r))
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
		user, token, err := authService.Login(r.Context(), emailOrPhone, req.Password, requestIP(r))
		if err != nil {
			jsonError(w, err.Error(), http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AuthResponse{User: user, Token: token})
	}
}

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

func handleForgotPassword(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		// Always succeed to prevent user enumeration
		_ = authService.RequestPasswordReset(r.Context(), req.Email)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "If an account with that email exists, a reset link has been sent.",
		})
	}
}

func handleResetPassword(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Token    string `json:"token"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := authService.ResetPassword(r.Context(), req.Token, req.Password); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Password reset successfully. You can now log in."})
	}
}

func handleImpersonateToken(authService *auth.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetUserID := r.Header.Get("X-Admin-Impersonate")
		adminSecret := r.Header.Get("X-Admin-Secret")

		expectedSecret := os.Getenv("ADMIN_IMPERSONATE_SECRET")
		if expectedSecret == "" || subtle.ConstantTimeCompare([]byte(adminSecret), []byte(expectedSecret)) != 1 {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		targetUserID = strings.TrimSpace(targetUserID)
		if targetUserID == "" {
			jsonError(w, "target user ID is required", http.StatusBadRequest)
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
