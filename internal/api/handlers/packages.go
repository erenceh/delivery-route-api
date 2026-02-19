package handlers

import (
	"delivery-route-service/internal/api/dto"
	"delivery-route-service/internal/ports"
	"log"
	"net/http"
)

// PackageHandler exposes read-only package retrieval endpoints.
type PackageHandler struct {
	Repo ports.PackageRepository
}

func (h *PackageHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	pkgs, err := h.Repo.ListPackages()
	if err != nil {
		log.Printf("list packages failed: %v", err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}

	res := dto.ListPackagesResponse{
		Packages: make([]dto.PackageResponse, 0, len(pkgs)),
	}
	for _, p := range pkgs {
		res.Packages = append(res.Packages, dto.PackageResponse{
			PackageID:   p.PackageID,
			Destination: p.Destination,
			LoadedAt:    p.LoadedAt,
			DeliveredAt: p.DeliveredAt,
		})
	}

	writeJSON(w, r, http.StatusOK, res)
}
