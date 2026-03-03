package services_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/services"
	"delivery-route-service/internal/testutil"
)

func TestPlanDeliveries(t *testing.T) {
	hub := "Hub"
	destA := "DestA"
	destB := "DestB"
	departAt := time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC)

	// All pairs required by a 2-destination, 2-truck run (hub→dests + full
	// pairwise dest↔dest and dest→hub, as fetched by the goroutine pool).
	twoDests := []testutil.MockPair{
		{From: hub, To: destA, Meters: 1000, Seconds: 60},
		{From: hub, To: destB, Meters: 2000, Seconds: 120},
		{From: destA, To: hub, Meters: 1000, Seconds: 60},
		{From: destA, To: destB, Meters: 3000, Seconds: 180},
		{From: destB, To: hub, Meters: 2000, Seconds: 120},
		{From: destB, To: destA, Meters: 3000, Seconds: 180},
	}

	// Capacity-overflow scenario: only hub→destA is fetched before assignment fails.
	overflowPairs := []testutil.MockPair{
		{From: hub, To: destA, Meters: 1000, Seconds: 60},
	}

	repoErr := errors.New("database unavailable")

	tests := []struct {
		name        string
		req         services.PlanDeliveriesRequest
		repo        *testutil.MockPackageRepository
		provider    *testutil.MockDistanceProvider
		wantPlans   int
		wantErr     bool
		errContains string
	}{
		{
			name: "empty list when no packages exist",
			req: services.PlanDeliveriesRequest{
				Hub:           hub,
				TruckCount:    2,
				TruckCapacity: 5,
				DepartAt:      departAt,
			},
			repo:      testutil.NewMockPackageRepository(nil, nil),
			provider:  testutil.NewMockDistanceProvider(nil),
			wantPlans: 0,
		},
		{
			name: "plans are only returned for trucks with assigned packages",
			req: services.PlanDeliveriesRequest{
				Hub:           hub,
				TruckCount:    2,
				TruckCapacity: 5,
				DepartAt:      departAt,
			},
			repo: testutil.NewMockPackageRepository([]*domain.Package{
				{PackageID: 1, Destination: destA},
				{PackageID: 2, Destination: destB},
			}, nil),
			provider:  testutil.NewMockDistanceProvider(twoDests),
			wantPlans: 2,
		},
		{
			name: "propagates repo error",
			req: services.PlanDeliveriesRequest{
				Hub:           hub,
				TruckCount:    2,
				TruckCapacity: 5,
				DepartAt:      departAt,
			},
			repo:        testutil.NewMockPackageRepository(nil, repoErr),
			provider:    testutil.NewMockDistanceProvider(nil),
			wantErr:     true,
			errContains: repoErr.Error(),
		},
		{
			name: "error when hub is empty",
			req: services.PlanDeliveriesRequest{
				Hub:           "",
				TruckCount:    2,
				TruckCapacity: 5,
				DepartAt:      departAt,
			},
			repo:        testutil.NewMockPackageRepository(nil, nil),
			provider:    testutil.NewMockDistanceProvider(nil),
			wantErr:     true,
			errContains: "hub",
		},
		{
			name: "error when packages exceed total truck capacity",
			req: services.PlanDeliveriesRequest{
				Hub:           hub,
				TruckCount:    1,
				TruckCapacity: 1,
				DepartAt:      departAt,
			},
			repo: testutil.NewMockPackageRepository([]*domain.Package{
				{PackageID: 1, Destination: destA},
				{PackageID: 2, Destination: destA},
			}, nil),
			provider:    testutil.NewMockDistanceProvider(overflowPairs),
			wantErr:     true,
			errContains: "capacity",
		},
		{
			name: "error when TruckCount is 0",
			req: services.PlanDeliveriesRequest{
				Hub:           hub,
				TruckCount:    0,
				TruckCapacity: 5,
				DepartAt:      departAt,
			},
			repo:        testutil.NewMockPackageRepository(nil, nil),
			provider:    testutil.NewMockDistanceProvider(nil),
			wantErr:     true,
			errContains: "truck count",
		},
		{
			name: "error when TruckCount is negative",
			req: services.PlanDeliveriesRequest{
				Hub:           hub,
				TruckCount:    -1,
				TruckCapacity: 5,
				DepartAt:      departAt,
			},
			repo:        testutil.NewMockPackageRepository(nil, nil),
			provider:    testutil.NewMockDistanceProvider(nil),
			wantErr:     true,
			errContains: "truck count",
		},
		{
			name: "error when TruckCapacity is 0",
			req: services.PlanDeliveriesRequest{
				Hub:           hub,
				TruckCount:    2,
				TruckCapacity: 0,
				DepartAt:      departAt,
			},
			repo:        testutil.NewMockPackageRepository(nil, nil),
			provider:    testutil.NewMockDistanceProvider(nil),
			wantErr:     true,
			errContains: "truck capacity",
		},
		{
			name: "error when TruckCapacity is negative",
			req: services.PlanDeliveriesRequest{
				Hub:           hub,
				TruckCount:    2,
				TruckCapacity: -1,
				DepartAt:      departAt,
			},
			repo:        testutil.NewMockPackageRepository(nil, nil),
			provider:    testutil.NewMockDistanceProvider(nil),
			wantErr:     true,
			errContains: "truck capacity",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plans, err := services.PlanDeliveries(context.Background(), tc.req, tc.repo, tc.provider)

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
			if len(plans) != tc.wantPlans {
				t.Fatalf("expected %d plans, got %d", tc.wantPlans, len(plans))
			}
		})
	}
}
