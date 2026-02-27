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

func PlanDeliveries(
	ctx context.Context,
	req PlanDeliveriesRequest,
	repo ports.PackageRepository,
	provider ports.DistanceProvider,
) ([]*domain.RoutePlan, error) {
	pkgs, err := repo.ListPackages(ctx)
	if err != nil {
		return nil, fmt.Errorf("plan deliveries: list package: %w", err)
	}

	pkgDest := make(map[string][]*domain.Package)
	for _, pkg := range pkgs {
		d := strings.TrimSpace(pkg.Destination)
		if d == "" {
			return nil, fmt.Errorf(
				"plan deliveries: package_id=%d has empty destination",
				pkg.PackageID,
			)
		}
		pkgDest[d] = append(pkgDest[d], pkg)
	}

	destinations := make([]string, 0, len(pkgDest))
	for d := range pkgDest {
		destinations = append(destinations, d)
	}
	if len(destinations) == 0 {
		return []*domain.RoutePlan{}, nil
	}

	distances := make(map[string]ports.DistanceResult, len(destinations))

	// Prefer a single hub->many lookup when supporte to reduce external API calls.
	if mp, ok := provider.(ports.DistanceMatrixProvider); ok {
		results, err := mp.GetDistances(ctx, req.Hub, destinations)
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
			r, err := provider.GetDistance(ctx, req.Hub, d)
			if err != nil {
				return nil, fmt.Errorf("plan deliveries: get distance hub -> %q: %w", d, err)
			}
			distances[d] = r
		}
	}

	trucks := make([]*domain.Truck, 0, req.TruckCount)
	for i := 0; i < req.TruckCount; i++ {
		trucks = append(trucks, domain.NewTruck(i+1, req.TruckCapacity, req.Hub))
	}

	// Assign packages to trucks before computing individual routes.
	if err := AssignPackagesByDistance(trucks, pkgDest, distances, destinations); err != nil {
		return nil, fmt.Errorf("plan deliveries: assign packages: %w", err)
	}

	// Build pairwiseDist: "origin|destination" → DistanceResult for all pairs
	// needed by the nearest-neighbor route planner.
	pairwiseDist := make(map[string]ports.DistanceResult)

	// Hub → each destination (already fetched above).
	for _, d := range destinations {
		pairwiseDist[req.Hub+"|"+d] = distances[d]
	}

	// Each destination → all other destinations and hub.
	hubAndDests := append([]string{req.Hub}, destinations...)
	mp, hasMatrix := provider.(ports.DistanceMatrixProvider)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, 5)
	resultsCh := make(chan pairwiseResult, len(destinations))
	var wg sync.WaitGroup

	for _, origin := range destinations {
		targets := make([]string, 0, len(hubAndDests)-1)
		for _, t := range hubAndDests {
			if t != origin {
				targets = append(targets, t)
			}
		}

		wg.Add(1)
		go func(orig string) {
			sem <- struct{}{}
			defer wg.Done()
			defer func() { <-sem }()

			var res map[string]ports.DistanceResult
			if hasMatrix {
				var e error
				res, e = mp.GetDistances(ctx, orig, targets)
				if e != nil {
					resultsCh <- pairwiseResult{origin: orig, err: fmt.Errorf("plan deliveries: get pairwise distances from %q: %w", orig, e)}
					cancel()
					return
				}
			} else {
				res = make(map[string]ports.DistanceResult, len(targets))
				for _, t := range targets {
					r, e := provider.GetDistance(ctx, orig, t)
					if e != nil {
						resultsCh <- pairwiseResult{origin: orig, err: fmt.Errorf("plan deliveries: get pairwise distance from %q to %q: %w", orig, t, e)}
						cancel()
						return
					}
					res[t] = r
				}
			}

			resultsCh <- pairwiseResult{origin: orig, results: res}
		}(origin)
	}

	wg.Wait()
	close(resultsCh)

	var pairwiseErr error
	for res := range resultsCh {
		if res.err != nil {
			if pairwiseErr == nil {
				pairwiseErr = res.err
			}
			continue
		}
		for _, t := range hubAndDests {
			if t != res.origin {
				r, ok := res.results[t]
				if !ok {
					return nil, fmt.Errorf("plan deliveries: missing pairwise distance from %q to %q", res.origin, t)
				}
				pairwiseDist[res.origin+"|"+t] = r
			}
		}
	}
	if pairwiseErr != nil {
		return nil, pairwiseErr
	}

	// Compute and apply a route plan per truck
	plans := make([]*domain.RoutePlan, 0, len(trucks))
	for _, truck := range trucks {
		plan, err := NearestNeighborRoute(ctx, truck, req.DepartAt, pairwiseDist, req.ReturnToStart)
		if err != nil {
			return nil, fmt.Errorf("plan deliveries: plan nearest neighbor route: %w", err)
		}

		plans = append(plans, plan)
	}

	return plans, nil
}
