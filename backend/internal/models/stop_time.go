package models

// StopTime represents the scheduled arrival and departure at a route stop.
type StopTime struct {
	ID            int    `gorm:"size:32;primaryKey;generated:identity always;column:id" json:"id"`
	TripID        int    `gorm:"size:32;column:trip_id;not null;uniqueIndex:uq_stop_times_trip_route_stop" json:"trip_id"`
	RouteStopID   int    `gorm:"size:32;column:route_stop_id;not null;index:idx_stop_times_route_stop_id;uniqueIndex:uq_stop_times_trip_route_stop" json:"route_stop_id"`
	RouteID       int    `gorm:"size:32;column:route_id;not null" json:"route_id"`
	ArrivalTime   string `gorm:"column:arrival_time;type:interval;not null;check:ck_stop_times_arrival_nonnegative,arrival_time >= INTERVAL '0 seconds'" json:"arrival_time"`
	DepartureTime string `gorm:"column:departure_time;type:interval;not null;check:ck_stop_times_departure_order,departure_time >= arrival_time" json:"departure_time"`

	Trip      Trip      `gorm:"foreignKey:TripID,RouteID;references:ID,RouteID;constraint:fk_stop_times_trip_route,OnDelete:CASCADE" json:"trip,omitempty"`
	RouteStop RouteStop `gorm:"foreignKey:RouteStopID,RouteID;references:ID,RouteID;constraint:fk_stop_times_route_stop_route,OnDelete:RESTRICT" json:"route_stop,omitempty"`
	// RouteID is already constrained through both composite foreign keys.
	Route Route `gorm:"foreignKey:RouteID;references:ID;constraint:-" json:"route,omitempty"`
}

// TableName returns the database table used for StopTime records.
func (StopTime) TableName() string {
	return "stop_times"
}
