package domain

import "time"

// Represents a single delivery unit handled by the system.
// A Package has a unique identifier and a single destination address.
// Delivery timestamps are poplated during simulation after a route
// has been planned and applied.
type Package struct {
	PackageID   int
	Destination string
	LoadedAt    *time.Time
	DeliveredAt *time.Time
}
