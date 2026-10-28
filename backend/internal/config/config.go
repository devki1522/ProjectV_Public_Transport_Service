/*
Package config provides a standardized way to get config values from .env files,
while providing fallback values if the variable is not present.
*/
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
}

type ServerConfig struct {
	Port int
}

type DatabaseConfig struct {
	DSN             string
	MaxIdleConns    int
	MaxOpenCons     int
	ConnMaxLifetime time.Duration
}

func Load() (Config, error) {
	port, err := strconv.Atoi(getEnv("PORT", "8080"))
	if err != nil {
		return Config{}, fmt.Errorf("PORT: %w", err)
	}

	maxIdle, err := strconv.Atoi(getEnv("DB_MAX_IDLE_CONNS", "5"))
	if err != nil {
		return Config{}, fmt.Errorf("DB_MAX_IDLE_CONNS: %w", err)
	}

	maxOpen, err := strconv.Atoi(getEnv("DB_MAX_OPEN_CONNS", "20"))
	if err != nil {
		return Config{}, fmt.Errorf("DB_MAX_OPEN_CONNS: %w", err)
	}

	lifetime, err := time.ParseDuration(getEnv("DB_CONN_MAX_LIFETIME", "1h"))
	if err != nil {
		return Config{}, fmt.Errorf("DB_CONN_MAX_LIFETIME: %w", err)
	}

	if port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("PORT must be between 1 and 65535")
	}

	if maxIdle < 0 || maxOpen <= 0 || maxIdle > maxOpen || lifetime <= 0 {
		return Config{}, fmt.Errorf("invalid database pool configuration")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	return Config{
		Server: ServerConfig{
			Port: port,
		},
		Database: DatabaseConfig{
			DSN:             dsn,
			MaxIdleConns:    maxIdle,
			MaxOpenCons:     maxOpen,
			ConnMaxLifetime: lifetime,
		},
	}, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
