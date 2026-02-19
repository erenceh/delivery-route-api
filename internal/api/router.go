package api

import (
	"delivery-route-service/internal/api/handlers"
	"delivery-route-service/internal/ports"
	"net/http"
)

// NewRouter wires HTTP handlers with their dependencies and returns an http.Handler.
// This is the API composition root (handlers stay unaware of concrete adapters).
func NewRouter(repo ports.PackageRepository, provider ports.DistanceProvider, hub string) http.Handler {
	mux := http.NewServeMux()

	pkgHandler := &handlers.PackageHandler{Repo: repo}
	planHandler := &handlers.PlanHandler{
		Repo:       repo,
		Provider:   provider,
		DefaultHub: hub,
	}

	mux.HandleFunc("/health", handlers.Health)
	mux.HandleFunc("/packages", pkgHandler.List)
	mux.HandleFunc("/plans", planHandler.Plan)

	return loggingMiddleware(mux)
}
