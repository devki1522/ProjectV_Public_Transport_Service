package models

type RouteStop struct {
	ID           int `gorm:"primaryKey;column:id;uniqueIndex:uq_route_stops_id_route" json:"id"`
	RouteID      int `gorm:"column:route_id;not null;uniqueIndex:uq_route_stops_id_route;uniqueIndex:uq_route_stops_sequence" json:"route_id"`
	StopID       int `gorm:"column:stop_id;not null" json:"stop_id"`
	StopSequence int `gorm:"column:stop_sequence;not null;check:stop_sequence > 0;uniqueIndex:uq_route_stops_sequence" json:"stop_sequence"`

	Route     Route      `gorm:"foreignKey:RouteID;references:ID;constraint:OnDelete:RESTRICT" json:"route,omitempty"`
	Stop      Stop       `gorm:"foreignKey:StopID;references:ID;constraint:OnDelete:RESTRICT" json:"stop,omitempty"`
	StopTimes []StopTime `gorm:"foreignKey:RouteStopID" json:"stop_times,omitempty"`
}

func (RouteStop) TableName() string {
	return "route_stops"
}
