package models

import "time"

type SeedImport struct {
	Name       string    `gorm:"primaryKey;column:name;type:text" json:"name"`
	Sha256     string    `gorm:"column:sha256;type:text;not null;check:length(sha256) = 64" json:"sha256"`
	ImportedAt time.Time `gorm:"column:imported_at;type:timestamptz;default:CURRENT_TIMESTAMP;not null" json:"imported_at"`
}

func (SeedImport) TableName() string {
	return "seed_imports"
}
