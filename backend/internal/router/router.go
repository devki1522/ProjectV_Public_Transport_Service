// Package router maps HTTP methods and paths to handlers and wires dependencies.
package router

import (
	"encoding/json"
	"net/http"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/handlers"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/services"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func New(db *gorm.DB) http.Handler {
	router := chi.NewRouter()
	router.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	RegisterTransitRoutes(router, handlers.NewTransitRouteHandler(services.NewTransitRouteService(db)))
	return router
}

// RegisterTransitRoutes is also usable with a handler backed by a fake service in tests.
func RegisterTransitRoutes(router chi.Router, handler *handlers.TransitRouteHandler) {
	router.Get("/api/routes", handler.List)
	router.Post("/api/routes", handler.Create)
	router.Get("/api/routes/{id}", handler.Get)
	router.Patch("/api/routes/{id}", handler.Update)
	router.Delete("/api/routes/{id}", handler.Delete)
}
