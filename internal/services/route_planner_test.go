package services

import (
	"context"
	"delivery-route-service/internal/adapters/distance"
	"delivery-route-service/internal/domain"
	"testing"
	"time"
)

func TestRoutePlannerPlanRoute(t *testing.T) {
	type pair struct {
		from    string
		to      string
		meters  int
		seconds int
	}

	packages := []*domain.Package{
		{PackageID: 1, Destination: "A"},
		{PackageID: 2, Destination: "B"},
		{PackageID: 3, Destination: "C"},
	}

	pairs := []distance.MockPair{
		{From: "HUB", To: "A", Meters: 1000, Seconds: 300},
		{From: "HUB", To: "B", Meters: 2000, Seconds: 600},
		{From: "HUB", To: "C", Meters: 1500, Seconds: 450},
		{From: "A", To: "B", Meters: 800, Seconds: 240},
		{From: "A", To: "C", Meters: 700, Seconds: 210},
		{From: "B", To: "C", Meters: 900, Seconds: 270},
		{From: "A", To: "HUB", Meters: 1000, Seconds: 300},
		{From: "B", To: "HUB", Meters: 2000, Seconds: 600},
		{From: "C", To: "HUB", Meters: 1500, Seconds: 450},
		{From: "B", To: "A", Meters: 800, Seconds: 240},
		{From: "C", To: "A", Meters: 700, Seconds: 210},
		{From: "C", To: "B", Meters: 900, Seconds: 270},
	}

	provider := distance.NewMockDistanceProvider(pairs)

	depart := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	plan, err := PlanRoute(context.Background(), 1, depart, "HUB", packages, provider, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Stops) != 3 {
		t.Fatalf("expected 3 stops, got %d", len(plan.Stops))
	}
	if plan.Stops[0].Destination != "A" {
		t.Fatalf("expected first stop A, got %q", plan.Stops[0].Destination)
	}
	if plan.Stops[1].Destination != "C" {
		t.Fatalf("expected second stop C, got %q", plan.Stops[1].Destination)
	}
	if plan.Stops[2].Destination != "B" {
		t.Fatalf("expected third stop B, got %q", plan.Stops[2].Destination)
	}

	if plan.TotalDurationSeconds != 780 {
		t.Fatalf("duration = %d, want 780", plan.TotalDurationSeconds)
	}
	if plan.TotalDistanceMeters != 2600 {
		t.Fatalf("distance = %d, want 2600", plan.TotalDistanceMeters)
	}
}
