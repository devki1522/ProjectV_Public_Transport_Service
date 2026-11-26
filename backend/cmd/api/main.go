// Package main starts the public transport service API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/auth"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/budget"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/database"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/disruptions"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/handlers"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/models"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/router"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/services"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/staffhr"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/transit"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/vehicles"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

// @title Public Transport Service API
// @version 1.0
// @description Public resident schedules, cookie-authenticated transit administration, and Bearer JWT service-to-service integrations (SH-S07). Admin browser mutations require an administrator session, a trusted Origin, and X-CSRF-Token from login or /auth/me; /integrations/* endpoints also accept Authorization: Bearer <token> from Registration Service without cookies or CSRF.
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Registration Service service-account JWT formatted as "Bearer <token>" (SH-S07).
//
// main loads configuration, connects to the database, wires authentication, and starts the HTTP server.
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
		&models.VehicleLocation{},
		&models.Disruption{},
		&models.DisruptionRoute{},
		&models.DisruptionImpact{},
		&models.RouteShape{},
		&models.RoutePattern{},
		&models.RoutePatternStop{},
		&models.SeedImport{},
	); err != nil {
		log.Fatalf("migrate database :%v", err)
	}

	database.EnsureSeeded(db)

	handler, cleanup, err := buildRouter(cfg, db)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}()

	log.Printf("Backend running on port %d (env=%s auth_provider=%s)", cfg.Server.Port, cfg.Env, cfg.Auth.Provider)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func buildRouter(cfg config.Config, db *gorm.DB) (http.Handler, func(), error) {
	provider, err := newAuthProvider(cfg)
	if err != nil {
		return nil, nil, err
	}

	validator, err := auth.NewJWTValidator(cfg.Auth.JWTTrust, nil)
	if err != nil {
		return nil, nil, err
	}

	sessions := auth.NewInMemorySessionStore(nil, time.Minute, 10000)
	csrf := auth.NewCSRFProtector(cfg.Auth.TrustedOrigins)
	throttler := auth.NewLoginThrottler(10, time.Minute, cfg.Auth.TrustedProxy, nil)

	authHandler := auth.NewHandler(
		provider,
		validator,
		sessions,
		csrf,
		throttler,
		cfg.Auth.SessionCookie,
		cfg.Auth.CookieSecure,
		cfg.Auth.RequiredRole,
		nil,
	)
	var bearerValidator auth.BearerTokenValidator
	if bv, ok := provider.(auth.BearerTokenValidator); ok {
		bearerValidator = bv
	} else if validator != nil {
		bearerValidator = validator
	}
	authMiddleware := auth.NewMiddlewareWithBearerValidator(
		sessions,
		validator,
		bearerValidator,
		cfg.Auth.SessionCookie,
		cfg.Auth.CookieSecure,
	)
	adminHandler := handlers.NewAdminHandler(services.NewAdminDashboardService(services.NewPostgresDashboardRepository(db)))

	var simulator *vehicles.Simulator
	var vehicleHandler *handlers.VehicleHandler
	var disruptionHandler *handlers.DisruptionHandler
	var estimates transit.EstimateProvider
	if db != nil {
		vehicleService := vehicles.NewService(vehicles.NewPostgresRepository(db))
		estimates = vehicles.NewLiveEstimateProvider(db)
		simulator = vehicles.NewSimulator(vehicleService, vehicles.SimulatorConfig{
			VehicleCount: cfg.Vehicles.SimulateVehicleCount,
			TickInterval: cfg.Vehicles.SimulateTickInterval,
		})
		if cfg.Vehicles.Simulate {
			if _, err := simulator.Start(nil); err != nil {
				log.Printf("vehicle simulator startup warning: %v", err)
			}
		}
		vehicleHandler = handlers.NewVehicleHandler(vehicleService, simulator)
		disruptionHandler = handlers.NewDisruptionHandler(disruptions.NewService(disruptions.NewPostgresRepository(db)))
	}

	staffClient := staffhr.NewClient(cfg.StaffHR, nil)
	staffHandler := handlers.NewStaffHRHandler(staffClient)
	budgetClient := budget.NewClient(cfg.Budget, nil)
	budgetHandler := handlers.NewBudgetHandler(budgetClient)

	h := router.NewWithDependencies(db, router.Dependencies{
		APIVersion:        cfg.Server.APIVersion,
		Estimates:         estimates,
		AuthHandler:       authHandler,
		AuthMiddleware:    authMiddleware,
		CSRF:              csrf,
		RequiredRole:      cfg.Auth.RequiredRole,
		IntegrationRoles:  cfg.Auth.IntegrationRoles,
		AdminHandler:      adminHandler,
		VehicleHandler:    vehicleHandler,
		DisruptionHandler: disruptionHandler,
		StaffHRHandler:    staffHandler,
		BudgetHandler:     budgetHandler,
	})

	cleanup := func() {
		if simulator != nil {
			simulator.Close()
		}
		sessions.Close()
	}

	return h, cleanup, nil
}

func newAuthProvider(cfg config.Config) (auth.Provider, error) {
	switch cfg.Auth.Provider {
	case config.AuthProviderMock:
		return auth.NewMockProvider(cfg.Auth.JWTTrust, cfg.Auth.Mock, nil)
	case config.AuthProviderRegistration:
		httpClient := &http.Client{Timeout: 10 * time.Second}
		return auth.NewRegistrationProvider(cfg.Auth.Registration, httpClient)
	default:
		return nil, fmt.Errorf("unsupported auth provider %q", cfg.Auth.Provider)
	}
}

func newRouter(db *gorm.DB) http.Handler {
	return router.New(db)
}
