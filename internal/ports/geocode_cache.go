package ports

import (
	"context"
	"delivery-route-service/internal/domain"
)

type GeocodeCache interface {
	// Fetch cached coordinates for the given addresses.
	GetMany(ctx context.Context, addresses []string) (map[string]domain.Coordinates, error)
	// Store address -> coordinate mappings in the cache.
	PutMany(ctx context.Context, results map[string]domain.Coordinates) error
}
