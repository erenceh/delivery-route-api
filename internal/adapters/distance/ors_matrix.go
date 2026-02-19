package distance

import (
	"bytes"
	"context"
	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/ports"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
)

type matrixRequest struct {
	Locations    [][]float64 `json:"locations"`
	Destinations []int       `json:"destinations"`
	Metrics      []string    `json:"metrics"`
	Sources      []int       `json:"sources"`
}

type matrixResponse struct {
	Distances [][]*float64 `json:"distances"`
	Durations [][]*float64 `json:"durations"`
}

// fetchMatrixRow retrives distance and duration from one origin to many destinations.
// using the OpenRouteService matrix endpoint.
func (o *ORSDistanceProvider) fetchMatrixRow(
	ctx context.Context,
	originCoord domain.Coordinates,
	destinations []string,
	destinationCoords []domain.Coordinates,
) (map[string]ports.DistanceResult, error) {
	if len(destinations) != len(destinationCoords) {
		return nil, errors.New("destinations and destinationCoords are expected to have the same length")
	}

	if len(destinations) == 0 {
		return map[string]ports.DistanceResult{}, nil
	}

	endpoint := fmt.Sprintf("%s/v2/matrix/%s", o.baseURL, o.profile)

	locations := make([][]float64, 0, 1+len(destinationCoords))
	locations = append(locations, originCoord.CoordsToList())
	for _, c := range destinationCoords {
		locations = append(locations, c.CoordsToList())
	}

	destIdx := make([]int, 0, len(destinationCoords))
	for i := 1; i < len(locations); i++ {
		destIdx = append(destIdx, i)
	}

	bodyObj := matrixRequest{
		Locations:    locations,
		Destinations: destIdx,
		Metrics:      []string{"distance", "duration"},
		Sources:      []int{0},
	}

	payload, err := json.Marshal(bodyObj)
	if err != nil {
		return nil, fmt.Errorf("marshal matrix request: %w", err)
	}

	resp, err := o.doWithRetry(ctx, func() (*http.Request, error) {
		body := bytes.NewReader(payload)
		return o.newRequest(ctx, http.MethodPost, endpoint, body)
	})
	if err != nil {
		return nil, fmt.Errorf("matrix request failed: %w", err)
	}
	defer resp.Body.Close()

	var mr matrixResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("decode matrix response: %w", err)
	}

	if len(mr.Distances) != 1 || len(mr.Durations) != 1 {
		return nil, fmt.Errorf(
			"expected 1 source row; got distances=%d durations=%d",
			len(mr.Distances), len(mr.Durations),
		)
	}

	rowDistances := mr.Distances[0]
	rowDurations := mr.Durations[0]

	if len(rowDistances) != len(destinations) || len(rowDurations) != len(destinations) {
		return nil, fmt.Errorf(
			"row lengths do not match destinations: distances=%d durations=%d destinations=%d",
			len(rowDistances), len(rowDurations), len(destinations),
		)
	}

	out := make(map[string]ports.DistanceResult, len(destinations))
	for i, dest := range destinations {
		metersPtr := rowDistances[i]
		secondsPtr := rowDurations[i]

		if metersPtr == nil || secondsPtr == nil {
			return nil, fmt.Errorf("matrix returned invalid metrics for %q:", dest)
		}

		meters := *metersPtr
		seconds := *secondsPtr

		// ORS returns float metrics; round to nearest integer for domain consistency.
		out[dest] = ports.DistanceResult{
			DistanceMeters:  int(math.Round(meters)),
			DurationSeconds: int(math.Round(seconds)),
		}
	}

	return out, nil
}
