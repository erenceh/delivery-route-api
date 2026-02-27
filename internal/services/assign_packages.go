package services

import (
	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/ports"
	"errors"
	"fmt"
	"slices"
)

// AssignPackagesByDistance assigns packages to trucks using a simple heuristic.
//
// Destinations are sorted by hub distance and chunked across trucks to produce a
// deterministic, reasonably balanced distribution without solving a full VRP.
// This is a planning shortcut intended for predictable demo behavior.
func AssignPackagesByDistance(
	trucks []*domain.Truck,
	pkgDest map[string][]*domain.Package,
	distances map[string]ports.DistanceResult,
	destinations []string,
) error {
	if len(trucks) == 0 {
		return errors.New("assign packages: truck list must not be empty")
	}

	// Sort by hub distance so each truck receives a contiguous "band" of destinations.
	slices.SortFunc(destinations, func(a, b string) int {
		da := distances[a].DistanceMeters
		db := distances[b].DistanceMeters
		if da < db {
			return -1
		}
		if da > db {
			return 1
		}
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	})

	nTrucks := len(trucks)
	nDests := len(destinations)

	// Ceiling division: distribute destinations as evenly as possible across trucks.
	chunkSize := (nDests + nTrucks - 1) / nTrucks

	for ti := 0; ti < nTrucks; ti++ {
		start := ti * chunkSize
		if start >= nDests {
			break
		}

		end := start + chunkSize
		if end > nDests {
			end = nDests
		}

		// Load all packages for this destination band onto the truck.
		// If capacity is exceeded, assignment fails fast rather than rebalancing.
		for _, d := range destinations[start:end] {
			for _, pkg := range pkgDest[d] {
				if err := trucks[ti].Load(pkg); err != nil {
					return fmt.Errorf("assign packages: truck %d: %w", trucks[ti].TruckID, err)
				}
			}
		}
	}

	return nil
}
