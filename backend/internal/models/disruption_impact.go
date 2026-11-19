package models

// DisruptionImpact represents trip, vehicle, or route-segment impact links for a disruption.
type DisruptionImpact struct {
	ID                 int     `gorm:"size:32;primaryKey;generated:identity always;column:id" json:"id"`
	DisruptionID       int     `gorm:"size:32;column:disruption_id;not null;index:idx_disruption_impacts_disruption_id" json:"disruption_id"`
	TripID             *int    `gorm:"size:32;column:trip_id;index:idx_disruption_impacts_trip_id" json:"trip_id,omitempty"`
	VehicleID          *int    `gorm:"size:32;column:vehicle_id;index:idx_disruption_impacts_vehicle_id" json:"vehicle_id,omitempty"`
	SegmentDescription *string `gorm:"column:segment_description;type:varchar(255)" json:"segment_description,omitempty"`

	Disruption Disruption `gorm:"foreignKey:DisruptionID;references:ID;constraint:OnDelete:CASCADE" json:"disruption,omitempty"`
	Trip       *Trip      `gorm:"foreignKey:TripID;references:ID;constraint:OnDelete:SET NULL" json:"trip,omitempty"`
	Vehicle    *Vehicle   `gorm:"foreignKey:VehicleID;references:ID;constraint:OnDelete:SET NULL" json:"vehicle,omitempty"`
}

// TableName returns the database table used for DisruptionImpact records.
func (DisruptionImpact) TableName() string {
	return "disruption_impacts"
}
