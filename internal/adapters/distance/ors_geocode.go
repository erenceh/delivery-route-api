package distance

import (
	"context"
	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/platform/obs"
	"encoding/json"
	"fmt"
	"net/http"
)

type geocodeResponse struct {
	Features []struct {
		Geometry struct {
			Coordinates []float64 `json:"coordinates"`
		} `json:"geometry"`
	} `json:"features"`
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
	out := make(map[string]domain.Coordinates)
	for _, a := range addresses {
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}

		req, err := o.newRequest(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("get geocode request: %w", err)
		}

		norm := o.normalize(a)

		q := req.URL.Query()
		q.Set("text", norm)
		q.Set("boundary.country", "US")
		q.Set("size", "1")
		req.URL.RawQuery = q.Encode()

		resp, err := o.doWithRetry(ctx, func() (*http.Request, error) {
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
		if err != nil {
			return nil, fmt.Errorf("execute request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
		}

		var decoded geocodeResponse
		if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
			return nil, fmt.Errorf("decode geocode response: %w", err)
		}

		if len(decoded.Features) == 0 {
			return nil, fmt.Errorf("no geocode results for %q", a)
		}

		coords := decoded.Features[0].Geometry.Coordinates

		if len(coords) != 2 {
			return nil, fmt.Errorf("invalid coordinate format for %q", a)
		}

		out[norm] = domain.Coordinates{
			Lon: coords[0],
			Lat: coords[1],
		}
	}

	return out, nil
}
