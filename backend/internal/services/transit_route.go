// Package services validates and manages transit route data.
package services

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/models"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrTransitRouteNotFound = errors.New("route not found")
	ErrTransitRouteConflict = errors.New("a route with that number and direction already exists")
)

// ValidationError reports invalid route input to the application layer.
type ValidationError struct{ Message string }

// Error returns the validation error message.
func (e *ValidationError) Error() string { return e.Message }

// CreateTransitRouteInput contains the fields callers may set. Imported source metadata
// and database-generated identifiers are deliberately managed separately.
type CreateTransitRouteInput struct {
	Number    string `json:"number"`
	Name      string `json:"name"`
	Direction string `json:"direction"`
}

// Pointers distinguish an omitted field from an explicit false or empty string.
type UpdateTransitRouteInput struct {
	Number    *string `json:"number"`
	Name      *string `json:"name"`
	Direction *string `json:"direction"`
	IsActive  *bool   `json:"is_active"`
}

// TransitRouteService manages transit route records and validates changes.
type TransitRouteService struct{ db *gorm.DB }

// NewTransitRouteService creates a route service backed by the given database.
func NewTransitRouteService(db *gorm.DB) *TransitRouteService { return &TransitRouteService{db: db} }

// List returns the transit routes available to clients.
func (s *TransitRouteService) List(ctx context.Context) ([]models.Route, error) {
	routes := make([]models.Route, 0)
	err := s.db.WithContext(ctx).Order("id").Find(&routes).Error
	return routes, err
}

// Get returns one transit route by identifier.
func (s *TransitRouteService) Get(ctx context.Context, id int) (models.Route, error) {
	var route models.Route
	err := s.db.WithContext(ctx).First(&route, id).Error
	return route, transitRouteError(err)
}

// Create validates and creates a transit route.
func (s *TransitRouteService) Create(ctx context.Context, input CreateTransitRouteInput) (models.Route, error) {
	route := models.Route{
		Number: strings.TrimSpace(input.Number), Name: strings.TrimSpace(input.Name),
		Direction: strings.TrimSpace(input.Direction), IsActive: true,
	}
	if err := validateTransitRouteText("number", route.Number, 20); err != nil {
		return models.Route{}, err
	}
	if err := validateTransitRouteText("name", route.Name, 100); err != nil {
		return models.Route{}, err
	}
	if err := validateDirection(route.Direction); err != nil {
		return models.Route{}, err
	}
	err := s.db.WithContext(ctx).Create(&route).Error
	return route, transitRouteError(err)
}

// Update validates and updates a transit route.
func (s *TransitRouteService) Update(ctx context.Context, id int, input UpdateTransitRouteInput) (models.Route, error) {
	updates, err := transitRouteUpdates(input)
	if err != nil {
		return models.Route{}, err
	}
	return s.update(ctx, id, updates)
}

// Deactivate preserves schedules and other records that reference this route.
func (s *TransitRouteService) Deactivate(ctx context.Context, id int) error {
	_, err := s.update(ctx, id, map[string]any{"is_active": false})
	return err
}

func (s *TransitRouteService) update(ctx context.Context, id int, updates map[string]any) (models.Route, error) {
	var route models.Route
	// RETURNING gets the updated row in the same statement, avoiding a read/write race.
	result := s.db.WithContext(ctx).Model(&route).Clauses(clause.Returning{}).
		Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return models.Route{}, transitRouteError(result.Error)
	}
	if result.RowsAffected == 0 {
		return models.Route{}, ErrTransitRouteNotFound
	}
	return route, nil
}

func transitRouteUpdates(input UpdateTransitRouteInput) (map[string]any, error) {
	updates := make(map[string]any)
	for _, field := range []struct {
		name  string
		value *string
		max   int
	}{{"number", input.Number, 20}, {"name", input.Name, 100}} {
		if field.value != nil {
			value := strings.TrimSpace(*field.value)
			if err := validateTransitRouteText(field.name, value, field.max); err != nil {
				return nil, err
			}
			updates[field.name] = value
		}
	}
	if input.Direction != nil {
		value := strings.TrimSpace(*input.Direction)
		if err := validateDirection(value); err != nil {
			return nil, err
		}
		updates["direction"] = value
	}
	if input.IsActive != nil {
		updates["is_active"] = *input.IsActive
	}
	if len(updates) == 0 {
		return nil, &ValidationError{"provide at least one field to update"}
	}
	return updates, nil
}

func validateTransitRouteText(field, value string, max int) error {
	if value == "" || utf8.RuneCountInString(value) > max || strings.ContainsRune(value, 0) {
		return &ValidationError{field + " is required and must fit its database length limit"}
	}
	return nil
}

func validateDirection(value string) error {
	switch value {
	case "direction_0", "direction_1":
		return nil
	default:
		return &ValidationError{"direction must be direction_0 or direction_1"}
	}
}

func transitRouteError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrTransitRouteNotFound
	}
	var pgError *pgconn.PgError
	if errors.Is(err, gorm.ErrDuplicatedKey) || (errors.As(err, &pgError) && pgError.Code == "23505") {
		return ErrTransitRouteConflict
	}
	return err
}
