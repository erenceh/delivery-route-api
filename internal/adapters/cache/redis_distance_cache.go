package cache

import (
	"context"
	"delivery-route-service/internal/platform/obs"
	"delivery-route-service/internal/ports"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisDistanceCache is a Redis-backed cache for origin->destination distance results.
type RedisDistanceCache struct {
	client *redis.Client
}

func NewRedisDistanceCache(client *redis.Client) *RedisDistanceCache {
	return &RedisDistanceCache{client: client}
}

// Fetch cached distances for one origin and multiple destinations.
func (r *RedisDistanceCache) GetMany(
	ctx context.Context,
	origin string,
	destinations []string,
) (_ map[string]ports.DistanceResult, err error) {
	defer obs.Time(ctx, "distance.cache.GetMany")(&err)

	if r.client == nil {
		return nil, errors.New("distance cache: db is nil")
	}
	if origin == "" {
		return nil, errors.New("get distance cache: origin must not be empty")
	}
	if len(destinations) == 0 {
		return map[string]ports.DistanceResult{}, nil
	}

	unique := make([]string, 0, len(destinations))
	seen := map[string]struct{}{}
	for _, dest := range destinations {
		dest = strings.TrimSpace(dest)
		if dest == "" {
			continue
		}

		if _, ok := seen[dest]; ok {
			continue
		}
		seen[dest] = struct{}{}
		unique = append(unique, dest)
	}

	keys := make([]string, 0, len(unique))
	uniqueDests := make([]string, 0, len(unique))
	for _, dest := range unique {
		keys = append(keys, fmt.Sprintf("distance:%s|%s", origin, dest))
		uniqueDests = append(uniqueDests, dest)
	}

	// pipeline all GETs in one round trip
	pipe := r.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.Get(ctx, key)
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, fmt.Errorf("distance cache pipeline exec: %w", err)
	}

	out := make(map[string]ports.DistanceResult, len(destinations))
	for i, cmd := range cmds {
		val, err := cmd.Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("distance cache get %q: %w", keys[i], err)
		}

		var result ports.DistanceResult
		if err := json.Unmarshal([]byte(val), &result); err != nil {
			return nil, fmt.Errorf("distance cache unmarshal %q: %w", keys[i], err)
		}

		out[uniqueDests[i]] = result
	}

	return out, nil
}

// Store many cached distance results for a single origin.
func (r *RedisDistanceCache) PutMany(
	ctx context.Context,
	origin string,
	results map[string]ports.DistanceResult,
) error {
	if r.client == nil {
		return errors.New("distance cache: db is nil")
	}
	if origin == "" {
		return errors.New("insert distance cache: origin must not be empty")
	}
	if len(results) == 0 {
		return nil
	}

	unique := make([]string, 0, len(results))
	seen := map[string]struct{}{}
	for dest := range results {
		dest = strings.TrimSpace(dest)
		if dest == "" {
			continue
		}
		if _, ok := seen[dest]; ok {
			continue
		}
		seen[dest] = struct{}{}
		unique = append(unique, dest)
	}

	keys := make([]string, 0, len(unique))
	uniqueDests := make([]string, 0, len(unique))
	for _, dest := range unique {
		keys = append(keys, fmt.Sprintf("distance:%s|%s", origin, dest))
		uniqueDests = append(uniqueDests, dest)
	}

	pipe := r.client.Pipeline()
	for i, key := range keys {
		val, err := json.Marshal(results[uniqueDests[i]])
		if err != nil {
			return fmt.Errorf("distance cache marshal %q: %w", key, err)
		}
		pipe.Set(ctx, key, val, 24*time.Hour)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("distance cache pipeline exec: %w", err)
	}

	return nil
}
