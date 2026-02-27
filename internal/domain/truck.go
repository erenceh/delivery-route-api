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

func NewTruck(id int, capacity int, hub string) *Truck {
	return &Truck{
		TruckID:       id,
		Capacity:      16,
		StartLocation: hub,
	}
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
