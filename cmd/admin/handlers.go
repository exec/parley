package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"parley/internal/db"
)

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	u, err := repo.AdminGetUser(r.Context(), req.Username)
	if err != nil || !u.Active {
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	repo.AdminUpdateLastLogin(r.Context(), u.ID)

	claims := jwt.MapClaims{
		"admin_id": u.ID,
		"username": u.Username,
		"exp":      time.Now().Add(12 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(adminJWTSecret))
	if err != nil {
		jsonError(w, "token generation failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{
		"token":    tokenStr,
		"username": u.Username,
	})
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := repo.GetAdminStats(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, stats)
}

func handleListBots(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	bots, err := repo.AdminGetBots(r.Context(), q, limit, offset)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if bots == nil {
		bots = []db.AdminBotRow{}
	}
	jsonOK(w, bots)
}

func handleListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	users, err := repo.AdminSearchUsers(r.Context(), q, limit, offset)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if users == nil {
		users = []db.User{}
	}
	jsonOK(w, users)
}

func handleGetUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	u, err := repo.GetUserByID(r.Context(), id)
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	jsonOK(w, u)
}

func handleBanUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Reason == "" {
		req.Reason = "violation of Terms of Service"
	}
	if err := repo.BanUser(r.Context(), id, req.Reason); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	repo.ForceLogoutUser(r.Context(), id)
	publishKick(fmt.Sprintf("%d", id))
	log.Printf("audit: admin_ban user_id=%d reason=%q", id, req.Reason)
	jsonOK(w, map[string]string{"message": "user banned"})
}

func handleUnbanUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := repo.UnbanUser(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("audit: admin_unban user_id=%d", id)
	jsonOK(w, map[string]string{"message": "user unbanned"})
}

