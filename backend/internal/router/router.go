// Package router maps HTTP methods and paths to handlers and wires dependencies.
package router

import (
	"encoding/json"
	"net/http"

	_ "github.com/devki1522/ProjectV_Public_Transport_Service/backend/docs"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/auth"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/disruptions"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/handlers"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/services"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/transit"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/vehicles"
	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"gorm.io/gorm"
)

// Dependencies supplies optional and required services to the HTTP router.
type Dependencies struct {
	Estimates         transit.EstimateProvider
	AuthHandler       *auth.Handler
	AuthMiddleware    *auth.Middleware
	CSRF              *auth.CSRFProtector
	RequiredRole      string
	AdminHandler      *handlers.AdminHandler
	VehicleHandler    *handlers.VehicleHandler
	DisruptionHandler *handlers.DisruptionHandler
}

// New builds the HTTP router with default dependencies.
// When authentication dependencies are omitted, protected routes fail closed.
func New(db *gorm.DB) http.Handler {
	return NewWithDependencies(db, Dependencies{})
}

// NewWithEstimates connects a live estimate source when one is available.
// A nil provider serves scheduled departures with null estimates.
func NewWithEstimates(db *gorm.DB, estimates transit.EstimateProvider) http.Handler {
	return NewWithDependencies(db, Dependencies{Estimates: estimates})
}

// NewWithDependencies registers public resident routes, authentication routes,
// protected route mutations, disruption and external incident endpoints, and administrator endpoints.
func NewWithDependencies(db *gorm.DB, deps Dependencies) http.Handler {
	r := chi.NewRouter()
	r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("./doc.json")))
	r.Get("/api/health", health)

	RegisterTransitRoutes(
		r,
		handlers.NewTransitRouteHandler(services.NewTransitRouteService(db)),
		deps.AuthMiddleware,
		deps.CSRF,
		deps.RequiredRole,
	)

	RegisterScheduleRoutes(
		r,
		handlers.NewScheduleHandler(transit.NewService(transit.NewPostgresRepository(db), deps.Estimates)),
	)

	vehicleHandler := deps.VehicleHandler
	if vehicleHandler == nil {
		vehicleHandler = handlers.NewVehicleHandler(vehicles.NewService(vehicles.NewPostgresRepository(db)), nil)
	}
	RegisterVehicleRoutes(r, vehicleHandler, deps.AuthMiddleware, deps.CSRF, deps.RequiredRole)

	disruptionHandler := deps.DisruptionHandler
	if disruptionHandler == nil {
		disruptionHandler = handlers.NewDisruptionHandler(disruptions.NewService(disruptions.NewPostgresRepository(db)))
	}
	RegisterDisruptionRoutes(r, disruptionHandler, deps.AuthMiddleware, deps.CSRF, deps.RequiredRole)

	if deps.AuthHandler != nil && deps.AuthMiddleware != nil {
		RegisterAuthRoutes(r, deps.AuthHandler, deps.AuthMiddleware)
	}

	adminHandler := deps.AdminHandler
	if adminHandler == nil && db != nil {
		adminHandler = handlers.NewAdminHandler(services.NewAdminDashboardService(services.NewPostgresDashboardRepository(db)))
	}
	if adminHandler != nil {
		RegisterAdminRoutes(r, adminHandler, deps.AuthMiddleware, deps.RequiredRole)
	}

	return r
}

// RegisterAuthRoutes registers /api/auth/login, /api/auth/me, and /api/auth/logout.
func RegisterAuthRoutes(r chi.Router, handler *auth.Handler, authMiddleware *auth.Middleware) {
	r.Post("/api/auth/login", handler.Login)
	r.With(authMiddleware.RequireAuthenticated).Get("/api/auth/me", handler.Me)
	r.Post("/api/auth/logout", handler.Logout)
}

// RegisterAdminRoutes registers administrator-only endpoints protected by authentication and role checks.
func RegisterAdminRoutes(r chi.Router, handler *handlers.AdminHandler, authMiddleware *auth.Middleware, requiredRole string) {
	r.Group(func(admin chi.Router) {
		if authMiddleware == nil || requiredRole == "" {
			admin.Use(failClosedAuthMiddleware)
		} else {
			admin.Use(authMiddleware.RequireAuthenticated)
			admin.Use(authMiddleware.RequireRole(requiredRole))
		}
		admin.Get("/api/admin/health", handler.Health)
		admin.Get("/api/admin/dashboard", handler.Dashboard)
	})
}

// RegisterScheduleRoutes registers public resident schedule, departure, and stop read endpoints.
func RegisterScheduleRoutes(r chi.Router, handler *handlers.ScheduleHandler) {
	r.Get("/api/routes/{id}/schedule", handler.Timetable)
	r.Get("/api/departures", handler.Departures)
	r.Get("/api/stops/{stopID}/departures", handler.StopDepartures)
	r.Get("/api/routes/{routeID}/departures", handler.RouteDepartures)
	r.Get("/api/trips/{tripID}/departures", handler.TripDepartures)
	r.Get("/api/stops", handler.Stops)
	r.Get("/api/stops/{id}", handler.Stop)
	r.Get("/api/journeys", handler.Journeys)
}

