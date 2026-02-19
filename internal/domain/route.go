package domain

import "time"

// Represents a single stop in a delivery route.
// A RouteStop corresponds to arriving at a specific destination at a computed time,
// and delivering one or more packages associated with that destination.
type RouteStop struct {
	Destination string
	ArriveAt    time.Time
	PackageIDs  []int
}

// Represents the planned delivery route for a single truck.
// A RoutePlan is the output of a routing algorithm and describes the order
// sequence of delivery stops, along with aggregate distance and duration metrics.
// It is immutable planning data and contains no side effects.
type RoutePlan struct {
	TruckID              int
	DepartAt             time.Time
	Stops                []RouteStop
	TotalDurationSeconds int
	TotalDistanceMeters  int
}
