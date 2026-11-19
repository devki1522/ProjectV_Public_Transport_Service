// Package router maps HTTP methods and paths to handlers and wires dependencies.
package router

import (
	"encoding/json"
	"net/http"
	"strings"

	_ "github.com/devki1522/ProjectV_Public_Transport_Service/backend/docs"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/auth"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/disruptions"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/handlers"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/services"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/staffhr"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/transit"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/vehicles"
	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"gorm.io/gorm"
)

// Dependencies supplies optional and required services to the HTTP router.
type Dependencies struct {
	APIVersion        string
	Estimates         transit.EstimateProvider
	AuthHandler       *auth.Handler
	AuthMiddleware    *auth.Middleware
	CSRF              *auth.CSRFProtector
	RequiredRole      string
	AdminHandler      *handlers.AdminHandler
	VehicleHandler    *handlers.VehicleHandler
	DisruptionHandler *handlers.DisruptionHandler
	StaffHRHandler    *handlers.StaffHRHandler
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

// APIBasePath returns the versioned API base path (/api/transport/{version}), defaulting to v1.
func APIBasePath(version string) string {
	cleaned := strings.Trim(strings.TrimSpace(version), "/")
	if cleaned == "" {
		cleaned = "v1"
	}
	return "/api/transport/" + cleaned
}

// NewWithDependencies registers public resident routes, authentication routes,
// protected route mutations, disruption and external incident endpoints, and administrator endpoints.
func NewWithDependencies(db *gorm.DB, deps Dependencies) http.Handler {
	r := chi.NewRouter()
	r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("./doc.json")))

	r.Route(APIBasePath(deps.APIVersion), func(api chi.Router) {
		api.Get("/health", health)

		RegisterTransitRoutes(
			api,
			handlers.NewTransitRouteHandler(services.NewTransitRouteService(db)),
			deps.AuthMiddleware,
			deps.CSRF,
			deps.RequiredRole,
		)

		RegisterScheduleRoutes(
			api,
			handlers.NewScheduleHandler(transit.NewService(transit.NewPostgresRepository(db), deps.Estimates)),
		)

		vehicleHandler := deps.VehicleHandler
		if vehicleHandler == nil {
			vehicleHandler = handlers.NewVehicleHandler(vehicles.NewService(vehicles.NewPostgresRepository(db)), nil)
		}
		RegisterVehicleRoutes(api, vehicleHandler, deps.AuthMiddleware, deps.CSRF, deps.RequiredRole)

		disruptionHandler := deps.DisruptionHandler
		if disruptionHandler == nil {
			disruptionHandler = handlers.NewDisruptionHandler(disruptions.NewService(disruptions.NewPostgresRepository(db)))
		}
		RegisterDisruptionRoutes(api, disruptionHandler, deps.AuthMiddleware, deps.CSRF, deps.RequiredRole)

		if deps.AuthHandler != nil && deps.AuthMiddleware != nil {
			RegisterAuthRoutes(api, deps.AuthHandler, deps.AuthMiddleware)
		}

		adminHandler := deps.AdminHandler
		if adminHandler == nil && db != nil {
			adminHandler = handlers.NewAdminHandler(services.NewAdminDashboardService(services.NewPostgresDashboardRepository(db)))
		}
		if adminHandler != nil {
			RegisterAdminRoutes(api, adminHandler, deps.AuthMiddleware, deps.RequiredRole)
		}

		staffHandler := deps.StaffHRHandler
		if staffHandler == nil {
			staffHandler = handlers.NewStaffHRHandler(staffhr.NewClient(config.StaffHRConfig{}, nil))
		}
		RegisterStaffHRRoutes(api, staffHandler, deps.AuthMiddleware, deps.CSRF, deps.RequiredRole)
	})

	return r
}

// RegisterAuthRoutes registers /auth/login, /auth/me, and /auth/logout.
func RegisterAuthRoutes(r chi.Router, handler *auth.Handler, authMiddleware *auth.Middleware) {
	r.Post("/auth/login", handler.Login)
	r.With(authMiddleware.RequireAuthenticated).Get("/auth/me", handler.Me)
	r.Post("/auth/logout", handler.Logout)
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
		admin.Get("/admin/health", handler.Health)
		admin.Get("/admin/dashboard", handler.Dashboard)
	})
}

// RegisterScheduleRoutes registers public resident schedule, departure, and stop read endpoints.
func RegisterScheduleRoutes(r chi.Router, handler *handlers.ScheduleHandler) {
	r.Get("/routes/{id}/schedule", handler.Timetable)
	r.Get("/departures", handler.Departures)
	r.Get("/stops/{stopID}/departures", handler.StopDepartures)
	r.Get("/routes/{routeID}/departures", handler.RouteDepartures)
	r.Get("/trips/{tripID}/departures", handler.TripDepartures)
	r.Get("/stops", handler.Stops)
	r.Get("/stops/{id}", handler.Stop)
	r.Get("/journeys", handler.Journeys)
}

