package cache

import (
	"database/sql"
	"delivery-route-service/internal/domain"
	"errors"
	"fmt"
	"strings"
)

// SQLite backed cache mapping address strings to geographic coordinates.
// Address keys are expected to be consistent (e.g., normalized)
// by the caller.
type SqliteGeocodeCache struct {
	DB *sql.DB
}

func NewSqliteGeocodeCache(db *sql.DB) *SqliteGeocodeCache {
	return &SqliteGeocodeCache{DB: db}
}

// Fetch cached coordinates for the given addresses.
func (s *SqliteGeocodeCache) GetMany(addresses []string) (map[string]domain.Coordinates, error) {
	if s.DB == nil {
		return nil, errors.New("geocode cache: db is nil")
	}

	if len(addresses) == 0 {
		return map[string]domain.Coordinates{}, nil
	}

	seen := map[string]struct{}{}
	uniq := make([]string, 0, len(addresses))
	ph := make([]string, 0, len(addresses))
	for _, a := range addresses {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}

		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		uniq = append(uniq, a)
		ph = append(ph, "?")
	}

	if len(uniq) == 0 {
		return map[string]domain.Coordinates{}, nil
	}

	placeholders := strings.Join(ph, ",")
	args := make([]any, 0, len(uniq))
	for _, a := range uniq {
		args = append(args, a)
	}

	// SQLite does not support binding slices directly in an IN (...) clause.
	// Only the placeholder structure is interpolated; all values remain parameterized.
	q := fmt.Sprintf(`
	SELECT 
        address,
        lon,
        lat
    FROM geocode_cache
    WHERE address IN (%s);
	`, placeholders)

	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("get geocode cache: query geocode_cache table: %w", err)
	}
	defer rows.Close()

	out := make(map[string]domain.Coordinates, len(uniq))
	for rows.Next() {
		var addr string
		var lon, lat float64
		if err := rows.Scan(&addr, &lon, &lat); err != nil {
			return nil, fmt.Errorf("get geocode cache: scan rows: %w", err)
		}
		out[addr] = domain.Coordinates{Lon: lon, Lat: lat}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get geocode cache: row iteration: %w", err)
	}

	return out, nil
}

// Store address -> coordinate mappings in the cache.
func (s *SqliteGeocodeCache) PutMany(results map[string]domain.Coordinates) error {
	if s.DB == nil {
		return errors.New("geocode cache: db is nil")
	}

	if len(results) == 0 {
		return nil
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("insert geocode cache: db begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
	INSERT OR REPLACE INTO geocode_cache (
        address,
        lon,
        lat
    )
    VALUES (?, ?, ?);
	`)
	if err != nil {
		return fmt.Errorf("insert geocode cache: db prepare: %w", err)
	}
	defer stmt.Close()

	for addr, c := range results {
		if strings.TrimSpace(addr) == "" {
			return fmt.Errorf("insert geocode cache: empty address key")
		}

		if _, err := stmt.Exec(addr, c.Lon, c.Lat); err != nil {
			return fmt.Errorf("insert geocode cache coord=%q: %w", addr, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("insert geocode cache commit: %w", err)
	}

	return nil
}