// RegisterTransitRoutes registers public route reads and administrator-protected route writes.
func RegisterTransitRoutes(
	r chi.Router,
	handler *handlers.TransitRouteHandler,
	authMiddleware *auth.Middleware,
	csrf *auth.CSRFProtector,
	requiredRole string,
) {
	r.Get("/api/routes", handler.List)
	r.Get("/api/routes/{id}", handler.Get)

	r.Group(func(protected chi.Router) {
		if authMiddleware == nil || csrf == nil || requiredRole == "" {
			protected.Use(failClosedAuthMiddleware)
		} else {
			protected.Use(authMiddleware.RequireAuthenticated)
			protected.Use(authMiddleware.RequireRole(requiredRole))
			protected.Use(csrf.RequireMutationCSRF)
		}
		protected.Post("/api/routes", handler.Create)
		protected.Patch("/api/routes/{id}", handler.Update)
		protected.Delete("/api/routes/{id}", handler.Delete)
	})
}

// RegisterVehicleRoutes registers public vehicle location/shape/arrival reads and protected vehicle update and simulator endpoints.
func RegisterVehicleRoutes(
	r chi.Router,
	handler *handlers.VehicleHandler,
	authMiddleware *auth.Middleware,
	csrf *auth.CSRFProtector,
	requiredRole string,
) {
	r.Get("/api/vehicles", handler.ListActive)
	r.Get("/api/routes/{id}/vehicles", handler.ListByRoute)
	r.Get("/api/trips/{id}/vehicles", handler.ListByTrip)
	r.Get("/api/trips/{id}/arrivals", handler.TripArrivals)
	r.Get("/api/stops/{id}/arrivals", handler.StopArrivals)
	r.Get("/api/routes/{id}/shape", handler.RouteShape)
	r.Get("/api/vehicles/simulator", handler.SimulatorStatus)

	r.Group(func(protected chi.Router) {
		if authMiddleware == nil || csrf == nil || requiredRole == "" {
			protected.Use(failClosedAuthMiddleware)
		} else {
			protected.Use(authMiddleware.RequireAuthenticated)
			protected.Use(authMiddleware.RequireRole(requiredRole))
			protected.Use(csrf.RequireMutationCSRF)
		}
		protected.Post("/api/vehicles/locations", handler.SubmitLocation)
		protected.Post("/api/admin/vehicles/simulator/start", handler.StartSimulator)
		protected.Post("/api/admin/vehicles/simulator/stop", handler.StopSimulator)
		protected.Post("/api/admin/vehicles/simulator/step", handler.StepSimulator)
		protected.Post("/api/admin/vehicles/simulator/reset", handler.ResetSimulator)
	})
}

// RegisterDisruptionRoutes registers public disruption and operational status read endpoints,
// protected enterprise service-status reads, and CSRF-protected admin/integration disruption mutations.
func RegisterDisruptionRoutes(
	r chi.Router,
	handler *handlers.DisruptionHandler,
	authMiddleware *auth.Middleware,
	csrf *auth.CSRFProtector,
	requiredRole string,
) {
	r.Get("/api/disruptions", handler.List)
	r.Get("/api/disruptions/active", handler.ListActive)
	r.Get("/api/disruptions/affected", handler.AffectedServices)
	r.Get("/api/disruptions/{id}", handler.Get)
	r.Get("/api/service-status", handler.ServiceStatus)
	r.Get("/api/routes/{id}/status", handler.RouteStatus)
	r.Get("/api/stops/{id}/status", handler.StopStatus)
	r.Get("/api/trips/{id}/status", handler.TripStatus)

	r.Group(func(protectedRead chi.Router) {
		if authMiddleware == nil || requiredRole == "" {
			protectedRead.Use(failClosedAuthMiddleware)
		} else {
			protectedRead.Use(authMiddleware.RequireAuthenticated)
			protectedRead.Use(authMiddleware.RequireRole(requiredRole))
		}
		protectedRead.Get("/api/integrations/service-status", handler.EnterpriseServiceStatus)
	})

	r.Group(func(protectedMutation chi.Router) {
		if authMiddleware == nil || csrf == nil || requiredRole == "" {
			protectedMutation.Use(failClosedAuthMiddleware)
		} else {
			protectedMutation.Use(authMiddleware.RequireAuthenticated)
			protectedMutation.Use(authMiddleware.RequireRole(requiredRole))
			protectedMutation.Use(csrf.RequireMutationCSRF)
		}
		protectedMutation.Post("/api/disruptions", handler.Create)
		protectedMutation.Patch("/api/disruptions/{id}", handler.Update)
		protectedMutation.Post("/api/integrations/utilities/notices", handler.IngestUtilitiesNotice)
		protectedMutation.Post("/api/integrations/emergency/incidents", handler.IngestEmergencyIncident)
		protectedMutation.Post("/api/integrations/security/traffic-alerts", handler.IngestSecurityTrafficAlert)
	})
}

func failClosedAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "authentication required"})
	})
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
