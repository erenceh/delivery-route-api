package distance

import (
	"context"
	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/platform/obs"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type geocodeResponse struct {
	Features []struct {
		Geometry struct {
			Coordinates []float64 `json:"coordinates"`
		} `json:"geometry"`
	} `json:"features"`
}

type geocodeResult struct {
	address string
	result  domain.Coordinates
	err     error
}

// geocodeSingle takes a single address and fetches the geocode coordinates
// from OpenRouteService API
func (o *ORSDistanceProvider) geocodeSingle(
	ctx context.Context,
	address string,
) (domain.Coordinates, error) {
	endpoint := o.baseURL + "/geocode/search"

	resp, e := o.doWithRetry(ctx, func() (*http.Request, error) {
		req, err := o.newRequest(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		q := req.URL.Query()
		q.Set("text", address)
		q.Set("boundary.country", "US")
		q.Set("size", "1")
		req.URL.RawQuery = q.Encode()
		return req, nil
	})
	if e != nil {
		return domain.Coordinates{}, fmt.Errorf("execute request: %w", e)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return domain.Coordinates{}, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var decoded geocodeResponse
	if e := json.NewDecoder(resp.Body).Decode(&decoded); e != nil {
		return domain.Coordinates{}, fmt.Errorf("decode geocode response: %w", e)
	}

	if len(decoded.Features) == 0 {
		return domain.Coordinates{}, fmt.Errorf("no geocode results for %q", address)
	}

	coords := decoded.Features[0].Geometry.Coordinates
	return domain.Coordinates{Lon: coords[0], Lat: coords[1]}, nil
}

// geocodeMany resolves addresses individually using OpenRouteService (/geocode/search).
// Calls are deduplicated and may be retried via doWithRetry.
func (o *ORSDistanceProvider) geocodeMany(
	ctx context.Context,
	addresses []string,
) (_ map[string]domain.Coordinates, err error) {
	defer obs.Time(ctx, "ors.geocodeMany")(&err)

	seen := make(map[string]struct{}, len(addresses))
	addrList := make([]string, 0, len(addresses))
	for _, a := range addresses {
		a = strings.Join(strings.Fields(a), " ")
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		addrList = append(addrList, a)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, 5)
	resultsCh := make(chan geocodeResult, len(addrList))
	var wg sync.WaitGroup

	// Fan-out: spawn one goroutine per address, bounded by sem semaphore.
	for _, a := range addrList {
		wg.Add(1)
		go func(addr string) {
			sem <- struct{}{}
			defer wg.Done()
			defer func() { <-sem }()

			coords, err := o.geocodeSingle(ctx, addr)
			if err != nil {
				resultsCh <- geocodeResult{address: addr, err: err}
				cancel()
				return
			}
			resultsCh <- geocodeResult{address: addr, result: coords}
		}(a)
	}

	wg.Wait()
	close(resultsCh)

	out := make(map[string]domain.Coordinates)
	var geocodeErr error
	for res := range resultsCh {
		if res.err != nil {
			if geocodeErr == nil {
				geocodeErr = res.err
			}
			continue
		}
		out[res.address] = res.result
	}
	if geocodeErr != nil {
		return nil, geocodeErr
	}

	return out, nil
}
