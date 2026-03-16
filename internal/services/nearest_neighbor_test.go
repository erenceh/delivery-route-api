package services_test

import (
	"context"
	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/ports"
	"delivery-route-service/internal/services"
	"strings"
	"testing"
	"time"
)

func TestNearestNeighbor(t *testing.T) {
	pkgs := []*domain.Package{
		{PackageID: 1, Destination: "DestA"},
		{PackageID: 2, Destination: "DestB"},
	}

	tests := []struct {
		name              string
		truck             *domain.Truck
		departAt          time.Time
		distances         map[string]ports.DistanceResult
		returnToStart     bool
		wantErr           bool
		errContains       string
		wantEmptyPlan     *domain.RoutePlan
		wantStopOrder     []string
		wantFirstAddress  string
		wantTotalDistance int
		wantTotalDuration int
	}{
		{
			name:     "Error when startLocation is empty",
			truck:    domain.NewTruck(1, 1, ""),
			departAt: time.Now(),
			distances: map[string]ports.DistanceResult{
				"HUB|DestA": {DistanceMeters: 100, DurationSeconds: 60},
				"DestA|HUB": {DistanceMeters: 100, DurationSeconds: 60},
			},
			returnToStart: true,
			wantErr:       true,
			errContains:   "startLocation",
		},
		{
			name:     "Empty plan returned when no packages",
			truck:    domain.NewTruck(1, 1, "HUB"),
			departAt: time.Now(),
			distances: map[string]ports.DistanceResult{
				"HUB|DestA": {DistanceMeters: 100, DurationSeconds: 60},
				"DestA|HUB": {DistanceMeters: 100, DurationSeconds: 60},
			},
			returnToStart: true,
			wantEmptyPlan: &domain.RoutePlan{
				TruckID:              1,
				DepartAt:             time.Now(),
				Stops:                []domain.RouteStop{},
				TotalDurationSeconds: 0,
				TotalDistanceMeters:  0,
			},
		},
		{
			name: "Nearest duration stop selected first (greedy step)",
			truck: &domain.Truck{
				TruckID:       1,
				Capacity:      2,
				StartLocation: "HUB",
				Packages:      pkgs,
			},
			departAt: time.Now(),
			distances: map[string]ports.DistanceResult{
				"HUB|DestA":   {DistanceMeters: 100, DurationSeconds: 60},
				"HUB|DestB":   {DistanceMeters: 200, DurationSeconds: 120},
				"DestA|DestB": {DistanceMeters: 100, DurationSeconds: 60},
				"DestB|HUB":   {DistanceMeters: 200, DurationSeconds: 120},
				"DestA|HUB":   {DistanceMeters: 100, DurationSeconds: 60},
			},
			returnToStart: true,
			wantStopOrder: []string{"DestA", "DestB"},
		},
		{
			name: "Tie-breaker selects alphabetically first address when durations are equal",
			truck: &domain.Truck{
				TruckID:       1,
				Capacity:      2,
				StartLocation: "HUB",
				Packages:      pkgs,
			},
			departAt: time.Now(),
			distances: map[string]ports.DistanceResult{
				"HUB|DestA":   {DistanceMeters: 100, DurationSeconds: 60},
				"HUB|DestB":   {DistanceMeters: 100, DurationSeconds: 60},
				"DestA|DestB": {DistanceMeters: 0, DurationSeconds: 0},
				"DestB|HUB":   {DistanceMeters: 100, DurationSeconds: 60},
				"DestA|HUB":   {DistanceMeters: 100, DurationSeconds: 60},
			},
			returnToStart:    true,
			wantFirstAddress: "DestA",
		},
		{
			name: "returnToStart adds return leg to totals",
			truck: &domain.Truck{
				TruckID:       1,
				Capacity:      2,
				StartLocation: "HUB",
				Packages:      pkgs,
			},
			departAt: time.Now(),
			distances: map[string]ports.DistanceResult{
				"HUB|DestA":   {DistanceMeters: 100, DurationSeconds: 60},
				"HUB|DestB":   {DistanceMeters: 200, DurationSeconds: 120},
				"DestA|DestB": {DistanceMeters: 100, DurationSeconds: 60},
				"DestB|HUB":   {DistanceMeters: 200, DurationSeconds: 120},
				"DestA|HUB":   {DistanceMeters: 100, DurationSeconds: 60},
			},
			returnToStart:     true,
			wantTotalDistance: 400,
			wantTotalDuration: 240,
		},
		{
			name: "Total distance and duration sum correctly across stops",
			truck: &domain.Truck{
				TruckID:       1,
				Capacity:      2,
				StartLocation: "HUB",
				Packages:      pkgs,
			},
			departAt: time.Now(),
			distances: map[string]ports.DistanceResult{
				"HUB|DestA":   {DistanceMeters: 100, DurationSeconds: 60},
				"HUB|DestB":   {DistanceMeters: 200, DurationSeconds: 120},
				"DestA|DestB": {DistanceMeters: 100, DurationSeconds: 60},
				"DestB|HUB":   {DistanceMeters: 200, DurationSeconds: 120},
				"DestA|HUB":   {DistanceMeters: 100, DurationSeconds: 60},
			},
			returnToStart:     false,
			wantTotalDistance: 200,
			wantTotalDuration: 120,
		},
		{
			name: "Error when distance missing for a leg",
			truck: &domain.Truck{
				TruckID:       1,
				Capacity:      2,
				StartLocation: "HUB",
				Packages:      pkgs,
			},
			departAt: time.Now(),
			distances: map[string]ports.DistanceResult{
				"HUB|DestA":   {DistanceMeters: 100, DurationSeconds: 60},
				"HUB|DestB":   {DistanceMeters: 100, DurationSeconds: 60},
				"DestA|DestB": {DistanceMeters: 0, DurationSeconds: 0},
			},
			returnToStart: true,
			wantErr:       true,
			errContains:   "missing distance",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := services.NearestNeighborRoute(context.Background(), tc.truck, tc.departAt, tc.distances, tc.returnToStart)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}

				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("expected error containing %q, got %q", tc.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantEmptyPlan != nil {
				if len(plan.Stops) != 0 {
					t.Fatalf("expected empty plan, got %d stops", len(plan.Stops))
				}

				if plan.TotalDistanceMeters != 0 || plan.TotalDurationSeconds != 0 {
					t.Fatalf("expected zero totals")
				}
			}

			if len(tc.wantStopOrder) > 0 {
				if len(plan.Stops) != len(tc.wantStopOrder) {
					t.Fatalf("expected %d stops, got %d", len(tc.wantStopOrder), len(plan.Stops))
				}

				for i, stop := range plan.Stops {
					if stop.Destination != tc.wantStopOrder[i] {
						t.Fatalf("stop %d: expected %q, got %q", i, tc.wantStopOrder[i], stop.Destination)
					}
				}
			}

			if tc.wantFirstAddress != "" && len(plan.Stops) > 0 {
				if plan.Stops[0].Destination != tc.wantFirstAddress {
					t.Fatalf("expected first stop %q, got %q", tc.wantFirstAddress, plan.Stops[0].Destination)
				}
			}

			if tc.wantTotalDistance > 0 && tc.wantTotalDuration > 0 {
				if plan.TotalDistanceMeters != tc.wantTotalDistance {
					t.Fatalf("expected distance: %d, got: %d", tc.wantTotalDistance, plan.TotalDistanceMeters)
				}

				if plan.TotalDurationSeconds != tc.wantTotalDuration {
					t.Fatalf("expected duration: %d, got: %d", tc.wantTotalDuration, plan.TotalDurationSeconds)
				}
			}
		})
	}
}
