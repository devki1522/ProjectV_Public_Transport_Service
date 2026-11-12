// Package router maps HTTP methods and paths to handlers and wires dependencies.
package router

import (
	"encoding/json"
	"net/http"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/auth"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/handlers"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/services"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/transit"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// Dependencies supplies optional and required services to the HTTP router.
type Dependencies struct {
	Estimates      transit.EstimateProvider
	AuthHandler    *auth.Handler
	AuthMiddleware *auth.Middleware
	CSRF           *auth.CSRFProtector
	RequiredRole   string
	AdminHandler   *handlers.AdminHandler
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
// protected route mutations, and administrator endpoints.
func NewWithDependencies(db *gorm.DB, deps Dependencies) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

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
	r.Get("/api/stops/{stopID}/departures", handler.Departures)
	r.Get("/api/routes/{routeID}/departures", handler.Departures)
	r.Get("/api/trips/{tripID}/departures", handler.Departures)
	r.Get("/api/stops/{id}", handler.Stop)
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

func failClosedAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "authentication required"})
	})
}
