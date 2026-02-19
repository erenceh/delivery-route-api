package domain

import (
	"fmt"
	"time"
)

// Delivery truck aggregate holding packages and producing/applying RoutePlans.
type Truck struct {
	TruckID       int
	Capacity      int
	StartLocation string
	DepartAt      *time.Time
	Packages      []*Package
}

// Load a single package onto the truck.
func (t *Truck) Load(pkg *Package) error {
	if len(t.Packages) >= int(t.Capacity) {
		return fmt.Errorf("load truck: Truck %d is at full capacity (capacity=%d)", t.TruckID, t.Capacity)
	}
	t.Packages = append(t.Packages, pkg)
	return nil
}

// Load multiple packages onto the truck.
func (t *Truck) LoadMultiple(pkgs []*Package) error {
	for _, pkg := range pkgs {
		if err := t.Load(pkg); err != nil {
			return err
		}
	}

	return nil
}

// Unload all packages from the truck.
func (t *Truck) Clear() {
	t.Packages = nil
}

// Apply a RoutePlan by mutating timestamps on loaded packages.
func (t *Truck) ApplyPlan(plan *RoutePlan) error {
	if plan.TruckID != t.TruckID {
		return fmt.Errorf("apply plan: RoutePlan truck_id %d does not match Truck %d", plan.TruckID, t.TruckID)
	}

	t.DepartAt = &plan.DepartAt
	for i := range t.Packages {
		t.Packages[i].LoadedAt = t.DepartAt
		t.Packages[i].DeliveredAt = nil
	}

	deliveredMap := make(map[int]time.Time)
	for _, stop := range plan.Stops {
		for _, pid := range stop.PackageIDs {
			deliveredMap[pid] = stop.ArriveAt
		}
	}

	for i := range t.Packages {
		if dt, ok := deliveredMap[t.Packages[i].PackageID]; ok {
			delivered := dt
			t.Packages[i].DeliveredAt = &delivered
		}
	}

	return nil
}
