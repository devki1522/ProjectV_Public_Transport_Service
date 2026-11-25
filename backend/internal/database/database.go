// Package database provides database connection setup for the API.
package database

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

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

// EnsureSeeded loads the GRT database snapshot from disk if the database has no routes yet.
func EnsureSeeded(db *gorm.DB) {
	var count int64
	if err := db.Table("routes").Count(&count).Error; err != nil || count > 0 {
		return
	}

	seedBytes := readSeedFile()
	if len(seedBytes) == 0 {
		return
	}

	log.Println("Database has no routes. Auto-seeding initial transit data...")
	sqlDB, err := db.DB()
	if err != nil {
		log.Printf("Auto-seed error accessing db: %v", err)
		return
	}

	tx, err := sqlDB.Begin()
	if err != nil {
		log.Printf("Auto-seed failed to begin transaction: %v", err)
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, stmt := range []string{
		"SET LOCAL standard_conforming_strings = on",
		"SET LOCAL search_path = public",
		"SELECT pg_advisory_xact_lock(714208301)",
	} {
		if _, err := tx.Exec(stmt); err != nil {
			log.Printf("Auto-seed setup failed: %v", err)
			return
		}
	}

	if _, err := tx.Exec(string(seedBytes)); err != nil {
		log.Printf("Auto-seed execution failed: %v", err)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("Auto-seed commit failed: %v", err)
		return
	}
	log.Println("Database auto-seeded successfully!")
}

func readSeedFile() []byte {
	candidates := []string{"seeds/grt.sql", "../database/seeds/grt.sql"}
	if custom := strings.TrimSpace(os.Getenv("SEED_SQL_PATH")); custom != "" {
		candidates = append([]string{custom}, candidates...)
	}
	for _, path := range candidates {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return data
		}
	}
	return nil
}