// RegisterTransitRoutes registers public route reads and administrator-protected route writes.
func RegisterTransitRoutes(
	r chi.Router,
	handler *handlers.TransitRouteHandler,
	authMiddleware *auth.Middleware,
	csrf *auth.CSRFProtector,
	requiredRole string,
) {
	r.Get("/routes", handler.List)
	r.Get("/routes/{id}", handler.Get)

	r.Group(func(protected chi.Router) {
		if authMiddleware == nil || csrf == nil || requiredRole == "" {
			protected.Use(failClosedAuthMiddleware)
		} else {
			protected.Use(authMiddleware.RequireAuthenticated)
			protected.Use(authMiddleware.RequireRole(requiredRole))
			protected.Use(csrf.RequireMutationCSRF)
		}
		protected.Post("/routes", handler.Create)
		protected.Patch("/routes/{id}", handler.Update)
		protected.Delete("/routes/{id}", handler.Delete)
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
	r.Get("/vehicles", handler.ListActive)
	r.Get("/routes/{id}/vehicles", handler.ListByRoute)
	r.Get("/trips/{id}/vehicles", handler.ListByTrip)
	r.Get("/trips/{id}/arrivals", handler.TripArrivals)
	r.Get("/stops/{id}/arrivals", handler.StopArrivals)
	r.Get("/routes/{id}/shape", handler.RouteShape)
	r.Get("/vehicles/simulator", handler.SimulatorStatus)

	r.Group(func(protected chi.Router) {
		if authMiddleware == nil || csrf == nil || requiredRole == "" {
			protected.Use(failClosedAuthMiddleware)
		} else {
			protected.Use(authMiddleware.RequireAuthenticated)
			protected.Use(authMiddleware.RequireRole(requiredRole))
			protected.Use(csrf.RequireMutationCSRF)
		}
		protected.Post("/vehicles/locations", handler.SubmitLocation)
		protected.Post("/admin/vehicles/simulator/start", handler.StartSimulator)
		protected.Post("/admin/vehicles/simulator/stop", handler.StopSimulator)
		protected.Post("/admin/vehicles/simulator/step", handler.StepSimulator)
		protected.Post("/admin/vehicles/simulator/reset", handler.ResetSimulator)
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
	r.Get("/disruptions", handler.List)
	r.Get("/disruptions/active", handler.ListActive)
	r.Get("/disruptions/affected", handler.AffectedServices)
	r.Get("/disruptions/{id}", handler.Get)
	r.Get("/road-closures", handler.ListRoadClosures)
	r.Get("/service-status", handler.ServiceStatus)
	r.Get("/routes/{id}/status", handler.RouteStatus)
	r.Get("/stops/{id}/status", handler.StopStatus)
	r.Get("/trips/{id}/status", handler.TripStatus)

	r.Group(func(protectedRead chi.Router) {
		if authMiddleware == nil || requiredRole == "" {
			protectedRead.Use(failClosedAuthMiddleware)
		} else {
			protectedRead.Use(authMiddleware.RequireAuthenticated)
			protectedRead.Use(authMiddleware.RequireRole(requiredRole))
		}
		protectedRead.Get("/integrations/service-status", handler.EnterpriseServiceStatus)
	})

	r.Group(func(protectedMutation chi.Router) {
		if authMiddleware == nil || csrf == nil || requiredRole == "" {
			protectedMutation.Use(failClosedAuthMiddleware)
		} else {
			protectedMutation.Use(authMiddleware.RequireAuthenticated)
			protectedMutation.Use(authMiddleware.RequireRole(requiredRole))
			protectedMutation.Use(csrf.RequireMutationCSRF)
		}
		protectedMutation.Post("/disruptions", handler.Create)
		protectedMutation.Patch("/disruptions/{id}", handler.Update)
		protectedMutation.Post("/road-closures", handler.CreateRoadClosure)
		protectedMutation.Post("/integrations/utilities/notices", handler.IngestUtilitiesNotice)
		protectedMutation.Post("/integrations/emergency/incidents", handler.IngestEmergencyIncident)
		protectedMutation.Post("/integrations/security/traffic-alerts", handler.IngestSecurityTrafficAlert)
	})
}

// RegisterStaffHRRoutes registers administrator-only Staff/HR employee count and onboarding routes.
func RegisterStaffHRRoutes(
	r chi.Router,
	handler *handlers.StaffHRHandler,
	authMiddleware *auth.Middleware,
	csrf *auth.CSRFProtector,
	requiredRole string,
) {
	r.Group(func(protectedRead chi.Router) {
		if authMiddleware == nil || requiredRole == "" {
			protectedRead.Use(failClosedAuthMiddleware)
		} else {
			protectedRead.Use(authMiddleware.RequireAuthenticated)
			protectedRead.Use(authMiddleware.RequireRole(requiredRole))
		}
		protectedRead.Get("/admin/staff", handler.Overview)
	})

	r.Group(func(protectedMutation chi.Router) {
		if authMiddleware == nil || csrf == nil || requiredRole == "" {
			protectedMutation.Use(failClosedAuthMiddleware)
		} else {
			protectedMutation.Use(authMiddleware.RequireAuthenticated)
			protectedMutation.Use(authMiddleware.RequireRole(requiredRole))
			protectedMutation.Use(csrf.RequireMutationCSRF)
		}
		protectedMutation.Post("/admin/staff/employees", handler.AddEmployee)
		protectedMutation.Put("/admin/staff/employees", handler.PutEmployee)
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
// @Router /api/transport/v1/health [get]
func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
