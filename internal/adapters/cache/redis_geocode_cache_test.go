package cache_test

import (
	"context"
	"delivery-route-service/internal/adapters/cache"
	"delivery-route-service/internal/domain"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisGeocodeCacheGetMany(t *testing.T) {
	for _, tt := range []struct {
		name       string
		nilClient  bool
		addresses  []string
		seedData   map[string]string
		wantErr    bool
		wantResult map[string]domain.Coordinates
	}{
		{
			name:      "error when client is nil",
			nilClient: true,
			addresses: []string{"AddrA"},
			wantErr:   true,
		},
		{
			name:       "empty map when no addresses",
			addresses:  []string{},
			wantErr:    false,
			wantResult: map[string]domain.Coordinates{},
		},
		{
			name:       "empty map when keys don't exist in cache (cache miss)",
			addresses:  []string{"AddrA", "AddrB"},
			wantErr:    false,
			wantResult: map[string]domain.Coordinates{},
		},
		{
			name:      "returns correct results when keys exist",
			addresses: []string{"AddrA", "AddrB"},
			seedData: map[string]string{
				"geocode:AddrA": `{"Lon":-112.1,"Lat":33.4}`,
				"geocode:AddrB": `{"Lon":-111.9,"Lat":33.5}`,
			},
			wantErr: false,
			wantResult: map[string]domain.Coordinates{
				"AddrA": {Lon: -112.1, Lat: 33.4},
				"AddrB": {Lon: -111.9, Lat: 33.5},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			mr := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

			var c *cache.RedisGeocodeCache
			if tt.nilClient {
				c = cache.NewRedisGeocodeCache(nil)
			} else {
				for key, val := range tt.seedData {
					mr.Set(key, val)
				}
				c = cache.NewRedisGeocodeCache(client)
			}

			result, err := c.GetMany(ctx, tt.addresses)

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
			for addr, want := range tt.wantResult {
				got, ok := result[addr]
				if !ok {
					t.Fatalf("missing result for %q", addr)
				}
				if got != want {
					t.Fatalf("addr %q: expected %+v, got %+v", addr, want, got)
				}
			}
		})
	}
}

func TestRedisGeocodeCachePutMany(t *testing.T) {
	for _, tt := range []struct {
		name      string
		nilClient bool
		results   map[string]domain.Coordinates
		wantErr   bool
	}{
		{
			name:      "error when client is nil",
			nilClient: true,
			results:   map[string]domain.Coordinates{"AddrA": {Lon: -112.1, Lat: 33.4}},
			wantErr:   true,
		},
		{
			name:    "returns nil when results map is empty",
			results: map[string]domain.Coordinates{},
			wantErr: false,
		},
		{
			name: "stores entries without error",
			results: map[string]domain.Coordinates{
				"AddrA": {Lon: -112.1, Lat: 33.4},
				"AddrB": {Lon: -111.9, Lat: 33.5},
			},
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			mr := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

			var c *cache.RedisGeocodeCache
			if tt.nilClient {
				c = cache.NewRedisGeocodeCache(nil)
			} else {
				c = cache.NewRedisGeocodeCache(client)
			}

			err := c.PutMany(ctx, tt.results)

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

func TestRedisGeocodeCacheRoundTrip(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c := cache.NewRedisGeocodeCache(client)

	t.Run("values stored are retrievable by GetMany", func(t *testing.T) {
		ctx := context.Background()
		stored := map[string]domain.Coordinates{
			"AddrA": {Lon: -112.1, Lat: 33.4},
			"AddrB": {Lon: -111.9, Lat: 33.5},
		}

		if err := c.PutMany(ctx, stored); err != nil {
			t.Fatalf("PutMany: unexpected error: %v", err)
		}

		addresses := make([]string, 0, len(stored))
		for addr := range stored {
			addresses = append(addresses, addr)
		}

		result, err := c.GetMany(ctx, addresses)
		if err != nil {
			t.Fatalf("GetMany: unexpected error: %v", err)
		}

		if len(result) != len(stored) {
			t.Fatalf("expected %d results, got %d", len(stored), len(result))
		}
		for addr, want := range stored {
			got, ok := result[addr]
			if !ok {
				t.Fatalf("missing result for %q", addr)
			}
			if got != want {
				t.Fatalf("addr %q: expected %+v, got %+v", addr, want, got)
			}
		}
	})
}
