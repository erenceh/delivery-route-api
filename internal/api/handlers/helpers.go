package handlers

import (
	"encoding/json"
	"log"
	"net/http"
)

func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode failed: method=%s path=%s err=%v", r.Method, r.URL.Path, err)
	}
}

func writeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	writeJSON(w, r, status, map[string]string{"error": msg})
}
