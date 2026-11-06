package models

type Vehicle struct {
	ID           int    `gorm:"size:32;primaryKey;generated:identity always;column:id" json:"id"`
	Number       string `gorm:"column:number;type:varchar(20);not null;unique" json:"number"`
	IsActive     bool   `gorm:"column:is_active;default:true;not null" json:"is_active"`
	IsAccessible bool   `gorm:"column:is_accessible;default:false;not null" json:"is_accessible"`
}

func (Vehicle) TableName() string {
	return "vehicles"
}
