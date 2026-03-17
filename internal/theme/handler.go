package theme

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"parley/internal/server"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

type setThemeReq struct {
	Theme         string `json:"theme"`
	CustomThemeID *int   `json:"custom_theme_id,omitempty"`
}

type themeReq struct {
	Name          string  `json:"name"`
	CSS           string  `json:"css"`
	BaseTheme     string  `json:"base_theme"`
	BackgroundURL *string `json:"background_url,omitempty"`
}

type errResp struct {
	Error         string   `json:"error"`
	OffendingURLs []string `json:"offending_urls,omitempty"`
}

type publishReq struct {
	Published bool `json:"published"`
}

type featureReq struct {
	Featured bool `json:"featured"`
}

func writeErr(w http.ResponseWriter, r *http.Request, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	render.JSON(w, r, errResp{Error: msg})
}

func userID(r *http.Request) (int64, bool) {
	s, ok := r.Context().Value(server.UserIDKey).(string)
	if !ok || s == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(s, 10, 64)
	return id, err == nil
}

var validBuiltin = map[string]bool{
	"rory": true, "citron-dark": true, "citron-light": true,
	"neon-nights": true, "abyss": true, "sakura": true,
}

func (h *Handler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	p, err := h.svc.GetPreferences(r.Context(), uid)
	if errors.Is(err, ErrNotFound) {
		// User predates the themes feature — return sensible defaults
		render.JSON(w, r, &UserPreferences{
			ActiveTheme:         "rory",
			ActiveCustomThemeID: nil,
			CustomThemes:        []UserTheme{},
		})
		return
	}
	if err != nil {
		writeErr(w, r, 500, err.Error())
		return
	}
	render.JSON(w, r, p)
}

func (h *Handler) SetActiveTheme(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	var req setThemeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, 400, "invalid request body")
		return
	}
	if req.Theme != "custom" && !validBuiltin[req.Theme] {
		writeErr(w, r, 400, "unknown theme ID")
		return
	}
	if req.Theme == "custom" && req.CustomThemeID == nil {
		writeErr(w, r, 400, "custom_theme_id required when theme is 'custom'")
		return
	}
	if err := h.svc.SetActiveTheme(r.Context(), uid, req.Theme, req.CustomThemeID); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeErr(w, r, 404, "theme not found")
			return
		}
		writeErr(w, r, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (h *Handler) CreateTheme(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	var req themeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, 400, "invalid request body")
		return
	}
	if len(req.Name) == 0 || len(req.Name) > 64 {
		writeErr(w, r, 400, "name must be 1-64 characters")
		return
	}
	if req.BaseTheme == "" {
		req.BaseTheme = "rory"
	}
	if !validBuiltin[req.BaseTheme] {
		writeErr(w, r, 400, "base_theme must be a built-in theme")
		return
	}
	t, err := h.svc.CreateTheme(r.Context(), uid, req.Name, req.CSS, req.BaseTheme, req.BackgroundURL)
	if err != nil {
		h.handleThemeErr(w, r, err)
		return
	}
	w.WriteHeader(201)
	render.JSON(w, r, t)
}

func (h *Handler) UpdateTheme(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, r, 400, "invalid theme id")
		return
	}
	var req themeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, 400, "invalid request body")
		return
	}
	if len(req.Name) == 0 || len(req.Name) > 64 {
		writeErr(w, r, 400, "name must be 1-64 characters")
		return
	}
	if req.BaseTheme == "" {
		req.BaseTheme = "rory"
	}
	if !validBuiltin[req.BaseTheme] {
		writeErr(w, r, 400, "base_theme must be a built-in theme")
		return
	}
	t, err := h.svc.UpdateTheme(r.Context(), id, uid, req.Name, req.CSS, req.BaseTheme, req.BackgroundURL)
	if err != nil {
		h.handleThemeErr(w, r, err)
		return
	}
	render.JSON(w, r, t)
}

func (h *Handler) DeleteTheme(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, r, 400, "invalid theme id"); return
	}
	if err := h.svc.DeleteTheme(r.Context(), id, uid); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeErr(w, r, 404, "theme not found")
			return
		}
		writeErr(w, r, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (h *Handler) ShareTheme(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, r, 400, "invalid theme id"); return
	}
	shareURL, err := h.svc.ShareTheme(r.Context(), id, uid)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeErr(w, r, 404, "theme not found")
			return
		}
		writeErr(w, r, 500, err.Error())
		return
	}
	render.JSON(w, r, map[string]string{"share_url": shareURL})
}

func (h *Handler) GetPublicTheme(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.GetPublicTheme(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeErr(w, r, 404, "theme not found")
			return
		}
		writeErr(w, r, 500, err.Error())
		return
	}
	render.JSON(w, r, t)
}

func (h *Handler) InstallTheme(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	t, err := h.svc.InstallTheme(r.Context(), chi.URLParam(r, "token"), uid)
	if err != nil {
		h.handleThemeErr(w, r, err)
		return
	}
	w.WriteHeader(201)
	render.JSON(w, r, t)
}

// GetThemeRepo handles GET /api/themes/repo — public, no auth required.
func (h *Handler) GetThemeRepo(w http.ResponseWriter, r *http.Request) {
	limit := 24
	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	offset := (page - 1) * limit
	themes, total, err := h.svc.GetPublishedThemes(r.Context(), limit, offset)
	if err != nil {
		writeErr(w, r, 500, err.Error())
		return
	}
	render.JSON(w, r, ThemeRepoResponse{Themes: themes, Total: total})
}

// TogglePublish handles POST /api/me/themes/{id}/publish — requires auth.
func (h *Handler) TogglePublish(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, r, 400, "invalid theme id")
		return
	}
	var req publishReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, 400, "invalid request body")
		return
	}
	if err := h.svc.SetPublished(r.Context(), id, uid, req.Published); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeErr(w, r, 404, "theme not found")
			return
		}
		writeErr(w, r, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

// ToggleFeature handles PUT /api/themes/{id}/feature — requires auth + Parley Admin badge.
func (h *Handler) ToggleFeature(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized")
		return
	}
	isAdmin, err := h.svc.IsParleyAdmin(r.Context(), uid)
	if err != nil {
		writeErr(w, r, 500, err.Error())
		return
	}
	if !isAdmin {
		writeErr(w, r, 403, "requires Parley Admin badge")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, r, 400, "invalid theme id")
		return
	}
	var req featureReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, 400, "invalid request body")
		return
	}
	if err := h.svc.SetFeatured(r.Context(), id, req.Featured); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeErr(w, r, 404, "theme not found")
			return
		}
		writeErr(w, r, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (h *Handler) handleThemeErr(w http.ResponseWriter, r *http.Request, err error) {
	var ve *ValidationError
	if errors.As(err, &ve) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(400)
		render.JSON(w, r, map[string]interface{}{
			"error":          "theme contains disallowed external URLs",
			"offending_urls": ve.OffendingURLs,
		})
		return
	}
	if errors.Is(err, ErrNotFound) {
		writeErr(w, r, 404, "theme not found")
		return
	}
	if errors.Is(err, ErrThemeLimit) {
		writeErr(w, r, 400, err.Error())
		return
	}
	writeErr(w, r, 500, err.Error())
}
