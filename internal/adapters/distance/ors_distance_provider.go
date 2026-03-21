package distance

import (
	"context"
	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/platform/obs"
	"delivery-route-service/internal/ports"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// ORSDistanceProvider implements DistanceProvider using OpenRouteService.
//
// It coordinates:
//   - Address normalization
//   - Persistent geocode caching
//   - Persistent distance matrix caching
//   - External API calls with retry/backoff
//
// The provider is safe for concurrent use.
type ORSDistanceProvider struct {
	session       *http.Client
	apiKey        string
	baseURL       string
	profile       string
	distanceCache ports.DistanceCache
	geocodeCache  ports.GeocodeCache
}

func NewORSDistanceProvider(
	apiKey string,
	distanceCache ports.DistanceCache,
	geocodeCache ports.GeocodeCache,
) (*ORSDistanceProvider, error) {
	if apiKey == "" {
		return nil, errors.New("ORS api key is empty")
	}

	provider := &ORSDistanceProvider{
		session:       &http.Client{Timeout: 10 * time.Second},
		apiKey:        apiKey,
		baseURL:       "https://api.openrouteservice.org",
		profile:       "driving-car",
		distanceCache: distanceCache,
		geocodeCache:  geocodeCache,
	}

	return provider, nil
}

// normalizeAndDedupe collapses whitespace in origin and destinations,
// removes duplicates, and filters out destinations equal to the origin.
func (o *ORSDistanceProvider) normalizeAndDedupe(
	origin string,
	destinations []string,
) (string, []string, error) {
	if origin == "" {
		return "", nil, errors.New("get ORS distance: origin must be non-empty")
	}

	if len(destinations) == 0 {
		return "", nil, errors.New("get ORS distance: destinations must be non-empty")
	}

	normOrigin := strings.Join(strings.Fields(origin), " ")

	seen := make(map[string]struct{}, len(destinations))
	normDestList := make([]string, 0, len(destinations))
	for _, d := range destinations {
		d = strings.Join(strings.Fields(d), " ")
		if d == normOrigin {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}

		seen[d] = struct{}{}
		normDestList = append(normDestList, d)
	}

	return normOrigin, normDestList, nil
}

// resolveDistances checks the distance cache and returns hits and misses.
func (o *ORSDistanceProvider) resolveDistances(
	ctx context.Context,
	origin string,
	destinations []string,
) (hits map[string]ports.DistanceResult, misses []string, err error) {
	destinationHits := make(map[string]ports.DistanceResult)
	// Check persistent distance cache before issuing external API calls.
	if o.distanceCache != nil {
		destinationHits, err = o.distanceCache.GetMany(ctx, origin, destinations)
		if err != nil {
			return nil, nil, fmt.Errorf("ORS get distance cache: %w", err)
		}
	}

	destinationMisses := make([]string, 0, len(destinations))
	for _, d := range destinations {
		if _, ok := destinationHits[d]; !ok {
			destinationMisses = append(destinationMisses, d)
		}
	}

	return destinationHits, destinationMisses, nil
}

// resolveCoordinates resolves addresses to coordinates via cache and ORS geocoding.
// Fresh results are written back to the geocode cache.
func (o *ORSDistanceProvider) resolveCoordinates(
	ctx context.Context,
	addresses []string,
) (coords map[string]domain.Coordinates, err error) {
	geocodeHits := make(map[string]domain.Coordinates)
	// Resolve coordinates via cache before calling ORS geocoding.
	if o.geocodeCache != nil {
		geocodeHits, err = o.geocodeCache.GetMany(ctx, addresses)
		if err != nil {
			return nil, fmt.Errorf("ORS get geocode cache: %w", err)
		}
	}

	geocodeMisses := make([]string, 0, len(addresses))
	for _, a := range addresses {
		if _, ok := geocodeHits[a]; !ok {
			geocodeMisses = append(geocodeMisses, a)
		}
	}

	coordResults := make(map[string]domain.Coordinates)
	if len(geocodeMisses) > 0 {
		coordResults, err = o.geocodeMany(ctx, geocodeMisses)
		if err != nil {
			return nil, fmt.Errorf("retrieving coordinates: %w", err)
		}
	}

	if o.geocodeCache != nil && len(coordResults) > 0 {
		if err := o.geocodeCache.PutMany(ctx, coordResults); err != nil {
			log.Printf("geocode cache write failed: %v", err)
		}
	}

	coords = make(map[string]domain.Coordinates, len(geocodeHits)+len(coordResults))
	for k, v := range geocodeHits {
		coords[k] = v
	}
	for k, v := range coordResults {
		coords[k] = v
	}

	return coords, nil
}

// fetchAndCacheDistance fetches distances from ORS for cache misses,
// validates results, and writes them to the distance cache.
func (o *ORSDistanceProvider) fetchAndCacheDistances(
	ctx context.Context,
	origin string,
	misses []string,
	coords map[string]domain.Coordinates,
) (distances map[string]ports.DistanceResult, err error) {
	originCoord, ok := coords[origin]
	if !ok {
		return nil, fmt.Errorf("missing coordinate for origin %q", origin)
	}

	destinationCoords := make([]domain.Coordinates, 0, len(misses))
	for _, d := range misses {
		coord, ok := coords[d]
		if !ok {
			return nil, fmt.Errorf("missing coordinate for destination %q", d)
		}
		destinationCoords = append(destinationCoords, coord)
	}

	// Fetch a single origin->many matrix row for all cache misses.
	distances, err = o.fetchMatrixRow(ctx, originCoord, misses, destinationCoords)
	if err != nil {
		return nil, fmt.Errorf("fetching matrix row: %w", err)
	}

	var missing []string
	for _, d := range misses {
		if _, ok := distances[d]; !ok {
			missing = append(missing, d)
		}
	}

	if len(missing) > 0 {
		missingStr := strings.Join(missing, ", ")
		return nil, fmt.Errorf(
			"ORS matrix service did not return the following destinations: %s",
			missingStr,
		)
	}

	if o.distanceCache != nil {
		if err := o.distanceCache.PutMany(ctx, origin, distances); err != nil {
			log.Printf("distance cache write failed: %v", err)
		}
	}

	return distances, nil
}

// Delegate to batched path to reuse caching and matrix logic.
func (o *ORSDistanceProvider) GetDistance(
	ctx context.Context,
	origin string,
	destination string,
) (ports.DistanceResult, error) {
	origin, destinations, err := o.normalizeAndDedupe(origin, []string{destination})
	if err != nil {
		return ports.DistanceResult{}, err
	}

	results, err := o.GetDistances(ctx, origin, destinations)
	if err != nil {
		return ports.DistanceResult{}, fmt.Errorf(
			"get distances %q -> %q: %w",
			origin, destination, err,
		)
	}

	result, ok := results[destinations[0]]
	if !ok {
		return ports.DistanceResult{}, fmt.Errorf("no distance result for %q -> %q", origin, destination)
	}

	return result, nil
}

func (o *ORSDistanceProvider) GetDistances(
	ctx context.Context,
	origin string,
	destinations []string,
) (out map[string]ports.DistanceResult, err error) {
	defer obs.Time(ctx, "ors.GetDistances")(&err)

	if len(destinations) == 0 {
		return map[string]ports.DistanceResult{}, nil
	}

	origin, destList, err := o.normalizeAndDedupe(origin, destinations)
	if err != nil {
		return nil, err
	}

	hits, misses, err := o.resolveDistances(ctx, origin, destList)
	if err != nil {
		return nil, err
	}

	if len(misses) == 0 {
		return hits, nil
	}

	coords, err := o.resolveCoordinates(ctx, append([]string{origin}, misses...))
	if err != nil {
		return nil, err
	}

	fetched, err := o.fetchAndCacheDistances(ctx, origin, misses, coords)
	if err != nil {
		return nil, err
	}

	out = make(map[string]ports.DistanceResult, len(hits)+len(fetched))
	for k, v := range hits {
		out[k] = v
	}
	for k, v := range fetched {
		out[k] = v
	}

	return out, nil
}
