package conductor

import (
	"encoding/json"
	"net/http"
)

// WriteJSON encodes v as JSON with the given HTTP status. Exported so
// the sub-package pkg/conductor/native can share the same shape
// without duplicating the function.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteErr is the convention for surfacing an error to the SPA:
// {"error": "<message>"} at the requested status.
func WriteErr(w http.ResponseWriter, status int, err error) {
	WriteJSON(w, status, map[string]string{"error": err.Error()})
}
