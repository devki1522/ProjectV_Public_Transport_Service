// Package router maps HTTP methods and paths to handlers and wires dependencies.
package router

import (
	"encoding/json"
	"net/http"

	_ "github.com/devki1522/ProjectV_Public_Transport_Service/backend/docs"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/handlers"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/services"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/transit"
	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"gorm.io/gorm"
)

// New builds the API router with schedule and route handlers.
func New(db *gorm.DB) http.Handler {
	return NewWithEstimates(db, nil)
}

// NewWithEstimates connects a live estimate source when one is available.
// A nil provider serves scheduled departures with null estimates.
func NewWithEstimates(db *gorm.DB, estimates transit.EstimateProvider) http.Handler {
	router := chi.NewRouter()
	router.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("./doc.json")))
	router.Get("/api/health", health)
	RegisterTransitRoutes(router, handlers.NewTransitRouteHandler(services.NewTransitRouteService(db)))
	RegisterScheduleRoutes(router, handlers.NewScheduleHandler(transit.NewService(transit.NewPostgresRepository(db), estimates)))
	return router
}

// RegisterScheduleRoutes attaches timetable, departure, and stop endpoints to the router.
func RegisterScheduleRoutes(router chi.Router, handler *handlers.ScheduleHandler) {
	router.Get("/api/routes/{id}/schedule", handler.Timetable)
	router.Get("/api/departures", handler.Departures)
	router.Get("/api/stops/{stopID}/departures", handler.StopDepartures)
	router.Get("/api/routes/{routeID}/departures", handler.RouteDepartures)
	router.Get("/api/trips/{tripID}/departures", handler.TripDepartures)
	router.Get("/api/stops/{id}", handler.Stop)
}

// RegisterTransitRoutes is also usable with a handler backed by a fake service in tests.
func RegisterTransitRoutes(router chi.Router, handler *handlers.TransitRouteHandler) {
	router.Get("/api/routes", handler.List)
	router.Post("/api/routes", handler.Create)
	router.Get("/api/routes/{id}", handler.Get)
	router.Patch("/api/routes/{id}", handler.Update)
	router.Delete("/api/routes/{id}", handler.Delete)
}

// health confirms that the HTTP server is running without querying PostgreSQL.
//
// @Summary Check API health
// @Tags health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /api/health [get]
func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
