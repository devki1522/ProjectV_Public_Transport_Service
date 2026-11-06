package models

import "encoding/json"

type RouteShape struct {
	ID       int             `gorm:"size:32;primaryKey;generated:identity always;column:id" json:"id"`
	SourceID string          `gorm:"column:source_id;type:text;not null;unique" json:"source_id"`
	Geometry json.RawMessage `gorm:"column:geometry;type:jsonb;not null;check:route_shapes_geometry_check,COALESCE((geometry->>'type' = 'LineString' AND jsonb_typeof(geometry->'coordinates') = 'array' AND jsonb_array_length(geometry->'coordinates') >= 2), FALSE)" json:"geometry"`

	RoutePatterns []RoutePattern `gorm:"foreignKey:ShapeID;constraint:OnDelete:RESTRICT" json:"route_patterns,omitempty"`
}

func (RouteShape) TableName() string {
	return "route_shapes"
}
