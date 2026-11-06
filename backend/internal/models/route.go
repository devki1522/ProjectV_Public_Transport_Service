package models

type Route struct {
	ID        int     `gorm:"primaryKey;column:id" json:"id"`
	Number    string  `gorm:"column:number;type:varchar(20);not null;uniqueIndex:uq_routes_number_direction" json:"number"`
	Name      string  `gorm:"column:name;type:varchar(100);not null" json:"name"`
	Direction string  `gorm:"column:direction;type:varchar(20);not null;uniqueIndex:uq_routes_number_direction;check:direction IN ('northbound', 'southbound', 'eastbound', 'westbound', 'direction_0', 'direction_1')" json:"direction"`
	IsActive  bool    `gorm:"column:is_active;default:true;not null" json:"is_active"`
	SourceID  *string `gorm:"column:source_id;unique" json:"source_id,omitempty"`
	RouteType *int    `gorm:"column:route_type;check:route_type >= 0" json:"route_type,omitempty"`

	RouteStops       []RouteStop       `gorm:"foreignKey:RouteID" json:"route_stops,omitempty"`
	Trips            []Trip            `gorm:"foreignKey:RouteID" json:"trips,omitempty"`
	RoutePatterns    []RoutePattern    `gorm:"foreignKey:RouteID" json:"route_patterns,omitempty"`
	DisruptionRoutes []DisruptionRoute `gorm:"foreignKey:RouteID" json:"disruption_routes,omitempty"`
}

func (Route) TableName() string {
	return "routes"
}
