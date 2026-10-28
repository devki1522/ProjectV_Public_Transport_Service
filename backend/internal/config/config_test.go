package config

import (
	"strings"
	"testing"
	"time"
)

const testDSN = "postgres://test:test@localhost:5432/test?sslmode=disable"

func setTestEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"PORT", "DB_MAX_IDLE_CONNS", "DB_MAX_OPEN_CONNS", "DB_CONN_MAX_LIFETIME"} {
		t.Setenv(key, "")
	}
	t.Setenv("DATABASE_URL", testDSN)
}

func TestLoadDefaults(t *testing.T) {
	setTestEnv(t)
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		Server: ServerConfig{Port: 8080},
		Database: DatabaseConfig{
			DSN: testDSN, MaxIdleConns: 5, MaxOpenCons: 20, ConnMaxLifetime: time.Hour,
		},
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadOverrides(t *testing.T) {
	setTestEnv(t)
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://custom:custom@localhost/custom")
	t.Setenv("DB_MAX_IDLE_CONNS", "2")
	t.Setenv("DB_MAX_OPEN_CONNS", "10")
	t.Setenv("DB_CONN_MAX_LIFETIME", "30m")
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		Server: ServerConfig{Port: 9090},
		Database: DatabaseConfig{
			DSN: "postgres://custom:custom@localhost/custom", MaxIdleConns: 2,
			MaxOpenCons: 10, ConnMaxLifetime: 30 * time.Minute,
		},
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name, key, value, wantError string
	}{
		{"missing database URL", "DATABASE_URL", "", "DATABASE_URL is required"},
		{"nonnumeric port", "PORT", "abc", "PORT:"},
		{"negative port", "PORT", "-1", "PORT must be between"},
		{"zero port", "PORT", "0", "PORT must be between"},
		{"port above maximum", "PORT", "65536", "PORT must be between"},
		{"nonnumeric idle count", "DB_MAX_IDLE_CONNS", "abc", "DB_MAX_IDLE_CONNS:"},
		{"negative idle count", "DB_MAX_IDLE_CONNS", "-1", "invalid database pool"},
		{"idle exceeds open", "DB_MAX_IDLE_CONNS", "21", "invalid database pool"},
		{"nonnumeric open count", "DB_MAX_OPEN_CONNS", "abc", "DB_MAX_OPEN_CONNS:"},
		{"zero open count", "DB_MAX_OPEN_CONNS", "0", "invalid database pool"},
		{"negative open count", "DB_MAX_OPEN_CONNS", "-1", "invalid database pool"},
		{"invalid lifetime", "DB_CONN_MAX_LIFETIME", "abc", "DB_CONN_MAX_LIFETIME:"},
		{"zero lifetime", "DB_CONN_MAX_LIFETIME", "0s", "invalid database pool"},
		{"negative lifetime", "DB_CONN_MAX_LIFETIME", "-1h", "invalid database pool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTestEnv(t)
			t.Setenv(tt.key, tt.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Load() error = %v, want error containing %q", err, tt.wantError)
			}
		})
	}
}

func TestLoadAcceptsBoundaryValues(t *testing.T) {
	for _, port := range []string{"1", "65535"} {
		t.Run(port, func(t *testing.T) {
			setTestEnv(t)
			t.Setenv("PORT", port)
			t.Setenv("DB_MAX_IDLE_CONNS", "0")
			t.Setenv("DB_MAX_OPEN_CONNS", "1")
			if _, err := Load(); err != nil {
				t.Fatalf("valid boundary configuration rejected: %v", err)
			}
		})
	}
	t.Run("idle equals open", func(t *testing.T) {
		setTestEnv(t)
		t.Setenv("DB_MAX_IDLE_CONNS", "20")
		if _, err := Load(); err != nil {
			t.Fatalf("equal idle and open limits rejected: %v", err)
		}
	})
}
