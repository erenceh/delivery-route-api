package testutil

import (
	"context"
	"delivery-route-service/internal/domain"
)

type MockPackageRepository struct {
	Packages []*domain.Package
	Err      error
}

func NewMockPackageRepository(packages []*domain.Package, err error) *MockPackageRepository {
	return &MockPackageRepository{Packages: packages, Err: err}
}

func (m *MockPackageRepository) ListPackages(ctx context.Context) ([]*domain.Package, error) {
	return m.Packages, m.Err
}
