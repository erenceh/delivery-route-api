package services_test

import (
	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/ports"
	"delivery-route-service/internal/services"
	"strings"
	"testing"
)

func TestAssignPackages(t *testing.T) {
	tests := []struct {
		name                 string
		trucks               []*domain.Truck
		pkgDest              map[string][]*domain.Package
		distances            map[string]ports.DistanceResult
		destinations         []string
		wantErr              bool
		errContains          string
		wantPackagesPerTruck []int
	}{
		{
			name:   "error when truck list is empty",
			trucks: []*domain.Truck{},
			pkgDest: map[string][]*domain.Package{
				"DestA": {{PackageID: 1, Destination: "DestA"}},
			},
			distances: map[string]ports.DistanceResult{
				"DestA": {DistanceMeters: 1000, DurationSeconds: 60},
			},
			destinations: []string{"DestA"},
			wantErr:      true,
			errContains:  "truck list",
		},
		{
			name: "error when more packages than trucks can hold",
			trucks: []*domain.Truck{
				{TruckID: 1, Capacity: 2, StartLocation: "HUB"},
			},
			pkgDest: map[string][]*domain.Package{
				"DestA": {{PackageID: 1, Destination: "DestA"}},
				"DestB": {{PackageID: 2, Destination: "DestB"}},
				"DestC": {{PackageID: 3, Destination: "DestC"}},
			},
			distances: map[string]ports.DistanceResult{
				"DestA": {DistanceMeters: 1000, DurationSeconds: 60},
				"DestB": {DistanceMeters: 2000, DurationSeconds: 120},
				"DestC": {DistanceMeters: 3000, DurationSeconds: 180},
			},
			destinations: []string{"DestA", "DestB", "DestC"},
			wantErr:      true,
			errContains:  "truck",
		},
		{
			name: "packages distributed evenly across multiple trucks",
			trucks: []*domain.Truck{
				{TruckID: 1, Capacity: 4, StartLocation: "HUB"},
				{TruckID: 2, Capacity: 4, StartLocation: "HUB"},
			},
			pkgDest: map[string][]*domain.Package{
				"DestA": {{PackageID: 1, Destination: "DestA"}},
				"DestB": {{PackageID: 2, Destination: "DestB"}},
				"DestC": {{PackageID: 3, Destination: "DestC"}},
				"DestD": {{PackageID: 4, Destination: "DestD"}},
			},
			distances: map[string]ports.DistanceResult{
				"DestA": {DistanceMeters: 1000, DurationSeconds: 60},
				"DestB": {DistanceMeters: 2000, DurationSeconds: 120},
				"DestC": {DistanceMeters: 3000, DurationSeconds: 180},
				"DestD": {DistanceMeters: 4000, DurationSeconds: 240},
			},
			destinations:         []string{"DestA", "DestB", "DestC", "DestD"},
			wantPackagesPerTruck: []int{2, 2},
		},
		{
			name: "single truck gets all packages",
			trucks: []*domain.Truck{
				{TruckID: 1, Capacity: 3, StartLocation: "HUB"},
			},
			pkgDest: map[string][]*domain.Package{
				"DestA": {{PackageID: 1, Destination: "DestA"}},
				"DestB": {{PackageID: 2, Destination: "DestB"}},
			},
			distances: map[string]ports.DistanceResult{
				"DestA": {DistanceMeters: 1000, DurationSeconds: 60},
				"DestB": {DistanceMeters: 2000, DurationSeconds: 120},
			},
			destinations:         []string{"DestA", "DestB"},
			wantPackagesPerTruck: []int{2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := services.AssignPackagesByDistance(tc.trucks, tc.pkgDest, tc.distances, tc.destinations)

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

			for i, want := range tc.wantPackagesPerTruck {
				if len(tc.trucks[i].Packages) != want {
					t.Fatalf("truck %d: expected %d packages, got %d",
						tc.trucks[i].TruckID, want, len(tc.trucks[i].Packages),
					)
				}
			}

		})
	}
}
