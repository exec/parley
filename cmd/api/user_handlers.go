package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"parley/internal/auth"
	"parley/internal/db"
	ws "parley/internal/websocket"
)

func handleGetMe(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		var id int64
		fmt.Sscan(userIDStr, &id)
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
			Badges:        user.Badges,
			EmailVerified: user.EmailVerified,
			PhoneNumber:   user.PhoneNumber,
			PhoneVerified: user.PhoneVerified,
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

func handleUpdateProfile(authService *auth.AuthService, repo *db.Repository, hub *ws.Hub) http.HandlerFunc {
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
						"user_id":    userIDStr,
						"username":   user.Username,
						"avatar_url": user.AvatarURL,
					})
					if marshalErr == nil {
						for _, srv := range servers {
							hub.BroadcastToChannel(fmt.Sprintf("server:%d", srv.ID), ws.EventUserUpdate, payload)
						}
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}
