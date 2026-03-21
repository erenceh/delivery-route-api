package ports

import "context"

type DistanceCache interface {
	// Fetch cached distances for one origin and multiple destinations.
	GetMany(ctx context.Context, origin string, destinations []string) (map[string]DistanceResult, error)
	// Store many cached distance results for a single origin.
	PutMany(ctx context.Context, origin string, results map[string]DistanceResult) error
}
