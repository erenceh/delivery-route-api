package dto

import "time"

type PackageResponse struct {
	PackageID   int        `json:"package_id"`
	Destination string     `json:"destination"`
	LoadedAt    *time.Time `json:"loaded_at"`
	DeliveredAt *time.Time `json:"delivered_at"`
}

type ListPackagesResponse struct {
	Packages []PackageResponse `json:"packages"`
}
