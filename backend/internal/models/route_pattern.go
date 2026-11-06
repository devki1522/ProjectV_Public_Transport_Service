package models

type RoutePattern struct {
	ID        int    `gorm:"primaryKey;column:id" json:"id"`
	SourceID  string `gorm:"column:source_id;type:text;not null;unique" json:"source_id"`
	RouteID   int    `gorm:"column:route_id;not null;index:idx_route_patterns_route_id" json:"route_id"`
	ShapeID   int    `gorm:"column:shape_id;not null;index:idx_route_patterns_shape_id" json:"shape_id"`
	Headsign  string `gorm:"column:headsign;type:text;not null" json:"headsign"`
	IsPrimary bool   `gorm:"column:is_primary;default:false;not null" json:"is_primary"`

	Route        Route              `gorm:"foreignKey:RouteID;references:ID;constraint:OnDelete:RESTRICT" json:"route,omitempty"`
	Shape        RouteShape         `gorm:"foreignKey:ShapeID;references:ID;constraint:OnDelete:RESTRICT" json:"shape,omitempty"`
	PatternStops []RoutePatternStop `gorm:"foreignKey:PatternID" json:"pattern_stops,omitempty"`
}

func (RoutePattern) TableName() string {
	return "route_patterns"
}
