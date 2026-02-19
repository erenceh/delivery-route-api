package handlers

import (
	"delivery-route-service/internal/api/dto"
	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/ports"
	"delivery-route-service/internal/services"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type PlanHandler struct {
	Repo       ports.PackageRepository
	Provider   ports.DistanceProvider
	DefaultHub string
}

// Plan orchestrates package assignment and route planning for all trucks.
// It coordinates repository access, assignment heuristics, and route computation.
func (h *PlanHandler) Plan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req dto.PlanRequest

	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()
	dec.DisallowUnknownFields()

	if err := dec.Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, r, http.StatusBadRequest, "body must contain only one JSON object")
		return
	}

	hub := strings.TrimSpace(req.Hub)
	if hub == "" {
		hub = strings.TrimSpace(h.DefaultHub)
	}
	if hub == "" {
		writeError(w, r, http.StatusBadRequest, "hub is required")
		return
	}

	// Apply defaults when request fields are omitted.
	depart := time.Now()
	if req.DepartAt != nil {
		depart = *req.DepartAt
	}

	truckCount := req.TruckCount
	if truckCount == 0 {
		truckCount = 3
	}
	if truckCount < 1 || truckCount > 10 {
		writeError(w, r, http.StatusBadRequest, "truck_count must be between 1 and 10")
		return
	}

	truckCap := req.TruckCapacity
	if truckCap == 0 {
		truckCap = 16
	}
	if truckCap < 1 || truckCap > 100 {
		writeError(w, r, http.StatusBadRequest, "truck_capacity must be between 1 and 100")
		return
	}

	if h.Repo == nil {
		log.Printf("PlanHandler Repo must not be nil")
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}

	pkgs, err := h.Repo.ListPackages()
	if err != nil {
		log.Printf("list packages failed: %v", err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}

	trucks := make([]*domain.Truck, 0, truckCount)
	for i := 0; i < truckCount; i++ {
		trucks = append(trucks, &domain.Truck{
			TruckID:       i + 1,
			Capacity:      truckCap,
			StartLocation: hub,
		})
	}

	if h.Provider == nil {
		log.Printf("PlanHandler Provider must not be nil")
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}

	// Assign packages to trucks before computing individual routes.
	if err := services.AssignPackagesByDistance(r.Context(), pkgs, trucks, hub, h.Provider); err != nil {
		log.Printf("failed to assign packages: %v", err)
		writeError(w, r, http.StatusInternalServerError, "internal server error")
		return
	}

	// Compute and apply a route plan per truck
	plans := make([]*domain.RoutePlan, 0, len(trucks))
	for _, t := range trucks {
		plan, err := services.PlanTruckRoute(r.Context(), t, depart, h.Provider, req.ReturnToStart)
		if err != nil {
			log.Printf("failed to plan truck route: %v", err)
			writeError(w, r, http.StatusInternalServerError, "internal server error")
			return
		}

		if err := t.ApplyPlan(plan); err != nil {
			log.Printf("failed to apply plan: %v", err)
			writeError(w, r, http.StatusInternalServerError, "internal server error")
			return
		}

		plans = append(plans, plan)
	}

	res := dto.ListPlanResponse{Plans: make([]dto.PlanResponse, 0, len(plans))}
	for _, p := range plans {
		stops := make([]dto.PlanStopResponse, 0, len(p.Stops))
		for _, s := range p.Stops {
			stops = append(stops, dto.PlanStopResponse{
				Destination: s.Destination,
				ArriveAt:    s.ArriveAt,
				PackageIDs:  s.PackageIDs,
			})
		}

		res.Plans = append(res.Plans, dto.PlanResponse{
			TruckID:              p.TruckID,
			DepartAt:             p.DepartAt,
			TotalDistanceMeters:  p.TotalDistanceMeters,
			TotalDurationSeconds: p.TotalDurationSeconds,
			Stops:                stops,
		})
	}

	writeJSON(w, r, http.StatusOK, res)
}
