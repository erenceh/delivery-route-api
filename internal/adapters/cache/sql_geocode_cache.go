package cache

import (
	"context"
	"database/sql"
	"delivery-route-service/internal/domain"
	"delivery-route-service/internal/platform/obs"
	"errors"
	"fmt"
	"strings"
)

// SQLGeocodeCache is a SQL-backed cache mapping addresses to coordinates.
type SQLGeocodeCache struct {
	DB *sql.DB
}

func NewSQLGeocodeCache(db *sql.DB) *SQLGeocodeCache {
	return &SQLGeocodeCache{DB: db}
}

// Fetch cached coordinates for the given addresses.
func (s *SQLGeocodeCache) GetMany(
	ctx context.Context,
	addresses []string,
) (_ map[string]domain.Coordinates, err error) {
	defer obs.Time(ctx, "geocode.cache.GetMany")(&err)

	if s.DB == nil {
		return nil, errors.New("geocode cache: db is nil")
	}

	if len(addresses) == 0 {
		return map[string]domain.Coordinates{}, nil
	}

	seen := map[string]struct{}{}
	uniq := make([]string, 0, len(addresses))
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
	}

	if len(uniq) == 0 {
		return map[string]domain.Coordinates{}, nil
	}

	q := `
	SELECT address, lon, lat
    FROM geocode_cache
    WHERE address = ANY($1::text[]);
	`

	rows, err := s.DB.QueryContext(ctx, q, uniq)
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
func (s *SQLGeocodeCache) PutMany(ctx context.Context, results map[string]domain.Coordinates) error {
	if s.DB == nil {
		return errors.New("geocode cache: db is nil")
	}

	if len(results) == 0 {
		return nil
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("insert geocode cache: db begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
	INSERT INTO geocode_cache (address, lon, lat)
    VALUES ($1, $2, $3)
	ON CONFLICT (address) DO UPDATE
	SET lon = EXCLUDED.lon,
		lat = EXCLUDED.lat;
	`)
	if err != nil {
		return fmt.Errorf("insert geocode cache: db prepare: %w", err)
	}
	defer stmt.Close()

	for addr, c := range results {
		if strings.TrimSpace(addr) == "" {
			return fmt.Errorf("insert geocode cache: empty address key")
		}

		if _, err := stmt.ExecContext(ctx, addr, c.Lon, c.Lat); err != nil {
			return fmt.Errorf("insert geocode cache coord=%q: %w", addr, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("insert geocode cache commit: %w", err)
	}

	return nil
}
