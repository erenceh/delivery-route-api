package dto

import "time"

type PlanRequest struct {
	Hub           string     `json:"hub"`
	DepartAt      *time.Time `json:"depart_at"`
	ReturnToStart bool       `json:"return_to_start"`
	TruckCount    int        `json:"truck_count"`
	TruckCapacity int        `json:"truck_capacity"`
}

type PlanStopResponse struct {
	Destination string    `json:"destination"`
	ArriveAt    time.Time `json:"arrive_at"`
	PackageIDs  []int     `json:"package_ids"`
}

type PlanResponse struct {
	TruckID              int                `json:"truck_id"`
	DepartAt             time.Time          `json:"depart_at"`
	TotalDistanceMeters  int                `json:"total_distance_meters"`
	TotalDurationSeconds int                `json:"total_duration_seconds"`
	Stops                []PlanStopResponse `json:"stops"`
}

type ListPlanResponse struct {
	Plans []PlanResponse `json:"plans"`
}
