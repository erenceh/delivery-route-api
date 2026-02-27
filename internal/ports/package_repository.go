package ports

import (
	"context"
	"delivery-route-service/internal/domain"
)

// Port: a boundary for retrieving Package entities from a data source.
type PackageRepository interface {
	// Retrieve all packages available for routing.
	ListPackages(ctx context.Context) ([]*domain.Package, error)
}
