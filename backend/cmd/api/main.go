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
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/config"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/database"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/handlers"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/models"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/router"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/services"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

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
		&models.Disruption{},
		&models.DisruptionRoute{},
		&models.RouteShape{},
		&models.RoutePattern{},
		&models.RoutePatternStop{},
		&models.SeedImport{},
	); err != nil {
		log.Fatalf("migrate database :%v", err)
	}

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
		nil,
	)
	authMiddleware := auth.NewMiddleware(sessions, validator, cfg.Auth.SessionCookie, cfg.Auth.CookieSecure)
	adminHandler := handlers.NewAdminHandler(services.NewAdminDashboardService(services.NewPostgresDashboardRepository(db)))

	h := router.NewWithDependencies(db, router.Dependencies{
		AuthHandler:    authHandler,
		AuthMiddleware: authMiddleware,
		CSRF:           csrf,
		RequiredRole:   cfg.Auth.RequiredRole,
		AdminHandler:   adminHandler,
	})

	return h, sessions.Close, nil
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
