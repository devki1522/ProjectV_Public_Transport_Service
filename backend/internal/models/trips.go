package models

type Trip struct {
	ID           int    `gorm:"primaryKey;column:id;uniqueIndex:uq_trips_id_route" json:"id"`
	RouteID      int    `gorm:"column:route_id;not null;uniqueIndex:uq_trips_id_route" json:"route_id"`
	ScheduleType string `gorm:"column:schedule_type;type:varchar(20);not null;check:schedule_type IN ('weekday','weekend','daily')" json:"schedule_type"`
	IsActive     bool   `gorm:"column:is_active;default:true;not null" json:"is_active"`

	Route     Route      `gorm:"foreignKey:RouteID;references:ID;constraint:OnDelete:RESTRICT" json:"route,omitempty"`
	StopTimes []StopTime `gorm:"foreignKey:TripID" json:"stop_times,omitempty"`
}

func (Trip) TableName() string {
	return "trips"
}
