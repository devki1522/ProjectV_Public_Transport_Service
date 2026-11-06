package models

type StopTime struct {
	ID            int    `gorm:"primaryKey;column:id" json:"id"`
	TripID        int    `gorm:"column:trip_id;not null;uniqueIndex:uq_stop_times_trip_route_stop" json:"trip_id"`
	RouteStopID   int    `gorm:"column:route_stop_id;not null;uniqueIndex:uq_stop_times_trip_route_stop" json:"route_stop_id"`
	RouteID       int    `gorm:"column:route_id;not null" json:"route_id"`
	ArrivalTime   string `gorm:"column:arrival_time;type:interval;not null;check:arrival_time >= INTERVAL '0 seconds'" json:"arrival_time"`
	DepartureTime string `gorm:"column:departure_time;type:interval;not null;check:departure_time >= arrival_time" json:"departure_time"`

	Trip      Trip      `gorm:"foreignKey:TripID;references:ID;constraint:OnDelete:CASCADE" json:"trip,omitempty"`
	RouteStop RouteStop `gorm:"foreignKey:RouteStopID;references:ID;constraint:OnDelete:RESTRICT" json:"route_stop,omitempty"`
	Route     Route     `gorm:"foreignKey:RouteID;references:ID" json:"route,omitempty"`
}

func (StopTime) TableName() string {
	return "stop_times"
}
