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
func NearestNeighborRoute(
	ctx context.Context,
	truck *domain.Truck,
	departAt time.Time,
	distances map[string]ports.DistanceResult,
	returnToStart bool,
) (*domain.RoutePlan, error) {
	startLocation := truck.StartLocation
	packages := truck.Packages

	if startLocation == "" {
		return nil, errors.New("plan route: startLocation must be non-empty")
	}

	if len(packages) == 0 {
		return &domain.RoutePlan{
			TruckID:              truck.TruckID,
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

		for _, d := range destinations {
			if _, ok := distances[currentLocation+"|"+d]; !ok {
				return nil, fmt.Errorf("plan route: missing distance result from %q to %q", currentLocation, d)
			}
		}

		var bestDestination string
		minDuration := math.MaxInt64

		// Select next stop by minimum travel duration (greedy step.)
		for _, d := range destinations {
			currentDuration := distances[currentLocation+"|"+d].DurationSeconds
			// Tie-breaker ensures deterministic ordering when durations are equal.
			if currentDuration < minDuration || (currentDuration == minDuration && (bestDestination == "" || d < bestDestination)) {
				minDuration = currentDuration
				bestDestination = d
			}
		}

		if bestDestination == "" {
			return nil, errors.New("plan route: failed to select next destination")
		}
		bestResult := distances[currentLocation+"|"+bestDestination]

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
		back, ok := distances[currentLocation+"|"+startLocation]
		if !ok {
			return nil, fmt.Errorf(
				"plan route: missing distance result for return leg from %q to %q",
				currentLocation, startLocation,
			)
		}

		currentTime = currentTime.Add(time.Duration(back.DurationSeconds) * time.Second)
		totalDurationSeconds += back.DurationSeconds
		totalDistanceMeters += back.DistanceMeters
	}

	return &domain.RoutePlan{
		TruckID:              truck.TruckID,
		DepartAt:             departAt,
		Stops:                stops,
		TotalDurationSeconds: totalDurationSeconds,
		TotalDistanceMeters:  totalDistanceMeters,
	}, nil
}