func handleForceLogout(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := repo.ForceLogoutUser(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	publishKick(fmt.Sprintf("%d", id))
	jsonOK(w, map[string]string{"message": "user force logged out"})
}

func handleImpersonate(w http.ResponseWriter, r *http.Request) {
	if parleyJWTSecret == "" {
		jsonError(w, "impersonation not configured", http.StatusNotImplemented)
		return
	}
	userIDStr := chi.URLParam(r, "id")
	claims := jwt.MapClaims{
		"user_id":       userIDStr,
		"impersonation": true,
		"exp":           time.Now().Add(1 * time.Hour).Unix(),
		"iat":           time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(parleyJWTSecret))
	if err != nil {
		jsonError(w, "token generation failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"token": tokenStr})
}

// handleAddUserInvites bumps a single user's registration-invite allowance
// by a caller-specified count. Capped at 10 per call — no cumulative cap.
func handleAddUserInvites(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.Count < 1 || req.Count > 10 {
		jsonError(w, "count must be between 1 and 10", http.StatusBadRequest)
		return
	}
	if err := repo.AdminAddUserInvites(r.Context(), id, req.Count); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("audit: admin_add_invites user_id=%d count=%d", id, req.Count)
	jsonOK(w, map[string]int{"count_added": req.Count})
}

// handleBulkAddInvites adds N registration invites to every active,
// non-system, non-bot user. Capped at 10 per call.
func handleBulkAddInvites(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.Count < 1 || req.Count > 10 {
		jsonError(w, "count must be between 1 and 10", http.StatusBadRequest)
		return
	}
	updated, err := repo.AdminBulkAddInvites(r.Context(), req.Count)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("audit: admin_bulk_add_invites count=%d users_updated=%d", req.Count, updated)
	jsonOK(w, map[string]interface{}{
		"count_added":    req.Count,
		"users_updated":  updated,
	})
}

func handleSetBadges(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req struct {
		Badges int `json:"badges"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := repo.AdminSetBadges(r.Context(), id, req.Badges); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"badges": req.Badges})
}

func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := repo.AdminDeleteUser(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"message": "user deleted"})
}

func handleSearchMessages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	authorID, _ := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	serverID, _ := strconv.ParseInt(r.URL.Query().Get("server_id"), 10, 64)
	limit := queryInt(r, "limit", 50)
	msgs, err := repo.SearchMessages(r.Context(), serverID, q, authorID, 0, limit, 0)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []*db.Message{}
	}
	jsonOK(w, msgs)
}

func handleMessageContext(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	before := queryInt(r, "before", 10)
	after := queryInt(r, "after", 10)
	msgs, err := repo.GetMessageContext(r.Context(), id, before, after)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []db.Message{}
	}
	jsonOK(w, map[string]interface{}{
		"messages":   msgs,
		"message_id": id,
	})
}

func handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := repo.AdminDeleteMessage(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"message": "deleted"})
}

func handleListReports(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	reports, err := repo.GetReports(r.Context(), status, limit, offset)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if reports == nil {
		reports = []db.Report{}
	}
	jsonOK(w, reports)
}

func handleGetReport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	report, err := repo.GetReport(r.Context(), id)
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	result := map[string]interface{}{"report": report}
	if report.ReportedMessageID != nil {
		msgs, err := repo.GetMessageContext(r.Context(), *report.ReportedMessageID, 10, 10)
		if err == nil {
			result["context"] = msgs
			result["target_message_id"] = *report.ReportedMessageID
		}
	}
	jsonOK(w, result)
}

func handleResolveReport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Status == "" {
		req.Status = "resolved"
	}
	adminID := getAdminID(r)
	if err := repo.ResolveReport(r.Context(), id, adminID, req.Status, req.Note); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"message": "report updated"})
}

func handleListCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := repo.GetReportCategories(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if cats == nil {
		cats = []db.ReportCategory{}
	}
	jsonOK(w, cats)
}

func handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}
	cat, err := repo.CreateReportCategory(r.Context(), req.Name)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, cat)
}

func handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := repo.DeleteReportCategory(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"message": "deleted"})
}

// Server category management

func handleListServerCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := repo.GetServerCategories(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if cats == nil {
		cats = []db.ServerCategory{}
	}
	jsonOK(w, cats)
}

func handleCreateServerCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}
	cat, err := repo.CreateServerCategory(r.Context(), req.Name)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, cat)
}

func handleDeleteServerCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := repo.DeleteServerCategory(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"message": "deleted"})
}

func handleListServers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	servers, err := repo.AdminGetServers(r.Context(), q, limit, offset)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if servers == nil {
		servers = []db.Server{}
	}
	jsonOK(w, servers)
}

func handleGenerateInvite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	code := make([]byte, 8)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			jsonError(w, "failed to generate code", http.StatusInternalServerError)
			return
		}
		code[i] = charset[n.Int64()]
	}

	// Get system user as invite creator
	sysUser, err := repo.GetSystemUser(r.Context())
	if err != nil {
		jsonError(w, "could not get system user", http.StatusInternalServerError)
		return
	}

	invite := &db.Invite{
		ServerID:  id,
		Code:      string(code),
		CreatedBy: sysUser.ID,
	}
	if err := repo.CreateInvite(r.Context(), invite); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"code": invite.Code})
}

func handleDisbandServer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	// Get server name before deletion
	servers, _ := repo.AdminGetServers(r.Context(), "", 100, 0)
	serverName := fmt.Sprintf("server #%d", id)
	for _, s := range servers {
		if s.ID == id {
			serverName = s.Name
			break
		}
	}

	// Get all member user IDs
	memberIDs, err := repo.GetServerMemberUserIDs(r.Context(), id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get system user
	sysUser, err := repo.GetSystemUser(r.Context())
	if err != nil {
		log.Printf("Warning: could not get system user for DMs: %v", err)
	}

	// Delete server (CASCADE removes channels, members, messages)
	if _, err := repo.DB().ExecContext(r.Context(), `DELETE FROM servers WHERE id = $1`, id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send DMs to all former members
	if sysUser != nil {
		msg := fmt.Sprintf("The server **%s** was disbanded due to violations of Parley's Terms of Service. We apologize for any disruption.", serverName)
		for _, memberID := range memberIDs {
			if memberID == sysUser.ID {
				continue
			}
			if err := repo.SendSystemDM(r.Context(), sysUser.ID, memberID, msg); err != nil {
				log.Printf("Warning: failed to send DM to user %d: %v", memberID, err)
			}
		}
	}

	jsonOK(w, map[string]interface{}{
		"message":          "server disbanded",
		"members_notified": len(memberIDs),
	})
}
