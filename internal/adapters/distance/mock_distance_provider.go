package distance

import (
	"context"
	"delivery-route-service/internal/ports"
	"fmt"
)

type MockPair struct {
	From, To string
	Meters   int
	Seconds  int
}

type MockDistanceProvider struct {
	m map[string]ports.DistanceResult
}

func NewMockDistanceProvider(pairs []MockPair) *MockDistanceProvider {
	m := make(map[string]ports.DistanceResult, len(pairs))
	for _, p := range pairs {
		m[p.From+"|"+p.To] = ports.DistanceResult{DistanceMeters: p.Meters, DurationSeconds: p.Seconds}
	}
	return &MockDistanceProvider{m: m}
}

func (p *MockDistanceProvider) GetDistance(ctx context.Context, origin, destination string) (ports.DistanceResult, error) {
	r, ok := p.m[origin+"|"+destination]
	if !ok {
		return ports.DistanceResult{}, fmt.Errorf("missing pair %q -> %q", origin, destination)
	}

	return r, nil
}
