// Package models defines the database records used by the transit API.
package models

// Route represents a transit route and its related schedule records.
type Route struct {
	ID        int     `gorm:"size:32;primaryKey;generated:identity always;column:id" json:"id"`
	Number    string  `gorm:"column:number;type:varchar(20);not null;uniqueIndex:uq_routes_number_direction" json:"number"`
	Name      string  `gorm:"column:name;type:varchar(100);not null" json:"name"`
	Direction string  `gorm:"column:direction;type:varchar(20);not null;uniqueIndex:uq_routes_number_direction;check:direction IN ('direction_0', 'direction_1')" json:"direction"`
	IsActive  bool    `gorm:"column:is_active;default:true;not null" json:"is_active"`
	SourceID  *string `gorm:"column:source_id;unique" json:"source_id,omitempty"`
	RouteType *int    `gorm:"size:32;column:route_type;check:route_type >= 0" json:"route_type,omitempty"`

	RouteStops       []RouteStop       `gorm:"foreignKey:RouteID;constraint:OnDelete:RESTRICT" json:"route_stops,omitempty"`
	Trips            []Trip            `gorm:"foreignKey:RouteID;constraint:OnDelete:RESTRICT" json:"trips,omitempty"`
	RoutePatterns    []RoutePattern    `gorm:"foreignKey:RouteID;constraint:OnDelete:RESTRICT" json:"route_patterns,omitempty"`
	DisruptionRoutes []DisruptionRoute `gorm:"foreignKey:RouteID;constraint:OnDelete:RESTRICT" json:"disruption_routes,omitempty"`
}

// TableName returns the database table used for Route records.
func (Route) TableName() string {
	return "routes"
}
