package models

type DisruptionRoute struct {
	ID           int  `gorm:"primaryKey;column:id" json:"id"`
	DisruptionID int  `gorm:"column:disruption_id;not null" json:"disruption_id"`
	RouteID      int  `gorm:"column:route_id;not null" json:"route_id"`
	StopID       *int `gorm:"column:stop_id" json:"stop_id,omitempty"`

	Disruption Disruption `gorm:"foreignKey:DisruptionID;references:ID;constraint:OnDelete:CASCADE" json:"disruption,omitempty"`
	Route      Route      `gorm:"foreignKey:RouteID;references:ID;constraint:OnDelete:RESTRICT" json:"route,omitempty"`
	Stop       *Stop      `gorm:"foreignKey:StopID;references:ID;constraint:OnDelete:RESTRICT" json:"stop,omitempty"`
}

func (DisruptionRoute) TableName() string {
	return "disruption_routes"
}
