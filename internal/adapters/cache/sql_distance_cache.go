package cache

import (
	"context"
	"database/sql"
	"delivery-route-service/internal/platform/obs"
	"delivery-route-service/internal/ports"
	"errors"
	"fmt"
	"strings"
)

// SQLDistanceCache is a SQL-backed cache for origin->destination distance results.
type SQLDistanceCache struct {
	DB *sql.DB
}

func NewSQLDistanceCache(db *sql.DB) *SQLDistanceCache {
	return &SQLDistanceCache{DB: db}
}

// Fetch cached distances for one origin and multiple destinations.
func (s *SQLDistanceCache) GetMany(
	ctx context.Context,
	origin string,
	destinations []string,
) (_ map[string]ports.DistanceResult, err error) {
	defer obs.Time(ctx, "distance.cache.GetMany")(&err)

	if s.DB == nil {
		return nil, errors.New("distance cache: db is nil")
	}

	if origin == "" {
		return nil, errors.New("get distance cache: origin must not be empty")
	}

	if len(destinations) == 0 {
		return map[string]ports.DistanceResult{}, nil
	}

	seen := map[string]struct{}{}
	uniq := make([]string, 0, len(destinations))
	for _, d := range destinations {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}

		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		uniq = append(uniq, d)
	}

	if len(uniq) == 0 {
		return map[string]ports.DistanceResult{}, nil
	}

	q := `
	SELECT destination, distance_meters, duration_seconds
    FROM distance_cache
    WHERE origin = $1 
        AND destination = ANY($2::text[]);
	`

	rows, err := s.DB.QueryContext(ctx, q, origin, uniq)
	if err != nil {
		return nil, fmt.Errorf("get distance cache: query distance_cache table: %w", err)
	}
	defer rows.Close()

	out := make(map[string]ports.DistanceResult, len(uniq))
	for rows.Next() {
		var dest string
		var meters, seconds int
		if err := rows.Scan(&dest, &meters, &seconds); err != nil {
			return nil, fmt.Errorf("get distance cache: scan rows: %w", err)
		}
		out[dest] = ports.DistanceResult{
			DistanceMeters:  meters,
			DurationSeconds: seconds,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get distance cache: row iteration: %w", err)
	}

	return out, nil
}

// Store many cached distance results for a single origin.
func (s *SQLDistanceCache) PutMany(
	ctx context.Context,
	origin string,
	results map[string]ports.DistanceResult,
) error {
	if s.DB == nil {
		return errors.New("distance cache: db is nil")
	}

	if origin == "" {
		return errors.New("insert distance cache: origin must not be empty")
	}

	if len(results) == 0 {
		return nil
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("insert distance cache: db begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
	INSERT INTO distance_cache (origin, destination, distance_meters, duration_seconds)
    VALUES ($1, $2, $3, $4)
	ON CONFLICT (origin, destination) DO UPDATE
	SET distance_meters = EXCLUDED.distance_meters,
		duration_seconds = EXCLUDED.duration_seconds;
	`)
	if err != nil {
		return fmt.Errorf("insert distance cache: db prepare: %w", err)
	}
	defer stmt.Close()

	for dest, r := range results {
		if strings.TrimSpace(dest) == "" {
			return fmt.Errorf("insert distance cache: empty destination key")
		}

		if _, err := stmt.ExecContext(ctx, origin, dest, r.DistanceMeters, r.DurationSeconds); err != nil {
			return fmt.Errorf("insert distance cache dest=%q: %w", dest, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("insert distance cache commit: %w", err)
	}

	return nil
}
