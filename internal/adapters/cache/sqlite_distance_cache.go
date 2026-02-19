package cache

import (
	"database/sql"
	"delivery-route-service/internal/ports"
	"errors"
	"fmt"
	"strings"
)

// SQLite backed cache for origin->destination distance results.
// Keys are expected to be consistent (e.g., already normalized)
// by the caller.
type SqliteDistanceCache struct {
	DB *sql.DB
}

func NewSqliteDistanceCache(db *sql.DB) *SqliteDistanceCache {
	return &SqliteDistanceCache{DB: db}
}

// Fetch cached distances for one origin and multiple destinations.
func (s *SqliteDistanceCache) GetMany(
	origin string,
	destinations []string,
) (map[string]ports.DistanceResult, error) {
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
	ph := make([]string, 0, len(destinations))
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
		ph = append(ph, "?")
	}

	if len(uniq) == 0 {
		return map[string]ports.DistanceResult{}, nil
	}

	placeholders := strings.Join(ph, ",")
	args := make([]any, 0, 1+len(uniq))
	args = append(args, origin)
	for _, d := range uniq {
		args = append(args, d)
	}

	// SQLite does not support binding slices directly in an IN (...) clause.
	// Only the placeholder structure is interpolated; all values remain parameterized.
	q := fmt.Sprintf(`
	SELECT 
        destination,
        distance_meters,
        duration_seconds
    FROM distance_cache
    WHERE origin = ? 
        AND destination IN (%s);
	`, placeholders)

	rows, err := s.DB.Query(q, args...)
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
func (s *SqliteDistanceCache) PutMany(origin string, results map[string]ports.DistanceResult) error {
	if s.DB == nil {
		return errors.New("distance cache: db is nil")
	}

	if origin == "" {
		return errors.New("insert distance cache: origin must not be empty")
	}

	if len(results) == 0 {
		return nil
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("insert distance cache: db begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
	INSERT OR REPLACE INTO distance_cache (
        origin,
        destination,
        distance_meters,
        duration_seconds
    )
    VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("insert distance cache: db prepare: %w", err)
	}
	defer stmt.Close()

	for dest, r := range results {
		if strings.TrimSpace(dest) == "" {
			return fmt.Errorf("insert distance cache: empty destination key")
		}

		if _, err := stmt.Exec(origin, dest, r.DistanceMeters, r.DurationSeconds); err != nil {
			return fmt.Errorf("insert distance cache dest=%q: %w", dest, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("insert distance cache commit: %w", err)
	}

	return nil
}
