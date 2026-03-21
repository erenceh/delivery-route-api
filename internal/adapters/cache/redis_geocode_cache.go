package cache

import (
	"context"
	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/platform/obs"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisGeocodeCache is a Redis-backed cache mapping addresses to coordinates.
type RedisGeocodeCache struct {
	client *redis.Client
}

func NewRedisGeocodeCache(client *redis.Client) *RedisGeocodeCache {
	return &RedisGeocodeCache{client: client}
}

// Fetch cached coordinates for the given addresses.
func (r *RedisGeocodeCache) GetMany(
	ctx context.Context,
	addresses []string,
) (_ map[string]domain.Coordinates, err error) {
	defer obs.Time(ctx, "geocode.cache.GetMany")(&err)

	if r.client == nil {
		return nil, errors.New("geocode cache: db is nil")
	}
	if len(addresses) == 0 {
		return map[string]domain.Coordinates{}, nil
	}

	unique := make([]string, 0, len(addresses))
	seen := map[string]struct{}{}
	for _, addr := range addresses {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		unique = append(unique, addr)
	}

	keys := make([]string, 0, len(unique))
	uniqueAddrs := make([]string, 0, len(unique))
	for _, addr := range unique {
		keys = append(keys, fmt.Sprintf("geocode:%s", addr))
		uniqueAddrs = append(uniqueAddrs, addr)
	}

	// pipeline all Gets in one round trip
	pipe := r.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.Get(ctx, key)
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, fmt.Errorf("geocode cache pipeline exec: %w", err)
	}

	out := make(map[string]domain.Coordinates, len(addresses))
	for i, cmd := range cmds {
		val, err := cmd.Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("geocode cache get %q: %w", keys[i], err)
		}

		var result domain.Coordinates
		if err := json.Unmarshal([]byte(val), &result); err != nil {
			return nil, fmt.Errorf("geocode cache unmarshal %q: %w", keys[i], err)
		}

		out[uniqueAddrs[i]] = result
	}

	return out, nil
}

// Store address -> coordinate mappings in the cache.
func (r *RedisGeocodeCache) PutMany(ctx context.Context, results map[string]domain.Coordinates) error {
	if r.client == nil {
		return errors.New("geocode cache: db is nil")
	}
	if len(results) == 0 {
		return nil
	}

	unique := make([]string, 0, len(results))
	seen := map[string]struct{}{}
	for addr := range results {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		unique = append(unique, addr)
	}

	keys := make([]string, 0, len(unique))
	uniqueAddrs := make([]string, 0, len(unique))
	for _, addr := range unique {
		keys = append(keys, fmt.Sprintf("geocode:%s", addr))
		uniqueAddrs = append(uniqueAddrs, addr)
	}

	pipe := r.client.Pipeline()
	for i, key := range keys {
		val, err := json.Marshal(results[uniqueAddrs[i]])
		if err != nil {
			return fmt.Errorf("geocode cache marshal %q: %w", key, err)
		}
		pipe.Set(ctx, key, val, 24*time.Hour)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("geocode cache pipeline exec: %w", err)
	}

	return nil
}
