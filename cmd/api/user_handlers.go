package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/validation"
	ws "parley/internal/websocket"
)

func handleGetMe(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		id, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			jsonError(w, "invalid user ID", http.StatusBadRequest)
			return
		}
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
			DisplayName:   user.DisplayName,
			Badges:        user.Badges,
			EmailVerified: user.EmailVerified,
		})
	}
}

// handleGetMePhone returns phone number and verification status for the authenticated user.
// Fetched on-demand in settings so that phone number is never written to localStorage.
func handleGetMePhone(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		id, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			jsonError(w, "invalid user ID", http.StatusBadRequest)
			return
		}
		user, err := repo.GetUserByID(r.Context(), id)
		if err != nil {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"phone_number":   user.PhoneNumber,
			"phone_verified": user.PhoneVerified,
		})
	}
}

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

func handleUpdateProfile(authService *auth.AuthService, repo *db.Repository, hub *ws.Hub, cdnHost string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "user not authenticated", http.StatusUnauthorized)
			return
		}

		var req struct {
			Username        string  `json:"username"`
			CurrentPassword string  `json:"current_password"`
			NewPassword     string  `json:"new_password"`
			AvatarURL       string  `json:"avatar_url"`
			BannerURL       string  `json:"banner_url"`
			Bio             *string `json:"bio"`
			DisplayName     *string `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		// Validate avatar/banner URLs must be empty or from the CDN host.
		if err := validateMediaURL(req.AvatarURL, cdnHost); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := validateMediaURL(req.BannerURL, cdnHost); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		user, err := authService.UpdateProfile(r.Context(), userIDStr, req.Username, req.CurrentPassword, req.NewPassword, req.AvatarURL, req.BannerURL, req.Bio, req.DisplayName)
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
						"user_id":      userIDStr,
						"username":     user.Username,
						"avatar_url":   user.AvatarURL,
						"banner_url":   user.BannerURL,
						"display_name": user.DisplayName,
						"bio":          user.Bio,
					})
					if marshalErr == nil {
						topics := make([]string, 0, len(servers))
						for _, srv := range servers {
							topics = append(topics, fmt.Sprintf("server:%d", srv.ID))
						}
						hub.BroadcastToChannels(topics, ws.EventUserUpdate, payload)
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}

// userMeResponse builds the JSON body for GET/PATCH /api/users/me.
func userMeResponse(u *db.User) map[string]interface{} {
	return map[string]interface{}{
		"id":             fmt.Sprintf("%d", u.ID),
		"username":       u.Username,
		"display_name":   u.DisplayName,
		"avatar_url":     u.AvatarURL,
		"banner_url":     u.BannerURL,
		"bio":            u.Bio,
		"badges":         u.Badges,
		"email_verified": u.EmailVerified,
		"status_type":    u.StatusType,
		"status_text":    u.StatusText,
	}
}

// handleGetMeSelf handles GET /api/users/me — returns the full profile for the
// authenticated identity (JWT or bot API key).
func handleGetMeSelf(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			jsonError(w, "invalid user ID", http.StatusBadRequest)
			return
		}
		user, err := repo.GetUserByID(r.Context(), id)
		if err != nil {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(userMeResponse(user))
	}
}

// handlePatchMe handles PATCH /api/users/me — updates username, display_name,
// and/or avatar_url. Password and email changes are ignored.
func handlePatchMe(repo *db.Repository, hub *ws.Hub, cdnHost string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Username    *string `json:"username"`
			DisplayName *string `json:"display_name"`
			AvatarURL   *string `json:"avatar_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.AvatarURL != nil {
			if err := validateMediaURL(*req.AvatarURL, cdnHost); err != nil {
				jsonError(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			jsonError(w, "invalid user ID", http.StatusBadRequest)
			return
		}

		user, err := repo.GetUserByID(r.Context(), userID)
		if err != nil {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}

		if req.Username != nil && *req.Username != "" {
			normalized := validation.NormalizeUsername(*req.Username)
			if !validation.ValidUsername(normalized) {
				jsonError(w, "username may only contain letters, numbers, underscores, hyphens and dots", http.StatusBadRequest)
				return
			}
			user.Username = normalized
		}
		if req.DisplayName != nil {
			user.DisplayName = validation.SanitizeSingleLine(*req.DisplayName)
		}
		if req.AvatarURL != nil {
			user.AvatarURL = *req.AvatarURL
		}

		if err := repo.UpdateUserFields(r.Context(), userID, user.Username, user.DisplayName, user.AvatarURL); err != nil {
			if db.IsUniqueViolation(err) {
				jsonError(w, "username already taken", http.StatusConflict)
				return
			}
			jsonError(w, "failed to update profile", http.StatusInternalServerError)
			return
		}

		updated, _ := repo.GetUserByID(r.Context(), userID)
		if updated == nil {
			updated = user
		}

		// Broadcast USER_UPDATE to all servers the user belongs to (using DB-confirmed values).
		if hub != nil {
			servers, serversErr := repo.GetServersByUserID(r.Context(), userID)
			if serversErr == nil {
				payload, marshalErr := json.Marshal(map[string]string{
					"user_id":      userIDStr,
					"username":     updated.Username,
					"display_name": updated.DisplayName,
					"avatar_url":   updated.AvatarURL,
				})
				if marshalErr == nil {
					topics := make([]string, 0, len(servers))
					for _, srv := range servers {
						topics = append(topics, fmt.Sprintf("server:%d", srv.ID))
					}
					hub.BroadcastToChannels(topics, ws.EventUserUpdate, payload)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(userMeResponse(updated))
	}
}
