package httputil

import (
	"encoding/json"
	"net/http"
)

// JSONError writes a JSON error response: {"message": "<msg>"}.
// All API handlers should use this instead of http.Error so clients always
// receive a consistent JSON body regardless of which package the error originates in.
func JSONError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"message": message}) //nolint:errcheck
}
