package models

import "time"

// SeedImport represents a record of an imported seed data set.
type SeedImport struct {
	Name       string    `gorm:"primaryKey;not null;column:name;type:text" json:"name"`
	Sha256     string    `gorm:"column:sha256;type:text;not null;check:length(sha256) = 64" json:"sha256"`
	ImportedAt time.Time `gorm:"column:imported_at;type:timestamptz;default:CURRENT_TIMESTAMP;not null" json:"imported_at"`
}

// TableName returns the database table used for SeedImport records.
func (SeedImport) TableName() string {
	return "seed_imports"
}
