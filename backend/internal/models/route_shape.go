package models

import "encoding/json"

type RouteShape struct {
	ID       int             `gorm:"primaryKey;column:id" json:"id"`
	SourceID string          `gorm:"column:source_id;type:text;not null;unique" json:"source_id"`
	Geometry json.RawMessage `gorm:"column:geometry;type:jsonb;not null" json:"geometry"`

	RoutePatterns []RoutePattern `gorm:"foreignKey:ShapeID" json:"route_patterns,omitempty"`
}

func (RouteShape) TableName() string {
	return "route_shapes"
}
