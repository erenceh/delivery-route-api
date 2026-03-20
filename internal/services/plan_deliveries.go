package services

import (
	"context"
	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/ports"
	"fmt"
	"strings"
	"sync"
	"time"
)

type pairwiseResult struct {
	origin  string
	results map[string]ports.DistanceResult
	err     error
}

type PlanDeliveriesRequest struct {
	Hub           string
	TruckCount    int
	TruckCapacity int
	DepartAt      time.Time
	ReturnToStart bool
}

// validateRequest checks that required fields in PlanDeliveriesRequest are valid.
func validateRequest(req PlanDeliveriesRequest) error {
	if req.Hub == "" {
		return fmt.Errorf("plan deliveries: hub address must not be empty")
	}
	if req.TruckCount <= 0 {
		return fmt.Errorf("plan deliveries: truck count must be positive, got %d", req.TruckCount)
	}
	if req.TruckCapacity <= 0 {
		return fmt.Errorf("plan deliveries: truck capacity must be positive, got %d", req.TruckCapacity)
	}
	return nil
}

// loadPackages fetches all packages from the repository and groups them by destinations.
// Returns an empty map and nil error if no packages exit.
func loadPackages(
	ctx context.Context,
	repo ports.PackageRepository,
) (pkgDest map[string][]*domain.Package, destinations []string, err error) {
	pkgs, err := repo.ListPackages(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("plan deliveries: list package: %w", err)
	}

	pkgDest = make(map[string][]*domain.Package)
	for _, pkg := range pkgs {
		d := strings.TrimSpace(pkg.Destination)
		if d == "" {
			return nil, nil, fmt.Errorf(
				"plan deliveries: package_id=%d has empty destination",
				pkg.PackageID,
			)
		}
		pkgDest[d] = append(pkgDest[d], pkg)
	}

	destinations = make([]string, 0, len(pkgDest))
	for d := range pkgDest {
		destinations = append(destinations, d)
	}

	return pkgDest, destinations, nil
}

// fetchHubDistances retrives travel distances from the hub to all destinations.
// Uses batched lookup when the provider supports it.
func fetchHubDistances(
	ctx context.Context,
	hub string,
	destinations []string,
	provider ports.DistanceProvider,
) (distances map[string]ports.DistanceResult, err error) {
	distances = make(map[string]ports.DistanceResult, len(destinations))
	// Prefer a single hub->many lookup when support to reduce external API calls.
	if mp, ok := provider.(ports.DistanceMatrixProvider); ok {
		results, err := mp.GetDistances(ctx, hub, destinations)
		if err != nil {
			return nil, fmt.Errorf("plan deliveries: get matrix distances from hub: %w", err)
		}

		for _, d := range destinations {
			r, ok := results[d]
			if !ok {
				return nil, fmt.Errorf("plan deliveries: missing hub distance for %q", d)
			}
			distances[d] = r
		}
	} else {
		for _, d := range destinations {
			r, err := provider.GetDistance(ctx, hub, d)
			if err != nil {
				return nil, fmt.Errorf("plan deliveries: get distance hub -> %q: %w", d, err)
			}
			distances[d] = r
		}
	}

	return distances, nil
}

// fetchDistancesFromOrigin fetches distances from a single origin to all targets.
// Uses batched lookup when the provider supports it, falls back to sequential calls.
func fetchDistancesFromOrigin(
	ctx context.Context,
	origin string,
	targets []string,
	provider ports.DistanceProvider,
) (distanceResult map[string]ports.DistanceResult, err error) {
	distanceResult = make(map[string]ports.DistanceResult, len(targets))
	if mp, ok := provider.(ports.DistanceMatrixProvider); ok {
		distanceResult, err = mp.GetDistances(ctx, origin, targets)
		if err != nil {
			return nil, fmt.Errorf("plan deliveries: get pairwise distances from %q: %w", origin, err)
		}
	} else {
		for _, t := range targets {
			r, e := provider.GetDistance(ctx, origin, t)
			if e != nil {
				return nil, fmt.Errorf("plan deliveries: get pairwise distance from %q to %q: %w", origin, t, e)
			}
			distanceResult[t] = r
		}
	}

	return distanceResult, nil
}

