package models

import "time"

// VehicleLocation stores the latest reported position and operating status for an active transit vehicle.
type VehicleLocation struct {
	ID            int       `gorm:"size:32;primaryKey;generated:identity always;column:id" json:"id"`
	VehicleID     int       `gorm:"size:32;column:vehicle_id;not null;uniqueIndex:uq_vehicle_locations_vehicle_id" json:"vehicle_id"`
	RouteID       int       `gorm:"size:32;column:route_id;not null;index:idx_vehicle_locations_route_id" json:"route_id"`
	TripID        *int      `gorm:"size:32;column:trip_id;index:idx_vehicle_locations_trip_id" json:"trip_id,omitempty"`
	Latitude      float64   `gorm:"column:latitude;type:double precision;not null;check:latitude BETWEEN -90 AND 90" json:"latitude"`
	Longitude     float64   `gorm:"column:longitude;type:double precision;not null;check:longitude BETWEEN -180 AND 180" json:"longitude"`
	Heading       *float64  `gorm:"column:heading;type:double precision;check:heading >= 0 AND heading <= 360" json:"heading,omitempty"`
	SpeedKmh      *float64  `gorm:"column:speed_kmh;type:double precision;check:speed_kmh >= 0 AND speed_kmh <= 200" json:"speed_kmh,omitempty"`
	Status        string    `gorm:"column:status;type:varchar(20);not null;default:'in_service';check:status IN ('in_service','delayed','layover','out_of_service')" json:"status"`
	SourceService string    `gorm:"column:source_service;type:varchar(100);not null;default:'transit'" json:"source_service"`
	IsSimulated   bool      `gorm:"column:is_simulated;default:false;not null" json:"is_simulated"`
	RecordedAt    time.Time `gorm:"column:recorded_at;type:timestamptz;not null;index:idx_vehicle_locations_recorded_at" json:"recorded_at"`
	ReceivedAt    time.Time `gorm:"column:received_at;type:timestamptz;not null" json:"received_at"`

	Vehicle Vehicle `gorm:"foreignKey:VehicleID;references:ID;constraint:OnDelete:CASCADE" json:"vehicle,omitempty"`
	Route   Route   `gorm:"foreignKey:RouteID;references:ID;constraint:OnDelete:RESTRICT" json:"route,omitempty"`
	Trip    *Trip   `gorm:"foreignKey:TripID;references:ID;constraint:OnDelete:SET NULL" json:"trip,omitempty"`
}

// TableName returns the database table used for VehicleLocation records.
func (VehicleLocation) TableName() string {
	return "vehicle_locations"
}
