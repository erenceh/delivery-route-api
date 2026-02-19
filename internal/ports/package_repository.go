package ports

import "delivery-route-service/internal/domain"

// Port: a boundary for retrieving Package entities from a data source.
type PackageRepository interface {
	// Retrieve all packages available for routing.
	ListPackages() ([]*domain.Package, error)
}
