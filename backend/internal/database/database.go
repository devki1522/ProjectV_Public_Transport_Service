// Package database provides database connection setup for the API.
package database

import (
	_ "embed"
	"fmt"
	"log"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

//go:embed seeds/grt.sql
var seedSQL string

// Connect opens and verifies a PostgreSQL connection using the supplied configuration with retries.
func Connect(cfg config.DatabaseConfig) (*gorm.DB, error) {
	var db *gorm.DB
	var err error

	for attempts := 1; attempts <= 30; attempts++ {
		db, err = gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
		if err == nil {
			sqlDB, err := db.DB()
			if err == nil {
				sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
				sqlDB.SetMaxOpenConns(cfg.MaxOpenCons)
				sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

				if err := sqlDB.Ping(); err == nil {
					log.Printf("Successfully connected to database after %d attempts", attempts)
					return db, nil
				}
				sqlDB.Close()
			}
		}
		log.Printf("Waiting for database to be ready (attempt %d/30)...", attempts)
		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("failed to connect to database after 30 attempts: %w", err)
}

// EnsureSeeded loads the GRT database snapshot if the database has not been seeded yet.
func EnsureSeeded(db *gorm.DB) {
	if seedSQL == "" {
		return
	}

	var count int64
	if err := db.Table("routes").Count(&count).Error; err == nil && count == 0 {
		log.Println("Database has no routes. Auto-seeding initial transit data...")
		sqlDB, err := db.DB()
		if err != nil {
			log.Printf("Auto-seed error accessing db: %v", err)
			return
		}
		query := fmt.Sprintf("BEGIN;\nSET LOCAL standard_conforming_strings = on;\nSET LOCAL search_path = public;\nSELECT pg_advisory_xact_lock(714208301);\n%s\nCOMMIT;\n", seedSQL)
		if _, err := sqlDB.Exec(query); err != nil {
			log.Printf("Auto-seed execution failed: %v", err)
		} else {
			log.Println("Database auto-seeded successfully!")
		}
	}
}
