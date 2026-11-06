package models

import "time"

type Disruption struct {
	ID            int        `gorm:"size:32;primaryKey;generated:identity always;column:id" json:"id"`
	Type          string     `gorm:"column:type;type:varchar(30);not null;check:type IN ('delay', 'detour', 'cancellation', 'stop_closure', 'road_closure', 'construction', 'emergency', 'traffic_restriction', 'accessibility')" json:"type"`
	Title         string     `gorm:"column:title;type:varchar(100);not null" json:"title"`
	Description   *string    `gorm:"column:description;type:text" json:"description,omitempty"`
	StartTime     time.Time  `gorm:"column:start_time;type:timestamptz;not null" json:"start_time"`
	EndTime       *time.Time `gorm:"column:end_time;type:timestamptz;check:ck_disruptions_time_order,end_time IS NULL OR end_time >= start_time" json:"end_time,omitempty"`
	Status        string     `gorm:"column:status;type:varchar(20);default:'scheduled';not null;check:status IN ('scheduled', 'active', 'resolved')" json:"status"`
	SourceService string     `gorm:"column:source_service;type:varchar(50);not null" json:"source_service"`

	DisruptionRoutes []DisruptionRoute `gorm:"foreignKey:DisruptionID;constraint:OnDelete:CASCADE" json:"disruption_routes,omitempty"`
}

func (Disruption) TableName() string {
	return "disruptions"
}
