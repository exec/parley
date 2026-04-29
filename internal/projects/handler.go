package projects

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
)

// Handler is the HTTP layer for project endpoints.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// ---- payloads ----

type repoPayload struct {
	Provider string `json:"provider"`
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
}

type createProjectRequest struct {
	ServerID    string        `json:"server_id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	ClaudeMD    string        `json:"claude_md"`
	SkillLevel  string        `json:"skill_level"`
	PresetSlug  string        `json:"preset_slug,omitempty"`
	VCChannelID *string       `json:"vc_channel_id,omitempty"`
	Repos       []repoPayload `json:"repos,omitempty"`
	Skills      []string      `json:"skills,omitempty"`
}

type updateProjectRequest struct {
	Name        *string        `json:"name,omitempty"`
	Description *string        `json:"description,omitempty"`
	SkillLevel  *string        `json:"skill_level,omitempty"`
	PresetSlug  *string        `json:"preset_slug,omitempty"` // string == "" → clear
	VCChannelID *string        `json:"vc_channel_id,omitempty"` // string == "" → clear
	Repos       *[]repoPayload `json:"repos,omitempty"`
	Skills      *[]string      `json:"skills,omitempty"`
}

type updateClaudeMDRequest struct {
	Content string `json:"content"`
}

// writeServiceError maps a service error to an HTTP status.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrProjectNotFound), errors.Is(err, ErrServerNotFound):
		httputil.JSONError(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrPresetNotFound):
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrNotMember):
		httputil.JSONError(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, ErrForbidden):
		httputil.JSONError(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, ErrInvalidInput):
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
	default:
		httputil.InternalError(w, err)
	}
}

// CreateProject handles POST /projects
func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	serverID, err := strconv.ParseInt(req.ServerID, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid server_id", http.StatusBadRequest)
		return
	}
	var vcChannelID *int64
	if req.VCChannelID != nil && *req.VCChannelID != "" {
		v, err := strconv.ParseInt(*req.VCChannelID, 10, 64)
		if err != nil {
			httputil.JSONError(w, "invalid vc_channel_id", http.StatusBadRequest)
			return
		}
		vcChannelID = &v
	}
	repos := make([]db.ProjectRepo, 0, len(req.Repos))
	for _, rp := range req.Repos {
		repos = append(repos, db.ProjectRepo{Provider: rp.Provider, Owner: rp.Owner, Repo: rp.Repo})
	}

	p, err := h.service.CreateProject(r.Context(), userID, CreateInput{
		ServerID:    serverID,
		Name:        req.Name,
		Description: req.Description,
		ClaudeMD:    req.ClaudeMD,
		SkillLevel:  req.SkillLevel,
		PresetSlug:  req.PresetSlug,
		VCChannelID: vcChannelID,
		Repos:       repos,
		Skills:      req.Skills,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

// GetServerProjects handles GET /servers/{id}/projects
func (h *Handler) GetServerProjects(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	serverID := chi.URLParam(r, "id")
	out, err := h.service.ListServerProjects(r.Context(), serverID, userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// GetProject handles GET /projects/{id}
func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	projectID := chi.URLParam(r, "id")
	p, err := h.service.GetProject(r.Context(), projectID, userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

// UpdateProject handles PATCH /projects/{id}
func (h *Handler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	projectID := chi.URLParam(r, "id")
	var req updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var in UpdateInput
	if req.Name != nil {
		in.Name = *req.Name
	}
	// Description: allow clearing to "". Sentinel: caller omits the field
	// entirely → leave alone; explicit empty string → set to "".
	if req.Description != nil {
		in.Description = *req.Description
	} else {
		in.Description = "__SKIP__"
	}
	if req.SkillLevel != nil {
		in.SkillLevel = *req.SkillLevel
	}
	if req.PresetSlug != nil {
		if *req.PresetSlug == "" {
			in.PresetSlug = "__CLEAR__"
		} else {
			in.PresetSlug = *req.PresetSlug
		}
	}
	if req.VCChannelID != nil {
		if *req.VCChannelID == "" {
			in.ClearVC = true
		} else {
			v, err := strconv.ParseInt(*req.VCChannelID, 10, 64)
			if err != nil {
				httputil.JSONError(w, "invalid vc_channel_id", http.StatusBadRequest)
				return
			}
			in.VCChannelID = &v
		}
	}
	if req.Repos != nil {
		conv := make([]db.ProjectRepo, 0, len(*req.Repos))
		for _, rp := range *req.Repos {
			conv = append(conv, db.ProjectRepo{Provider: rp.Provider, Owner: rp.Owner, Repo: rp.Repo})
		}
		in.Repos = &conv
	}
	if req.Skills != nil {
		in.Skills = req.Skills
	}

	p, err := h.service.UpdateProject(r.Context(), projectID, userID, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

// UpdateClaudeMD handles PATCH /projects/{id}/claude-md
func (h *Handler) UpdateClaudeMD(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	projectID := chi.URLParam(r, "id")
	var req updateClaudeMDRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	p, _, err := h.service.UpdateClaudeMD(r.Context(), projectID, userID, req.Content)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

// DeleteProject handles DELETE /projects/{id}
func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	projectID := chi.URLParam(r, "id")
	if err := h.service.DeleteProject(r.Context(), projectID, userID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListPresets handles GET /projects/presets
func (h *Handler) ListPresets(w http.ResponseWriter, r *http.Request) {
	if auth.GetUserIDFromContext(r) == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	out, err := h.service.ListPresets(r.Context())
	if err != nil {
		httputil.InternalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// GetClaudeMDVersions handles GET /projects/{id}/claude-md/versions
func (h *Handler) GetClaudeMDVersions(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	projectID := chi.URLParam(r, "id")
	out, err := h.service.GetClaudeMDVersions(r.Context(), projectID, userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
