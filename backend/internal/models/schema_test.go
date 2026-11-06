package models

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type ddlLogger struct {
	logger.Interface
	statements []string
}

func (l *ddlLogger) Trace(_ context.Context, _ time.Time, sql func() (string, int64), _ error) {
	statement, _ := sql()
	l.statements = append(l.statements, statement)
}

// TestSchemaDDL uses the real PostgreSQL migrator without connecting to a server.
// The PGlite integration test consumes its output to compare database catalogs.
func TestSchemaDDL(t *testing.T) {
	log := &ddlLogger{Interface: logger.Default.LogMode(logger.Silent)}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=localhost user=schema_test dbname=schema_test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: log})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	if err := db.Migrator().CreateTable(
		&Route{}, &Stop{}, &RouteStop{}, &Trip{}, &StopTime{}, &Vehicle{},
		&Disruption{}, &DisruptionRoute{}, &RouteShape{}, &RoutePattern{},
		&RoutePatternStop{}, &SeedImport{},
	); err != nil {
		t.Fatal(err)
	}
	tables := 0
	for _, sql := range log.statements {
		if strings.HasPrefix(sql, "CREATE TABLE") {
			tables++
		}
		if strings.Contains(sql, "bigint") || strings.Contains(sql, "serial") {
			t.Errorf("expected INTEGER columns and identity generation: %s", sql)
		}
	}
	if tables != 12 {
		t.Fatalf("generated %d tables; want 12", tables)
	}
	if path := os.Getenv("PROJECTV_SCHEMA_DDL_OUTPUT"); path != "" {
		data, err := json.Marshal(log.statements)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
