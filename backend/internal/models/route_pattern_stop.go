package models

// RoutePatternStop represents a stop in an ordered route pattern.
type RoutePatternStop struct {
	PatternID    int `gorm:"size:32;primaryKey;autoIncrement:false;not null;column:pattern_id" json:"pattern_id"`
	StopID       int `gorm:"size:32;column:stop_id;not null;index:idx_route_pattern_stops_stop_id" json:"stop_id"`
	StopSequence int `gorm:"size:32;primaryKey;autoIncrement:false;not null;column:stop_sequence;check:stop_sequence > 0" json:"stop_sequence"`

	Pattern RoutePattern `gorm:"foreignKey:PatternID;references:ID;constraint:OnDelete:CASCADE" json:"pattern,omitempty"`
	Stop    Stop         `gorm:"foreignKey:StopID;references:ID;constraint:OnDelete:RESTRICT" json:"stop,omitempty"`
}

// TableName returns the database table used for RoutePatternStop records.
func (RoutePatternStop) TableName() string {
	return "route_pattern_stops"
}
