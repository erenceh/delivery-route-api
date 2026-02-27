package distance

import (
	"context"
	"delivery-route-service/internal/adapters/cache"
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
	distanceCache *cache.SQLDistanceCache
	geocodeCache  *cache.SQLGeocodeCache
}

func NewORSDistanceProvider(
	apiKey string,
	distanceCache *cache.SQLDistanceCache,
	geocodeCache *cache.SQLGeocodeCache,
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

// normalize ensures consistent cache keys by collapsing whitespace.
func (o *ORSDistanceProvider) normalize(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Delegate to batched path to reuse caching and matrix logic.
func (o *ORSDistanceProvider) GetDistance(
	ctx context.Context,
	origin string,
	destination string,
) (ports.DistanceResult, error) {
	if origin == "" || destination == "" {
		return ports.DistanceResult{}, errors.New("get ORS distance: origin and destination must be non-empty")
	}

	normOrigin := o.normalize(origin)
	if normOrigin == "" {
		return ports.DistanceResult{}, errors.New("origin must be non-empty")
	}

	normDestination := o.normalize(destination)
	if normDestination == "" {
		return ports.DistanceResult{}, errors.New("destination must be non-empty")
	}

	results, err := o.GetDistances(ctx, normOrigin, []string{normDestination})
	if err != nil {
		return ports.DistanceResult{}, fmt.Errorf(
			"get distances %q -> %q: %w",
			normOrigin, normDestination, err,
		)
	}

	result, ok := results[normDestination]
	if !ok {
		return ports.DistanceResult{}, fmt.Errorf("no distance result for %q -> %q", origin, destination)
	}

	return result, nil
}

// Compute distances from a single origin to many destinations.
func (o *ORSDistanceProvider) GetDistances(
	ctx context.Context,
	origin string,
	destinations []string,
) (_ map[string]ports.DistanceResult, err error) {
	defer obs.Time(ctx, "ors.GetDistances")(&err)

	if origin == "" {
		return nil, errors.New("origin must be non-empty")
	}

	if len(destinations) == 0 {
		return map[string]ports.DistanceResult{}, nil
	}

	normOrigin := o.normalize(origin)
	if normOrigin == "" {
		return nil, errors.New("origin must be non-empty")
	}

	normDestinations := make([]string, 0, len(destinations))
	for _, d := range destinations {
		nd := o.normalize(d)
		if nd == "" {
			continue
		}
		normDestinations = append(normDestinations, nd)
	}

	seen := make(map[string]struct{}, len(normDestinations))
	destList := make([]string, 0, len(normDestinations))
	for _, d := range normDestinations {
		if d == normOrigin {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}

		seen[d] = struct{}{}
		destList = append(destList, d)
	}

	if len(destList) == 0 {
		return map[string]ports.DistanceResult{}, nil
	}

	destinationHits := make(map[string]ports.DistanceResult)
	// Check persistent distance cache before issuing external API calls.
	if o.distanceCache != nil {
		var err error
		destinationHits, err = o.distanceCache.GetMany(ctx, normOrigin, destList)
		if err != nil {
			return nil, fmt.Errorf("ORS get distance cache: %w", err)
		}
	}

	destinationMisses := make([]string, 0, len(destList))
	for _, d := range destList {
		if _, ok := destinationHits[d]; !ok {
			destinationMisses = append(destinationMisses, d)
		}
	}

	if len(destinationMisses) == 0 {
		return destinationHits, nil
	}

	needed := make([]string, 0, 1+len(destinationMisses))
	needed = append(needed, normOrigin)
	for _, d := range destinationMisses {
		needed = append(needed, d)
	}

	geocodeHits := make(map[string]domain.Coordinates)
	// Resolve coordinates via cache before calling ORS geocoding.
	if o.geocodeCache != nil {
		var err error
		geocodeHits, err = o.geocodeCache.GetMany(ctx, needed)
		if err != nil {
			return nil, fmt.Errorf("ORS get geocode cache: %w", err)
		}
	}

	geocodeMisses := make([]string, 0, len(needed))
	for _, a := range needed {
		if _, ok := geocodeHits[a]; !ok {
			geocodeMisses = append(geocodeMisses, a)
		}
	}

	fresh := make(map[string]domain.Coordinates)
	if len(geocodeMisses) > 0 {
		var err error
		fresh, err = o.geocodeMany(ctx, geocodeMisses)
		if err != nil {
			return nil, fmt.Errorf(
				"retrieving coordinates: %w",
				err,
			)
		}
	}

	if o.geocodeCache != nil && len(fresh) > 0 {
		if err := o.geocodeCache.PutMany(ctx, fresh); err != nil {
			log.Printf("geocode cache write failed: %v", err)
		}
	}

	coords := make(map[string]domain.Coordinates, len(geocodeHits)+len(fresh))
	for k, v := range geocodeHits {
		coords[k] = v
	}
	for k, v := range fresh {
		coords[k] = v
	}

	originCoord, ok := coords[normOrigin]
	if !ok {
		return nil, fmt.Errorf(
			"missing coordinate for origin %q",
			normOrigin,
		)
	}

	destinationCoords := make([]domain.Coordinates, 0, len(destinationMisses))
	for _, d := range destinationMisses {
		coord, ok := coords[d]
		if !ok {
			return nil, fmt.Errorf(
				"missing coordinate for destination %q",
				d,
			)
		}
		destinationCoords = append(destinationCoords, coord)
	}

	// Fetch a single origin->many matrix row for all cache misses.
	fetched, err := o.fetchMatrixRow(ctx, originCoord, destinationMisses, destinationCoords)
	if err != nil {
		return nil, fmt.Errorf(
			"fetching matrix row: %w",
			err,
		)
	}

	missing := make([]string, 0)

	for _, d := range destinationMisses {
		if _, ok := fetched[d]; !ok {
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
		if err := o.distanceCache.PutMany(ctx, normOrigin, fetched); err != nil {
			log.Printf("distance cache write failed: %v", err)
		}
	}

	out := make(map[string]ports.DistanceResult, len(destinationHits)+len(fetched))
	for k, v := range destinationHits {
		out[k] = v
	}
	for k, v := range fetched {
		out[k] = v
	}

	return out, nil
}
