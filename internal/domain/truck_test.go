package domain

import (
	"testing"
	"time"
)

func TestTruckApplyPlan(t *testing.T) {
	// build test data
	pkg1 := &Package{PackageID: 1, Destination: "A"}
	pkg2 := &Package{PackageID: 2, Destination: "B"}
	pkg3 := &Package{PackageID: 3, Destination: "C"}

	truck := &Truck{
		TruckID:  1,
		Capacity: 3,
		Packages: []*Package{pkg1, pkg2, pkg3},
	}

	departAt := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)

	plan := RoutePlan{
		TruckID:  1,
		DepartAt: departAt,
		Stops: []RouteStop{
			{
				Destination: "A",
				ArriveAt:    departAt.Add(10 * time.Minute),
				PackageIDs:  []int{1},
			},
			{
				Destination: "B",
				ArriveAt:    departAt.Add(20 * time.Minute),
				PackageIDs:  []int{2},
			},
		},
	}

	// call the method under test
	err := truck.ApplyPlan(&plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// verify behavior
	for _, pkg := range truck.Packages {
		if pkg.LoadedAt == nil {
			t.Errorf("package %d LoadedAt is nil", pkg.PackageID)
			continue
		}

		if !pkg.LoadedAt.Equal(departAt) {
			t.Errorf(
				"package %d LoadedAt = %v, want %v",
				pkg.PackageID,
				*pkg.LoadedAt,
				departAt,
			)
		}
	}

	if pkg1.DeliveredAt == nil || !pkg1.DeliveredAt.Equal(departAt.Add(10*time.Minute)) {
		t.Errorf("pk1 DeliveredAt incorrect: %v", pkg1.DeliveredAt)
	}

	if pkg2.DeliveredAt == nil || !pkg2.DeliveredAt.Equal(departAt.Add(20*time.Minute)) {
		t.Errorf("pk2 DeliveredAt incorrect: %v", pkg2.DeliveredAt)
	}

	if pkg3.DeliveredAt != nil {
		t.Errorf("pkg3 should not be delivered, got %v", pkg3.DeliveredAt)
	}
}
