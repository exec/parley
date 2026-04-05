package bin

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

// Handler handles HTTP requests for bin posts, versions, line comments, and tags.
type Handler struct {
	service *Service
}

// NewHandler creates a new bin Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// ---- Posts ----

type createPostRequest struct {
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Tags        []string         `json:"tags"`
	Files       []db.BinPostFile `json:"files"`
}

// CreatePost handles POST /channels/{channelID}/posts
func (h *Handler) CreatePost(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		httputil.JSONError(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	var req createPostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Title == "" {
		httputil.JSONError(w, "title is required", http.StatusBadRequest)
		return
	}

	post, err := h.service.CreatePost(r.Context(), channelID, userID, req.Title, req.Description, req.Tags, req.Files)
	if err != nil {
		if errors.Is(err, ErrChannelNotFound) {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
		} else if errors.Is(err, ErrNotBinChannel) {
			httputil.JSONError(w, err.Error(), http.StatusBadRequest)
		} else if errors.Is(err, ErrForbidden) {
			httputil.JSONError(w, "you do not have permission to create posts in this channel", http.StatusForbidden)
		} else {
			httputil.InternalError(w, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(post)
}

// ListPosts handles GET /channels/{channelID}/posts
func (h *Handler) ListPosts(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		httputil.JSONError(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()
	tag := q.Get("tag")
	language := q.Get("language")
	authorID := q.Get("author_id")
	sort := q.Get("sort")

	limit := 25
	offset := 0
	if l := q.Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			if v > 100 {
				v = 100
			}
			limit = v
		}
	}
	if o := q.Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	userID := auth.GetUserIDFromContext(r)
	posts, err := h.service.ListPosts(r.Context(), channelID, userID, tag, language, authorID, sort, limit, offset)
	if err != nil {
		if errors.Is(err, ErrChannelNotFound) {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
		} else if errors.Is(err, ErrNotBinChannel) {
			httputil.JSONError(w, err.Error(), http.StatusBadRequest)
		} else {
			httputil.InternalError(w, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(posts)
}

// GetPost handles GET /posts/{postID}
func (h *Handler) GetPost(w http.ResponseWriter, r *http.Request) {
	postID := chi.URLParam(r, "postID")
	if postID == "" {
		httputil.JSONError(w, "post ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	post, err := h.service.GetPost(r.Context(), postID, userID)
	if err != nil {
		if errors.Is(err, ErrPostNotFound) {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
			return
		}
		httputil.InternalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}

type editPostRequest struct {
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Tags        []string         `json:"tags"`
	Files       []db.BinPostFile `json:"files"`
}

// EditPost handles PUT /posts/{postID}
func (h *Handler) EditPost(w http.ResponseWriter, r *http.Request) {
	postID := chi.URLParam(r, "postID")
	if postID == "" {
		httputil.JSONError(w, "post ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	var req editPostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Title == "" {
		httputil.JSONError(w, "title is required", http.StatusBadRequest)
		return
	}

	post, err := h.service.EditPost(r.Context(), postID, userID, req.Title, req.Description, req.Tags, req.Files)
	if err != nil {
		if errors.Is(err, ErrPostNotFound) {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
		} else if errors.Is(err, ErrForbidden) {
			httputil.JSONError(w, err.Error(), http.StatusForbidden)
		} else {
			httputil.InternalError(w, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}

// DeletePost handles DELETE /posts/{postID}
func (h *Handler) DeletePost(w http.ResponseWriter, r *http.Request) {
	postID := chi.URLParam(r, "postID")
	if postID == "" {
		httputil.JSONError(w, "post ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	if err := h.service.DeletePost(r.Context(), postID, userID); err != nil {
		if errors.Is(err, ErrPostNotFound) {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
		} else if errors.Is(err, ErrForbidden) {
			httputil.JSONError(w, err.Error(), http.StatusForbidden)
		} else {
			httputil.InternalError(w, err)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---- Versions ----

// GetVersions handles GET /posts/{postID}/versions
func (h *Handler) GetVersions(w http.ResponseWriter, r *http.Request) {
	postID := chi.URLParam(r, "postID")
	if postID == "" {
		httputil.JSONError(w, "post ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	versions, err := h.service.GetVersions(r.Context(), postID, userID)
	if err != nil {
		if errors.Is(err, ErrPostNotFound) {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
		} else if errors.Is(err, ErrForbidden) {
			httputil.JSONError(w, "you do not have permission to view this channel", http.StatusForbidden)
		} else {
			httputil.InternalError(w, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(versions)
}

// GetVersion handles GET /posts/{postID}/versions/{versionID}
func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	versionID := chi.URLParam(r, "versionID")
	if versionID == "" {
		httputil.JSONError(w, "version ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	version, err := h.service.GetVersion(r.Context(), versionID, userID)
	if err != nil {
		if errors.Is(err, ErrVersionNotFound) {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
		} else if errors.Is(err, ErrForbidden) {
			httputil.JSONError(w, "you do not have permission to view this channel", http.StatusForbidden)
		} else {
			httputil.InternalError(w, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(version)
}

// ---- Line Comments ----

type createLineCommentRequest struct {
	VersionID  int64  `json:"version_id"`
	FileID     int64  `json:"file_id"`
	LineNumber int    `json:"line_number"`
	Content    string `json:"content"`
	ParentID   string `json:"parent_id"`
}

// CreateLineComment handles POST /posts/{postID}/line-comments
func (h *Handler) CreateLineComment(w http.ResponseWriter, r *http.Request) {
	postID := chi.URLParam(r, "postID")
	if postID == "" {
		httputil.JSONError(w, "post ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	var req createLineCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Content == "" {
		httputil.JSONError(w, "content is required", http.StatusBadRequest)
		return
	}
	if req.VersionID == 0 {
		httputil.JSONError(w, "version_id is required", http.StatusBadRequest)
		return
	}
	if req.FileID == 0 {
		httputil.JSONError(w, "file_id is required", http.StatusBadRequest)
		return
	}

	comment, err := h.service.CreateLineComment(r.Context(), postID, userID,
		strconv.FormatInt(req.VersionID, 10), strconv.FormatInt(req.FileID, 10),
		req.LineNumber, req.Content, req.ParentID)
	if err != nil {
		if errors.Is(err, ErrPostNotFound) {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
		} else if errors.Is(err, ErrForbidden) {
			httputil.JSONError(w, err.Error(), http.StatusForbidden)
		} else {
			httputil.InternalError(w, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(comment)
}

// GetLineComments handles GET /posts/{postID}/line-comments
func (h *Handler) GetLineComments(w http.ResponseWriter, r *http.Request) {
	postID := chi.URLParam(r, "postID")
	if postID == "" {
		httputil.JSONError(w, "post ID is required", http.StatusBadRequest)
		return
	}

	var versionID, fileID *string
	if v := r.URL.Query().Get("version_id"); v != "" {
		versionID = &v
	}
	if f := r.URL.Query().Get("file_id"); f != "" {
		fileID = &f
	}

	commentUserID := auth.GetUserIDFromContext(r)
	comments, err := h.service.GetLineComments(r.Context(), postID, commentUserID, versionID, fileID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comments)
}

type updateLineCommentRequest struct {
	Content string `json:"content"`
}

// UpdateLineComment handles PUT /line-comments/{id}
func (h *Handler) UpdateLineComment(w http.ResponseWriter, r *http.Request) {
	commentID := chi.URLParam(r, "id")
	if commentID == "" {
		httputil.JSONError(w, "comment ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	var req updateLineCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Content == "" {
		httputil.JSONError(w, "content is required", http.StatusBadRequest)
		return
	}

	comment, err := h.service.UpdateLineComment(r.Context(), commentID, userID, req.Content)
	if err != nil {
		if errors.Is(err, ErrCommentNotFound) {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
		} else if errors.Is(err, ErrForbidden) {
			httputil.JSONError(w, err.Error(), http.StatusForbidden)
		} else {
			httputil.InternalError(w, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comment)
}

// DeleteLineComment handles DELETE /line-comments/{id}
func (h *Handler) DeleteLineComment(w http.ResponseWriter, r *http.Request) {
	commentID := chi.URLParam(r, "id")
	if commentID == "" {
		httputil.JSONError(w, "comment ID is required", http.StatusBadRequest)
		return
	}

	userID := auth.GetUserIDFromContext(r)
	if userID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	if err := h.service.DeleteLineComment(r.Context(), commentID, userID); err != nil {
		if errors.Is(err, ErrCommentNotFound) {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
		} else if errors.Is(err, ErrForbidden) {
			httputil.JSONError(w, err.Error(), http.StatusForbidden)
		} else {
			httputil.InternalError(w, err)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---- Tags ----

type createTagRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// CreateTag handles POST /channels/{channelID}/tags
func (h *Handler) CreateTag(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		httputil.JSONError(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	tagUserID := auth.GetUserIDFromContext(r)
	if tagUserID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	var req createTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		httputil.JSONError(w, "name is required", http.StatusBadRequest)
		return
	}

	tag, err := h.service.CreateTag(r.Context(), channelID, tagUserID, req.Name, req.Color)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			httputil.JSONError(w, err.Error(), http.StatusForbidden)
			return
		}
		httputil.InternalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(tag)
}

// GetTags handles GET /channels/{channelID}/tags
func (h *Handler) GetTags(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		httputil.JSONError(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	tags, err := h.service.GetTags(r.Context(), channelID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tags)
}

// DeleteTag handles DELETE /channels/{channelID}/tags/{tagID}
func (h *Handler) DeleteTag(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "tagID")
	if tagID == "" {
		httputil.JSONError(w, "tag ID is required", http.StatusBadRequest)
		return
	}

	deleteTagUserID := auth.GetUserIDFromContext(r)
	if deleteTagUserID == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	if err := h.service.DeleteTag(r.Context(), tagID, deleteTagUserID); err != nil {
		if errors.Is(err, ErrTagNotFound) {
			httputil.JSONError(w, err.Error(), http.StatusNotFound)
		} else if errors.Is(err, ErrForbidden) {
			httputil.JSONError(w, err.Error(), http.StatusForbidden)
		} else {
			httputil.InternalError(w, err)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
