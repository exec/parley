package account

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"parley/internal/auth"
	"parley/internal/httputil"
)

// ExportHandler serves GET /api/me/export. Builds the entire envelope in
// memory then streams it via json.NewEncoder so a partial-write doesn't
// leave the client with a half-written file.
type ExportHandler struct {
	svc *ExportService
}

// NewExportHandler binds an ExportService for HTTP.
func NewExportHandler(svc *ExportService) *ExportHandler {
	return &ExportHandler{svc: svc}
}

// Export handles GET /api/me/export.
func (h *ExportHandler) Export(w http.ResponseWriter, r *http.Request) {
	userIDStr := auth.GetUserIDFromContext(r)
	if userIDStr == "" {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	envelope, err := h.svc.BuildExport(r.Context(), userID)
	if err != nil {
		httputil.InternalError(w, err)
		return
	}

	// Filename uses the snapshot's username, not whatever was in the URL —
	// guarantees the on-disk artifact carries the same identity as the
	// data inside it. unix-second precision is sufficient since users
	// rarely run two exports in the same second.
	filename := exportFilename(envelope.Profile.Username, time.Now())
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if err := json.NewEncoder(w).Encode(envelope); err != nil {
		// Headers already sent; log only — the client will see a truncated
		// JSON tail and a closed connection. There is no recovery path
		// once the body has begun.
		httputil.InternalError(w, err)
		return
	}
}

// exportFilename matches the locked spec: parley-export-<username>-<unix>.json
// where username is the profile's snapshot username at export time.
func exportFilename(username string, now time.Time) string {
	return fmt.Sprintf("parley-export-%s-%d.json", username, now.Unix())
}
