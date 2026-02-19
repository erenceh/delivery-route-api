package handlers

import (
	"net/http"
)

// Health provides a minimal liveness check endpoint.
func Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	res := map[string]string{"status": "ok"}
	writeJSON(w, r, http.StatusOK, res)
}