// collectPairwiseResults collects goroutine results from resultsCh and assembles
// the pairwise distance map. Seeds hub→destination distances before collecting.
func collectPairwiseResults(
	resultsCh <-chan pairwiseResult,
	hub string,
	hubAndDests []string,
	hubDistances map[string]ports.DistanceResult,
) (pairwiseDist map[string]ports.DistanceResult, err error) {
	pairwiseDist = make(map[string]ports.DistanceResult)
	// Seeds pairwiseDist with already fetched distances (Hub → destination).
	for _, d := range hubAndDests {
		if d != hub {
			pairwiseDist[hub+"|"+d] = hubDistances[d]
		}
	}

	for res := range resultsCh {
		if res.err != nil {
			if err == nil {
				err = res.err
			}
			continue
		}
		for _, t := range hubAndDests {
			if t != res.origin {
				r, ok := res.results[t]
				if !ok {
					return nil, fmt.Errorf(
						"plan deliveries: missing pairwise distance from %q to %q",
						res.origin, t)
				}
				pairwiseDist[res.origin+"|"+t] = r
			}
		}
	}

	return pairwiseDist, err
}

// fetchPairwiseDistances fetches distances between all destination pairs concurrently.
// Uses a bounded goroutine pool (semaphore size 5) to limit concurrent ORS calls.
// Hub→destination distances are seeded from the already-fetched distances map.
func fetchPairwiseDistances(
	ctx context.Context,
	hub string,
	destinations []string,
	distances map[string]ports.DistanceResult,
	provider ports.DistanceProvider,
) (pairwiseDist map[string]ports.DistanceResult, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, 5)
	resultsCh := make(chan pairwiseResult, len(destinations))
	var wg sync.WaitGroup

	// Each destination → all other destinations and hub.
	hubAndDests := append([]string{hub}, destinations...)
	for _, origin := range destinations {
		targets := make([]string, 0, len(hubAndDests)-1)
		for _, t := range hubAndDests {
			if t != origin {
				targets = append(targets, t)
			}
		}

		wg.Add(1)
		go func(orig string, tgts []string) {
			sem <- struct{}{}
			defer wg.Done()
			defer func() { <-sem }()
			distanceResult, err := fetchDistancesFromOrigin(ctx, orig, tgts, provider)
			if err != nil {
				resultsCh <- pairwiseResult{origin: orig, err: err}
				cancel()
				return
			}
			resultsCh <- pairwiseResult{origin: orig, results: distanceResult}
		}(origin, targets)
	}
	wg.Wait()
	close(resultsCh)

	// Build pairwiseDist: "origin|destination" → DistanceResult for all pairs
	// needed by the nearest-neighbor route planner.
	pairwiseDist, err = collectPairwiseResults(resultsCh, hub, hubAndDests, distances)
	if err != nil {
		return nil, err
	}

	return pairwiseDist, nil
}

// planRoutes computes a route plan per truck.
// Only trucks with assigned packages are included in the returned plans.
func planRoutes(
	ctx context.Context,
	req PlanDeliveriesRequest,
	pairwiseDist map[string]ports.DistanceResult,
	trucks []*domain.Truck,
) (plans []*domain.RoutePlan, err error) {

	// Compute and apply a route plan per truck
	plans = make([]*domain.RoutePlan, 0, len(trucks))
	for _, truck := range trucks {
		plan, err := NearestNeighborRoute(ctx, truck, req.DepartAt, pairwiseDist, req.ReturnToStart)
		if err != nil {
			return nil, fmt.Errorf("plan deliveries: plan nearest neighbor route: %w", err)
		}
		if len(plan.Stops) > 0 {
			plans = append(plans, plan)
		}
	}

	return plans, nil
}

// PlanDeliveries orchestrates the full route planning workflow.
// It loads packages, fetches distances, assigns packages to trucks,
// and computes a nearest-neighbor route plan for each truck.
// Only trucks with assigned packages are included in the returned plans.
func PlanDeliveries(
	ctx context.Context,
	req PlanDeliveriesRequest,
	repo ports.PackageRepository,
	provider ports.DistanceProvider,
) ([]*domain.RoutePlan, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}

	pkgDest, destinations, err := loadPackages(ctx, repo)
	if err != nil {
		return nil, err
	}

	if len(destinations) == 0 {
		return []*domain.RoutePlan{}, nil
	}

	distances, err := fetchHubDistances(ctx, req.Hub, destinations, provider)
	if err != nil {
		return nil, err
	}

	trucks := make([]*domain.Truck, 0, req.TruckCount)
	for i := 0; i < req.TruckCount; i++ {
		trucks = append(trucks, domain.NewTruck(i+1, req.TruckCapacity, req.Hub))
	}

	// Assign packages to trucks before computing individual routes.
	if err := AssignPackagesByDistance(trucks, pkgDest, distances, destinations); err != nil {
		return nil, fmt.Errorf("plan deliveries: assign packages: %w", err)
	}

	pairwiseDist, err := fetchPairwiseDistances(ctx, req.Hub, destinations, distances, provider)
	if err != nil {
		return nil, err
	}

	return planRoutes(ctx, req, pairwiseDist, trucks)
}
