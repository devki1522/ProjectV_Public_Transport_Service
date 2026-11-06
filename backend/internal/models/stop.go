package models

type Stop struct {
	ID                 int     `gorm:"size:32;primaryKey;generated:identity always;column:id" json:"id"`
	Name               string  `gorm:"column:name;type:varchar(100);not null" json:"name"`
	Latitude           float64 `gorm:"column:latitude;type:double precision;not null;check:latitude BETWEEN -90 AND 90" json:"latitude"`
	Longitude          float64 `gorm:"column:longitude;type:double precision;not null;check:longitude BETWEEN -180 AND 180" json:"longitude"`
	IsActive           bool    `gorm:"column:is_active;default:true;not null" json:"is_active"`
	IsAccessible       bool    `gorm:"column:is_accessible;default:false;not null" json:"is_accessible"`
	SourceID           *string `gorm:"column:source_id;unique" json:"source_id,omitempty"`
	StopCode           *string `gorm:"column:stop_code" json:"stop_code,omitempty"`
	PlatformCode       *string `gorm:"column:platform_code" json:"platform_code,omitempty"`
	StopURL            *string `gorm:"column:stop_url" json:"stop_url,omitempty"`
	LocationType       int     `gorm:"size:32;column:location_type;default:0;not null;check:location_type BETWEEN 0 AND 4" json:"location_type"`
	ParentStationID    *int    `gorm:"size:32;column:parent_station_id;index:idx_stops_parent_station_id" json:"parent_station_id,omitempty"`
	WheelchairBoarding int     `gorm:"size:32;column:wheelchair_boarding;default:0;not null;check:wheelchair_boarding BETWEEN 0 AND 2" json:"wheelchair_boarding"`

	ParentStation     *Stop              `gorm:"foreignKey:ParentStationID;references:ID;constraint:OnDelete:RESTRICT" json:"parent_station,omitempty"`
	ChildStops        []Stop             `gorm:"foreignKey:ParentStationID;constraint:OnDelete:RESTRICT" json:"child_stops,omitempty"`
	RouteStops        []RouteStop        `gorm:"foreignKey:StopID;constraint:OnDelete:RESTRICT" json:"route_stops,omitempty"`
	RoutePatternStops []RoutePatternStop `gorm:"foreignKey:StopID;constraint:OnDelete:RESTRICT" json:"route_pattern_stops,omitempty"`
	DisruptionRoutes  []DisruptionRoute  `gorm:"foreignKey:StopID;constraint:OnDelete:RESTRICT" json:"disruption_routes,omitempty"`
}

func (Stop) TableName() string {
	return "stops"
}
