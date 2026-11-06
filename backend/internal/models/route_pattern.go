package models

type RoutePattern struct {
	ID        int    `gorm:"size:32;primaryKey;generated:identity always;column:id" json:"id"`
	SourceID  string `gorm:"column:source_id;type:text;not null;unique" json:"source_id"`
	RouteID   int    `gorm:"size:32;column:route_id;not null;index:idx_route_patterns_route_id;uniqueIndex:uq_route_patterns_primary,where:is_primary" json:"route_id"`
	ShapeID   int    `gorm:"size:32;column:shape_id;not null;index:idx_route_patterns_shape_id" json:"shape_id"`
	Headsign  string `gorm:"column:headsign;type:text;not null" json:"headsign"`
	IsPrimary bool   `gorm:"column:is_primary;default:false;not null" json:"is_primary"`

	Route        Route              `gorm:"foreignKey:RouteID;references:ID;constraint:OnDelete:RESTRICT" json:"route,omitempty"`
	Shape        RouteShape         `gorm:"foreignKey:ShapeID;references:ID;constraint:OnDelete:RESTRICT" json:"shape,omitempty"`
	PatternStops []RoutePatternStop `gorm:"foreignKey:PatternID;constraint:OnDelete:CASCADE" json:"pattern_stops,omitempty"`
}

func (RoutePattern) TableName() string {
	return "route_patterns"
}
