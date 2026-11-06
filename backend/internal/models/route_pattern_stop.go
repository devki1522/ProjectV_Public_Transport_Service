package models

type RoutePatternStop struct {
	PatternID    int `gorm:"primaryKey;column:pattern_id" json:"pattern_id"`
	StopSequence int `gorm:"primaryKey;column:stop_sequence;check:stop_sequence > 0" json:"stop_sequence"`
	StopID       int `gorm:"column:stop_id;not null;index:idx_route_pattern_stops_stop_id" json:"stop_id"`

	Pattern RoutePattern `gorm:"foreignKey:PatternID;references:ID;constraint:OnDelete:CASCADE" json:"pattern,omitempty"`
	Stop    Stop         `gorm:"foreignKey:StopID;references:ID;constraint:OnDelete:RESTRICT" json:"stop,omitempty"`
}

func (RoutePatternStop) TableName() string {
	return "route_pattern_stops"
}
