package repositories

import (
	"database/sql"
	"delivery-route-service/internal/domain"
	"errors"
	"fmt"
)

// SQLite-backed implementation of the PackageRepository port.
type SqlitePackageRepository struct{ DB *sql.DB }

func NewSqlitePackageRepository(db *sql.DB) *SqlitePackageRepository {
	return &SqlitePackageRepository{DB: db}
}

// Return all packages stored in the database.
func (s *SqlitePackageRepository) ListPackages() ([]*domain.Package, error) {
	if s.DB == nil {
		return nil, errors.New("sqlite package repository: DB is nil")
	}

	query := `
	SELECT
		package_id,
		destination
	FROM packages
	ORDER BY package_id;
	`
	rows, err := s.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("list packages: query packages table: %w", err)
	}
	defer rows.Close()

	packages := make([]*domain.Package, 0, 64)
	for rows.Next() {
		var id int
		var dest string
		err := rows.Scan(&id, &dest)
		if err != nil {
			return nil, fmt.Errorf("list packages: scan row: %w", err)
		}
		packages = append(packages, &domain.Package{PackageID: id, Destination: dest})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list packages: row iteration: %w", err)
	}

	return packages, nil
}
