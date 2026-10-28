package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/database"
	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
)

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

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: newRouter(),
	}

	log.Printf("Backend running on port %d", cfg.Server.Port)
	log.Fatal(server.ListenAndServe())
}

func newRouter() http.Handler {
	router := chi.NewRouter()
	router.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
		})
	})
	return router
}
