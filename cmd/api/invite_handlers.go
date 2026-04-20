package main

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"

	"parley/internal/auth"
	"parley/internal/db"
)

// inviteCodeCharset is the alphabet for registration invite codes. No I/l/1/0
// pairs so codes are readable out of context.
const inviteCodeCharset = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
const inviteCodeLength = 10

func generateInviteCode() (string, error) {
	code := make([]byte, inviteCodeLength)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(inviteCodeCharset))))
		if err != nil {
			return "", err
		}
		code[i] = inviteCodeCharset[n.Int64()]
	}
	return string(code), nil
}

// inviteResponse mirrors db.RegistrationInvite but exposes int64 IDs as
// strings for frontend consumption and drops empty fields.
type inviteResponse struct {
	Code            string  `json:"code"`
	InviteeUsername string  `json:"invitee_username,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UsedAt          *string `json:"used_at,omitempty"`
}

func toInviteResponse(ri db.RegistrationInvite) inviteResponse {
	out := inviteResponse{
		Code:            ri.Code,
		InviteeUsername: ri.InviteeUsername,
		CreatedAt:       ri.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if ri.UsedAt != nil {
		s := ri.UsedAt.Format("2006-01-02T15:04:05Z07:00")
		out.UsedAt = &s
	}
	return out
}

// handleListMyInvites returns the invite codes the authenticated user owns,
// plus their remaining invite_count balance.
func handleListMyInvites(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "user not authenticated", http.StatusUnauthorized)
			return
		}
		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			jsonError(w, "invalid user id", http.StatusBadRequest)
			return
		}

		count, err := repo.GetUserInviteCount(r.Context(), userID)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		invites, err := repo.ListUserRegistrationInvites(r.Context(), userID)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		out := make([]inviteResponse, 0, len(invites))
		for _, ri := range invites {
			out = append(out, toInviteResponse(ri))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"invite_count": count,
			"invites":      out,
		})
	}
}

// handleCreateMyInvite generates a new single-use code for the authenticated
// user, atomically decrementing their invite_count. Returns 400 if they have
// no invites left.
func handleCreateMyInvite(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := auth.GetUserIDFromContext(r)
		if userIDStr == "" {
			jsonError(w, "user not authenticated", http.StatusUnauthorized)
			return
		}
		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			jsonError(w, "invalid user id", http.StatusBadRequest)
			return
		}

		code, err := generateInviteCode()
		if err != nil {
			jsonError(w, "failed to generate code", http.StatusInternalServerError)
			return
		}

		if err := repo.CreateRegistrationInvite(r.Context(), userID, code); err != nil {
			if err == db.ErrInvalidOperation {
				jsonError(w, "you have no invites left", http.StatusBadRequest)
				return
			}
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"code": code})
	}
}

// handleCheckInvite is an unauthenticated probe used by the Register page to
// tell the user "this code is invalid" without first making them fill out the
// whole form. Only returns whether the code is present + unused.
func handleCheckInvite(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			jsonError(w, "code is required", http.StatusBadRequest)
			return
		}
		valid, err := repo.RegistrationInviteUnused(r.Context(), code)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"valid": valid})
	}
}
