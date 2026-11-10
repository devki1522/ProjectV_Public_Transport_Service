package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func TestRouteUpdatesPreserveFalseAndOmittedFields(t *testing.T) {
	active := false
	name := "  New name  "
	updates, err := transitRouteUpdates(UpdateTransitRouteInput{Name: &name, IsActive: &active})
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 2 || updates["name"] != "New name" || updates["is_active"] != false {
		t.Fatalf("unexpected updates: %#v", updates)
	}
	if _, exists := updates["number"]; exists {
		t.Fatal("omitted number must not change")
	}
}

func TestRouteValidation(t *testing.T) {
	empty := " "
	longNumber := strings.Repeat("7", 21)
	badDirection := "up"
	for _, input := range []UpdateTransitRouteInput{
		{}, {Name: &empty}, {Number: &longNumber}, {Direction: &badDirection},
	} {
		_, err := transitRouteUpdates(input)
		var validation *ValidationError
		if !errors.As(err, &validation) {
			t.Errorf("expected validation error for %#v, got %v", input, err)
		}
	}
	// PostgreSQL varchar limits characters rather than UTF-8 bytes.
	if err := validateTransitRouteText("name", strings.Repeat("é", 100), 100); err != nil {
		t.Fatal(err)
	}
	for _, direction := range []string{"direction_0", "direction_1"} {
		if err := validateDirection(direction); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreateRejectsInvalidInputBeforeDatabaseAccess(t *testing.T) {
	service := NewTransitRouteService(nil)
	for _, input := range []CreateTransitRouteInput{
		{}, {Number: "7", Direction: "direction_0"}, {Number: "7", Name: "Main", Direction: "up"},
	} {
		_, err := service.Create(context.Background(), input)
		var validation *ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("expected validation error, got %v", err)
		}
	}
}

func TestCompassDirectionsAreRejected(t *testing.T) {
	service := NewTransitRouteService(nil)
	for _, direction := range []string{"northbound", "southbound", "eastbound", "westbound", "0", "1"} {
		t.Run(direction, func(t *testing.T) {
			// A nil database ensures both paths reject the value before querying.
			_, err := service.Create(context.Background(), CreateTransitRouteInput{
				Number: "7", Name: "Main", Direction: direction,
			})
			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("create accepted invalid direction %q: %v", direction, err)
			}
			_, err = service.Update(context.Background(), 1, UpdateTransitRouteInput{Direction: &direction})
			if !errors.As(err, &validation) {
				t.Fatalf("update accepted invalid direction %q: %v", direction, err)
			}
		})
	}
}

func TestRouteDatabaseErrors(t *testing.T) {
	if !errors.Is(transitRouteError(gorm.ErrRecordNotFound), ErrTransitRouteNotFound) {
		t.Fatal("missing route not mapped")
	}
	if !errors.Is(transitRouteError(&pgconn.PgError{Code: "23505"}), ErrTransitRouteConflict) {
		t.Fatal("duplicate route not mapped")
	}
	err := errors.New("connection failed")
	if transitRouteError(err) != err {
		t.Fatal("unexpected error must be preserved")
	}
}
