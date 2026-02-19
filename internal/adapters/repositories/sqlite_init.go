package repositories

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Initialize the SQLite database schema.
func InitSchema(db *sql.DB) error {
	if db == nil {
		return errors.New("init schema: DB is nil")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("init schema: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	createPackagesQuery := `
	CREATE TABLE IF NOT EXISTS packages (
		package_id INTEGER PRIMARY KEY,
		destination TEXT NOT NULL
	);
	`

	createDistanceCacheQuery := `
	CREATE TABLE IF NOT EXISTS distance_cache (
        origin TEXT NOT NULL,
        destination TEXT NOT NULL,
        distance_meters INTEGER NOT NULL,
        duration_seconds INTEGER NOT NULL,
        PRIMARY KEY (origin, destination)
    );
	`

	createGeocodeCacheQuery := `
	CREATE TABLE IF NOT EXISTS geocode_cache (
        address TEXT PRIMARY KEY,
        lon REAL NOT NULL,
        lat REAL NOT NULL
    );
	`

	createIndexQuery := `
	CREATE INDEX IF NOT EXISTS idx_distance_cache_destination_origin
    ON distance_cache(destination, origin);
	`

	statements := []string{
		createPackagesQuery,
		createDistanceCacheQuery,
		createGeocodeCacheQuery,
		createIndexQuery,
	}

	for i, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("init schema: exec statement #%d: %w", i+1, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("init schema: commit tx: %w", err)
	}

	return nil
}

type PackageSeed struct {
	PackageID   int    `json:"package_id"`
	Destination string `json:"destination"`
}

// Populate the database with package data from a JSON file.
func SeedFromJSON(db *sql.DB, jsonPath string) error {
	bytes, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("seed packages: read %q: %w", jsonPath, err)
	}

	var data []PackageSeed
	if err := json.Unmarshal(bytes, &data); err != nil {
		return fmt.Errorf("seed packages: parse json: %w", err)
	}

	rows := make([]PackageSeed, 0, len(data))
	for i, item := range data {
		packageID := item.PackageID
		if packageID <= 0 {
			return fmt.Errorf("seed packages: invalid packageID at index %d: %d", i+1, packageID)
		}

		dest := strings.TrimSpace(item.Destination)
		if dest == "" {
			return fmt.Errorf("seed packages: item dest at index %d: destination cannot be empty", i+1)
		}
		rows = append(rows, PackageSeed{PackageID: packageID, Destination: dest})
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("seed packages: begin tx: %w", err)
	}
	defer tx.Rollback()

	query := `
	INSERT OR REPLACE INTO packages (
		package_id,
		destination
	)
	VALUES (?, ?);
	`
	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("seed packages: prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, p := range rows {
		if _, err := stmt.Exec(p.PackageID, p.Destination); err != nil {
			return fmt.Errorf("seed packages: insert package_id=%d: %w", p.PackageID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("seed packages: commit tx: %w", err)
	}

	return nil
}
