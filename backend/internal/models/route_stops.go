package models

// RouteStop represents a stop occurrence within a route.
type RouteStop struct {
	ID           int `gorm:"size:32;primaryKey;generated:identity always;column:id;uniqueIndex:uq_route_stops_id_route" json:"id"`
	RouteID      int `gorm:"size:32;column:route_id;not null;uniqueIndex:uq_route_stops_id_route;uniqueIndex:uq_route_stops_sequence" json:"route_id"`
	StopID       int `gorm:"size:32;column:stop_id;not null;index:idx_route_stops_stop_id" json:"stop_id"`
	StopSequence int `gorm:"size:32;column:stop_sequence;not null;check:stop_sequence > 0;uniqueIndex:uq_route_stops_sequence" json:"stop_sequence"`

	Route     Route      `gorm:"foreignKey:RouteID;references:ID;constraint:OnDelete:RESTRICT" json:"route,omitempty"`
	Stop      Stop       `gorm:"foreignKey:StopID;references:ID;constraint:OnDelete:RESTRICT" json:"stop,omitempty"`
	StopTimes []StopTime `gorm:"foreignKey:RouteStopID,RouteID;references:ID,RouteID;constraint:fk_stop_times_route_stop_route,OnDelete:RESTRICT" json:"stop_times,omitempty"`
}

// TableName returns the database table used for RouteStop records.
func (RouteStop) TableName() string {
	return "route_stops"
}
