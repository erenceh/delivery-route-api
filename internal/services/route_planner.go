package services

import (
	"context"
	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/ports"
	"errors"
	"fmt"
	"math"
	"time"
)

// Plan a delivery route using a greedy nearest-neighbor algorithm.
//
// The algorithm minimizes immediate travel duration at each step.
// It does not attempt global route optimization (e.g., VRP solvers).
// The design prioritizes determinism and simplicity over optimality.
func PlanRoute(
	ctx context.Context,
	truckId int,
	departAt time.Time,
	startLocation string,
	packages []*domain.Package,
	distanceProvider ports.DistanceProvider,
	returnToStart bool,
) (*domain.RoutePlan, error) {
	if startLocation == "" {
		return nil, errors.New("plan route: startLocation must be non-empty")
	}

	if len(packages) == 0 {
		return &domain.RoutePlan{
			TruckID:              truckId,
			DepartAt:             departAt,
			Stops:                []domain.RouteStop{},
			TotalDurationSeconds: 0,
			TotalDistanceMeters:  0,
		}, nil
	}

	byDestination := make(map[string][]int)
	for _, pkg := range packages {
		byDestination[pkg.Destination] = append(byDestination[pkg.Destination], pkg.PackageID)
	}

	remainingDestinations := make(map[string]struct{})
	for dest := range byDestination {
		remainingDestinations[dest] = struct{}{}
	}

	currentTime := departAt
	currentLocation := startLocation

	stops := []domain.RouteStop{}
	totalDistanceMeters := 0
	totalDurationSeconds := 0

	for len(remainingDestinations) > 0 {
		destinations := make([]string, 0, len(remainingDestinations))
		for d := range remainingDestinations {
			destinations = append(destinations, d)
		}

		var (
			results map[string]ports.DistanceResult
			err     error
		)

		// Prefer batched distance lookups when supported to reduce external API calls.
		if provider, ok := distanceProvider.(ports.DistanceMatrixProvider); ok {
			results, err = provider.GetDistances(ctx, currentLocation, destinations)
			if err != nil {
				return nil, fmt.Errorf("plan route: get distances matrix from %q: %w", currentLocation, err)
			}
		} else {
			results = make(map[string]ports.DistanceResult, len(destinations))
			for _, d := range destinations {
				r, e := distanceProvider.GetDistance(ctx, currentLocation, d)
				if e != nil {
					return nil, fmt.Errorf("plan route: get distance: from %q to %q: %w", currentLocation, d, e)
				}
				results[d] = r
			}
		}

		for _, d := range destinations {
			if _, ok := results[d]; !ok {
				return nil, fmt.Errorf("plan route: missing distance result from %q to %q", currentLocation, d)
			}
		}

		var bestDestination string
		minDuration := math.MaxInt64

		// Select next stop by minimum travel duration (greedy step.)
		for _, d := range destinations {
			currentDuration := results[d].DurationSeconds
			// Tie-breaker ensures deterministic ordering when durations are equal.
			if currentDuration < minDuration || (currentDuration == minDuration && (bestDestination == "" || d < bestDestination)) {
				minDuration = currentDuration
				bestDestination = d
			}
		}

		if bestDestination == "" {
			return nil, errors.New("plan route: failed to select next destination")
		}
		bestResult := results[bestDestination]

		currentTime = currentTime.Add(time.Duration(bestResult.DurationSeconds) * time.Second)
		totalDurationSeconds += bestResult.DurationSeconds
		totalDistanceMeters += bestResult.DistanceMeters

		stops = append(
			stops,
			domain.RouteStop{
				Destination: bestDestination,
				ArriveAt:    currentTime,
				PackageIDs:  byDestination[bestDestination],
			},
		)

		delete(remainingDestinations, bestDestination)
		currentLocation = bestDestination
	}

	// Optionally includes return leg to hub for total route metrics.
	if returnToStart {
		back, err := distanceProvider.GetDistance(ctx, currentLocation, startLocation)
		if err != nil {
			return nil, fmt.Errorf("plan route: get distance return leg from %q to %q: %w", currentLocation, startLocation, err)
		}

		currentTime = currentTime.Add(time.Duration(back.DurationSeconds) * time.Second)
		totalDurationSeconds += back.DurationSeconds
		totalDistanceMeters += back.DistanceMeters
	}

	return &domain.RoutePlan{
		TruckID:              truckId,
		DepartAt:             departAt,
		Stops:                stops,
		TotalDurationSeconds: totalDurationSeconds,
		TotalDistanceMeters:  totalDistanceMeters,
	}, nil
}

// Create a RoutePlan for the currently loaded packages.
func PlanTruckRoute(
	ctx context.Context,
	truck *domain.Truck,
	departAt time.Time,
	distanceProvider ports.DistanceProvider,
	returnToStart bool,
) (*domain.RoutePlan, error) {
	if truck == nil {
		return nil, errors.New("plan truck route: truck must be non-nil")
	}

	if truck.StartLocation == "" {
		return nil, fmt.Errorf("plan truck route: truck %d startLocation must be non-empty", truck.TruckID)
	}

	// Delegate to PlanRoute while preserving truck-level invariants.
	plan, err := PlanRoute(ctx, truck.TruckID, departAt, truck.StartLocation, truck.Packages, distanceProvider, returnToStart)
	if err != nil {
		return nil, fmt.Errorf("plan truck route: for truck %d: %w", truck.TruckID, err)
	}
	return plan, nil
}
