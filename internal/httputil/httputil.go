package httputil

import (
	"encoding/json"
	"log"
	"net/http"
)

// JSONError writes a JSON error response: {"message": "<msg>"}.
// All API handlers should use this instead of http.Error so clients always
// receive a consistent JSON body regardless of which package the error originates in.
// InternalError logs the real error server-side and returns a generic 500 to the client.
// Use this instead of JSONError for unexpected/internal errors to avoid leaking
// database driver strings, file paths, or other internal details to attackers.
func InternalError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	JSONError(w, "internal server error", http.StatusInternalServerError)
}

func JSONError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"message": message}) //nolint:errcheck
}
