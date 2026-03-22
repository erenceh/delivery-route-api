package cache_test

import (
	"context"
	"delivery-route-service/internal/adapters/cache"
	"delivery-route-service/internal/ports"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"testing"
)

func TestRedisDistanceCacheGetMany(t *testing.T) {
	for _, tt := range []struct {
		name         string
		nilClient	 bool
		origin       string
		destinations []string
		seedData	 map[string]string
		wantErr      bool
		wantResult   map[string]ports.DistanceResult
	}{
		{
			name: "error when client is nil",
			nilClient: true,
			origin: "HUB",
			destinations: []string{"DestA"},
			wantErr: true,
		},
		{
			name: "error when origin is empty",
			origin: "",
			destinations: []string{"DestA", "DestB"},
			wantErr: true,
		},
		{
			name: "empty map when no destinations",
			origin: "HUB",
			destinations: []string{},
			wantErr: false,
			wantResult: map[string]ports.DistanceResult{},
		},
		{
			name: "empty map when keys don't exist in cache (cache miss)",
			origin: "HUB",
			destinations: []string{"DestA", "DestB"},
			wantErr: false,
			wantResult: map[string]ports.DistanceResult{},
		},
		{
			name: "returns correct results when keys exist",
			origin: "HUB",
			destinations: []string{"DestA", "DestB"},
			seedData: map[string]string{
				"distance:HUB|DestA": `{"DistanceMeters":100,"DurationSeconds":60}`,
				"distance:HUB|DestB": `{"DistanceMeters":200,"DurationSeconds":120}`,
			},
			wantErr: false,
			wantResult: map[string]ports.DistanceResult{
				"DestA": {DistanceMeters: 100, DurationSeconds: 60},
				"DestB": {DistanceMeters: 200, DurationSeconds: 120},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			mr := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

			var c *cache.RedisDistanceCache
			if tt.nilClient {
				c = cache.NewRedisDistanceCache(nil)
			} else {
				for key, val := range tt.seedData {
					mr.Set(key, val)
				}
				c = cache.NewRedisDistanceCache(client)
			}

			result , err := c.GetMany(ctx, tt.origin, tt.destinations)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result) != len(tt.wantResult) {
				t.Fatalf("expected %d results, got %d", len(tt.wantResult), len(result))
			}
			for dest, want := range tt.wantResult {
				got, ok := result[dest]
				if !ok {
					t.Fatalf("missing result for %q", dest)
				}
				if got != want {
					t.Fatalf("dest %q: expected %+v, got %+v", dest, want, got)
				}
			}
		})
	}
}

func TestRedisDistanceCachePutMany(t *testing.T) {
	for _, tt := range []struct {
		name      string
		nilClient bool
		origin    string
		results   map[string]ports.DistanceResult
		wantErr   bool
	}{
		{
			name:      "error when client is nil",
			nilClient: true,
			origin:    "HUB",
			results:   map[string]ports.DistanceResult{"DestA": {DistanceMeters: 100, DurationSeconds: 60}},
			wantErr:   true,
		},
		{
			name:    "error when origin is empty",
			origin:  "",
			results: map[string]ports.DistanceResult{"DestA": {DistanceMeters: 100, DurationSeconds: 60}},
			wantErr: true,
		},
		{
			name:    "returns nil when results map is empty",
			origin:  "HUB",
			results: map[string]ports.DistanceResult{},
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			mr := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

			var c *cache.RedisDistanceCache
			if tt.nilClient {
				c = cache.NewRedisDistanceCache(nil)
			} else {
				c = cache.NewRedisDistanceCache(client)
			}

			err := c.PutMany(ctx, tt.origin, tt.results)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRedisDistanceCacheRoundTrip(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c := cache.NewRedisDistanceCache(client)

	t.Run("values stored are retrievable by GetMany", func(t *testing.T) {
		ctx := context.Background()
		origin := "HUB"
		stored := map[string]ports.DistanceResult{
			"DestA": {DistanceMeters: 100, DurationSeconds: 60},
			"DestB": {DistanceMeters: 200, DurationSeconds: 120},
		}

		if err := c.PutMany(ctx, origin, stored); err != nil {
			t.Fatalf("PutMany: unexpected error: %v", err)
		}

		destinations := make([]string, 0, len(stored))
		for dest := range stored {
			destinations = append(destinations, dest)
		}

		result, err := c.GetMany(ctx, origin, destinations)
		if err != nil {
			t.Fatalf("GetMany: unexpected error: %v", err)
		}

		if len(result) != len(stored) {
			t.Fatalf("expected %d results, got %d", len(stored), len(result))
		}
		for dest, want := range stored {
			got, ok := result[dest]
			if !ok {
				t.Fatalf("missing result for %q", dest)
			}
			if got != want {
				t.Fatalf("dest %q: expected %+v, got %+v", dest, want, got)
			}
		}
	})
}
