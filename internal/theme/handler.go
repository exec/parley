package theme

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"parley/internal/auth"
	"parley/internal/httputil"
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
	Message       string   `json:"message"`
	OffendingURLs []string `json:"offending_urls,omitempty"`
}

type publishReq struct {
	Published bool `json:"published"`
}

type featureReq struct {
	Featured bool `json:"featured"`
}

func userID(r *http.Request) (int64, bool) {
	s := auth.GetUserIDFromContext(r)
	if s == "" {
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
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
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
		httputil.InternalError(w, err)
		return
	}
	render.JSON(w, r, p)
}

func (h *Handler) SetActiveTheme(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req setThemeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Theme != "custom" && !validBuiltin[req.Theme] {
		httputil.JSONError(w, "unknown theme ID", http.StatusBadRequest)
		return
	}
	if req.Theme == "custom" && req.CustomThemeID == nil {
		httputil.JSONError(w, "custom_theme_id required when theme is 'custom'", http.StatusBadRequest)
		return
	}
	if err := h.svc.SetActiveTheme(r.Context(), uid, req.Theme, req.CustomThemeID); err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.JSONError(w, "theme not found", http.StatusNotFound)
			return
		}
		httputil.InternalError(w, err)
		return
	}
	w.WriteHeader(204)
}

func (h *Handler) CreateTheme(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req themeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Name) == 0 || len(req.Name) > 64 {
		httputil.JSONError(w, "name must be 1-64 characters", http.StatusBadRequest)
		return
	}
	if req.BaseTheme == "" {
		req.BaseTheme = "rory"
	}
	if !validBuiltin[req.BaseTheme] {
		httputil.JSONError(w, "base_theme must be a built-in theme", http.StatusBadRequest)
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
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid theme id", http.StatusBadRequest)
		return
	}
	var req themeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Name) == 0 || len(req.Name) > 64 {
		httputil.JSONError(w, "name must be 1-64 characters", http.StatusBadRequest)
		return
	}
	if req.BaseTheme == "" {
		req.BaseTheme = "rory"
	}
	if !validBuiltin[req.BaseTheme] {
		httputil.JSONError(w, "base_theme must be a built-in theme", http.StatusBadRequest)
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
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid theme id", http.StatusBadRequest)
		return
	}
	if err := h.svc.DeleteTheme(r.Context(), id, uid); err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.JSONError(w, "theme not found", http.StatusNotFound)
			return
		}
		httputil.InternalError(w, err)
		return
	}
	w.WriteHeader(204)
}

func (h *Handler) ShareTheme(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid theme id", http.StatusBadRequest)
		return
	}
	shareURL, err := h.svc.ShareTheme(r.Context(), id, uid)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.JSONError(w, "theme not found", http.StatusNotFound)
			return
		}
		httputil.InternalError(w, err)
		return
	}
	render.JSON(w, r, map[string]string{"share_url": shareURL})
}

func (h *Handler) GetPublicTheme(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.GetPublicTheme(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.JSONError(w, "theme not found", http.StatusNotFound)
			return
		}
		httputil.InternalError(w, err)
		return
	}
	// Public shared theme — safe to cache briefly (token is immutable content).
	w.Header().Set("Cache-Control", "public, max-age=60")
	render.JSON(w, r, t)
}

func (h *Handler) InstallTheme(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
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
		httputil.InternalError(w, err)
		return
	}
	// Public repo listing — cache briefly so rapid page loads don't hammer the DB.
	w.Header().Set("Cache-Control", "public, max-age=30")
	render.JSON(w, r, ThemeRepoResponse{Themes: themes, Total: total})
}

// TogglePublish handles POST /api/me/themes/{id}/publish — requires auth.
func (h *Handler) TogglePublish(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid theme id", http.StatusBadRequest)
		return
	}
	var req publishReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.svc.SetPublished(r.Context(), id, uid, req.Published); err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.JSONError(w, "theme not found", http.StatusNotFound)
			return
		}
		httputil.InternalError(w, err)
		return
	}
	w.WriteHeader(204)
}

// ToggleFeature handles PUT /api/themes/{id}/feature — requires auth + Parley Admin badge.
func (h *Handler) ToggleFeature(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	isAdmin, err := h.svc.IsParleyAdmin(r.Context(), uid)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	if !isAdmin {
		httputil.JSONError(w, "requires Parley Admin badge", http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid theme id", http.StatusBadRequest)
		return
	}
	var req featureReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.svc.SetFeatured(r.Context(), id, req.Featured); err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.JSONError(w, "theme not found", http.StatusNotFound)
			return
		}
		httputil.InternalError(w, err)
		return
	}
	w.WriteHeader(204)
}

func (h *Handler) handleThemeErr(w http.ResponseWriter, r *http.Request, err error) {
	var ve *ValidationError
	if errors.As(err, &ve) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(400)
		render.JSON(w, r, errResp{
			Message:       "theme contains disallowed external URLs",
			OffendingURLs: ve.OffendingURLs,
		})
		return
	}
	if errors.Is(err, ErrNotFound) {
		httputil.JSONError(w, "theme not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, ErrThemeLimit) {
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if errors.Is(err, ErrAlreadyInstalled) {
		httputil.JSONError(w, "theme already installed", http.StatusConflict)
		return
	}
	httputil.InternalError(w, err)
}
