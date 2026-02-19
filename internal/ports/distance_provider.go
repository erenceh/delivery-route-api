package ports

import "context"

// Distance and travel duration between two locations.
type DistanceResult struct {
	DistanceMeters  int
	DurationSeconds int
}

// Contract for retrieving travel distance and duration between locations.
type DistanceProvider interface {
	// Return travel distance and estimated duration between two locations.
	GetDistance(ctx context.Context, origin string, destination string) (DistanceResult, error)
}
