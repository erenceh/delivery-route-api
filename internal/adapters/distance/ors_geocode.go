package distance

import (
	"context"
	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/platform/obs"
	"encoding/json"
	"fmt"
	"net/http"
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

// geocodeMany resolves addresses individually using OpenRouteService (/geocode/search).
// Calls are deduplicated and may be retried via doWithRetry.
func (o *ORSDistanceProvider) geocodeMany(
	ctx context.Context,
	addresses []string,
) (_ map[string]domain.Coordinates, err error) {
	defer obs.Time(ctx, "ors.geocodeMany")(&err)

	endpoint := o.baseURL + "/geocode/search"

	seen := make(map[string]struct{}, len(addresses))
	unique := make([]string, 0, len(addresses))
	for _, a := range addresses {
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		unique = append(unique, a)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, 5)
	resultsCh := make(chan geocodeResult, len(unique))
	var wg sync.WaitGroup

	for _, a := range unique {
		wg.Add(1)
		go func(addr string) {
			sem <- struct{}{}
			defer wg.Done()
			defer func() { <-sem }()

			norm := o.normalize(addr)

			resp, e := o.doWithRetry(ctx, func() (*http.Request, error) {
				req, err := o.newRequest(ctx, http.MethodGet, endpoint, nil)
				if err != nil {
					return nil, err
				}
				q := req.URL.Query()
				q.Set("text", norm)
				q.Set("boundary.country", "US")
				q.Set("size", "1")
				req.URL.RawQuery = q.Encode()
				return req, nil
			})
			if e != nil {
				resultsCh <- geocodeResult{address: addr, err: fmt.Errorf("execute request: %w", e)}
				cancel()
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				resultsCh <- geocodeResult{address: addr, err: fmt.Errorf("unexpected status: %d", resp.StatusCode)}
				cancel()
				return
			}

			var decoded geocodeResponse
			if e := json.NewDecoder(resp.Body).Decode(&decoded); e != nil {
				resultsCh <- geocodeResult{address: addr, err: fmt.Errorf("decode geocode response: %w", e)}
				cancel()
				return
			}

			if len(decoded.Features) == 0 {
				resultsCh <- geocodeResult{address: addr, err: fmt.Errorf("no geocode results for %q", addr)}
				cancel()
				return
			}

			coords := decoded.Features[0].Geometry.Coordinates
			if len(coords) != 2 {
				resultsCh <- geocodeResult{address: addr, err: fmt.Errorf("invalid coordinate format for %q", addr)}
				cancel()
				return
			}

			resultsCh <- geocodeResult{
				address: addr,
				result:  domain.Coordinates{Lon: coords[0], Lat: coords[1]},
			}
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
		out[o.normalize(res.address)] = res.result
	}
	if geocodeErr != nil {
		return nil, geocodeErr
	}

	return out, nil
}
