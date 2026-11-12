// Package main starts the public transport service API.
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/database"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/models"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/router"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

// @title Public Transport Service API
// @version 1.0
// @description Waterloo Region transit routes, timetables, and departures. Route writes currently have no authentication.
// @BasePath /
//
// main loads configuration, connects to the database, and starts the HTTP server.
func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file found, using environment variables")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := database.Connect(cfg.Database)
	if err != nil {
		log.Fatal(err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()

	if err := db.AutoMigrate(
		&models.Route{},
		&models.Stop{},
		&models.RouteStop{},
		&models.Trip{},
		&models.StopTime{},
		&models.Vehicle{},
		&models.Disruption{},
		&models.DisruptionRoute{},
		&models.RouteShape{},
		&models.RoutePattern{},
		&models.RoutePatternStop{},
		&models.SeedImport{},
	); err != nil {
		log.Fatalf("migrate database :%v", err)
	}

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: newRouter(db),
	}

	log.Printf("Backend running on port %d", cfg.Server.Port)
	log.Fatal(server.ListenAndServe())
}

func newRouter(db *gorm.DB) http.Handler {
	return router.New(db)
}
