package ports

import "context"

// Optional extension of DistanceProvider that supports batched lookups.
type DistanceMatrixProvider interface {
	DistanceProvider
	// Return distances from one origin to many destinations.
	GetDistances(ctx context.Context, origin string, destinations []string) (map[string]DistanceResult, error)
}
