package models

type DisruptionRoute struct {
	ID           int  `gorm:"size:32;primaryKey;generated:identity always;column:id" json:"id"`
	DisruptionID int  `gorm:"size:32;column:disruption_id;not null;uniqueIndex:uq_disruption_route_wide,where:stop_id IS NULL;uniqueIndex:uq_disruption_route_stop,where:stop_id IS NOT NULL" json:"disruption_id"`
	RouteID      int  `gorm:"size:32;column:route_id;not null;index:idx_disruption_routes_route_id;uniqueIndex:uq_disruption_route_wide,where:stop_id IS NULL;uniqueIndex:uq_disruption_route_stop,where:stop_id IS NOT NULL" json:"route_id"`
	StopID       *int `gorm:"size:32;column:stop_id;index:idx_disruption_routes_stop_id;uniqueIndex:uq_disruption_route_stop,where:stop_id IS NOT NULL" json:"stop_id,omitempty"`

	Disruption Disruption `gorm:"foreignKey:DisruptionID;references:ID;constraint:OnDelete:CASCADE" json:"disruption,omitempty"`
	Route      Route      `gorm:"foreignKey:RouteID;references:ID;constraint:OnDelete:RESTRICT" json:"route,omitempty"`
	Stop       *Stop      `gorm:"foreignKey:StopID;references:ID;constraint:OnDelete:RESTRICT" json:"stop,omitempty"`
}

func (DisruptionRoute) TableName() string {
	return "disruption_routes"
}
