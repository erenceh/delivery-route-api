package services

import (
	"context"
	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/ports"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// AssignPackagesByDistance assigns packages to trucks using a simple heuristic.
//
// Destinations are sorted by hub distance and chunked across trucks to produce a
// deterministic, reasonably balanced distribution without solving a full VRP.
// This is a planning shortcut intended for predictable demo behavior.
func AssignPackagesByDistance(
	ctx context.Context,
	pkgs []*domain.Package,
	trucks []*domain.Truck,
	hub string,
	provider ports.DistanceProvider,
) error {
	if len(trucks) == 0 {
		return errors.New("assign packages: truck list must not be empty")
	}
	if strings.TrimSpace(hub) == "" {
		return errors.New("assign packages: hub must be non-empty")
	}

	byDest := make(map[string][]*domain.Package)
	for _, pkg := range pkgs {
		d := strings.TrimSpace(pkg.Destination)
		if d == "" {
			return fmt.Errorf(
				"assign packages: package_id=%d has empty destination",
				pkg.PackageID,
			)
		}
		byDest[d] = append(byDest[d], pkg)
	}

	destinations := make([]string, 0, len(byDest))
	for d := range byDest {
		destinations = append(destinations, d)
	}
	if len(destinations) == 0 {
		return nil
	}

	distByDest := make(map[string]ports.DistanceResult, len(destinations))

	// Prefer a single hub->many lookup when supporte to reduce external API calls.
	if mp, ok := provider.(ports.DistanceMatrixProvider); ok {
		results, err := mp.GetDistances(ctx, hub, destinations)
		if err != nil {
			return fmt.Errorf("assign packages: get matrix distances from hub: %w", err)
		}

		for _, d := range destinations {
			r, ok := results[d]
			if !ok {
				return fmt.Errorf("assign packages: missing hub distance for %q", d)
			}
			distByDest[d] = r
		}
	} else {
		for _, d := range destinations {
			r, err := provider.GetDistance(ctx, hub, d)
			if err != nil {
				return fmt.Errorf("assign packages: get distance hub -> %q: %w", d, err)
			}
			distByDest[d] = r
		}
	}

	// Sort by hub distance so each truck receives a contiguous "band" of destinations.
	slices.SortFunc(destinations, func(a, b string) int {
		da := distByDest[a].DistanceMeters
		db := distByDest[b].DistanceMeters
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
			for _, pkg := range byDest[d] {
				if err := trucks[ti].Load(pkg); err != nil {
					return fmt.Errorf("assign packages: truck %d: %w", trucks[ti].TruckID, err)
				}
			}
		}
	}

	return nil
}
