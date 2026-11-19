package vehicles

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// Service validates and coordinates vehicle location updates, active fleet queries, and route shapes.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService constructs a vehicle tracking service backed by repo.
func NewService(repo Repository) *Service {
	return NewServiceWithClock(repo, time.Now)
}

// NewServiceWithClock constructs a vehicle tracking service with a custom clock for testing.
func NewServiceWithClock(repo Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, now: now}
}

// SubmitLocation validates an incoming vehicle position report and persists the vehicle's latest location.
func (s *Service) SubmitLocation(ctx context.Context, input LocationUpdateInput, defaultSource string) (LocationUpdateResult, error) {
	if s == nil || s.repo == nil {
		return LocationUpdateResult{}, fmt.Errorf("vehicle repository is not configured")
	}

	var vehicleID *int
	if input.VehicleID != nil {
		if *input.VehicleID <= 0 {
			return LocationUpdateResult{}, &InvalidInput{Message: "vehicle_id must be a positive 32-bit integer"}
		}
		v := *input.VehicleID
		vehicleID = &v
	}

	vehicleNumber := strings.TrimSpace(input.VehicleNumber)
	if strings.ContainsRune(vehicleNumber, 0) {
		return LocationUpdateResult{}, &InvalidInput{Message: "vehicle_number contains invalid NUL bytes"}
	}
	if utf8.RuneCountInString(vehicleNumber) > MaxVehicleNumberRunes {
		return LocationUpdateResult{}, &InvalidInput{Message: "vehicle_number must be at most 20 characters"}
	}
	if vehicleID == nil && vehicleNumber == "" {
		return LocationUpdateResult{}, &InvalidInput{Message: "vehicle_id or vehicle_number is required"}
	}

	if input.RouteID <= 0 {
		return LocationUpdateResult{}, &InvalidInput{Message: "route_id must be a positive 32-bit integer"}
	}

	var tripID *int
	if input.TripID != nil {
		if *input.TripID <= 0 {
			return LocationUpdateResult{}, &InvalidInput{Message: "trip_id must be a positive 32-bit integer"}
		}
		id := *input.TripID
		tripID = &id
	}

	if input.Latitude == nil || !isFiniteInRange(*input.Latitude, -90, 90) {
		return LocationUpdateResult{}, &InvalidInput{Message: "latitude is required and must be between -90 and 90"}
	}
	if input.Longitude == nil || !isFiniteInRange(*input.Longitude, -180, 180) {
		return LocationUpdateResult{}, &InvalidInput{Message: "longitude is required and must be between -180 and 180"}
	}

	var heading *float64
	if input.Heading != nil {
		if !isFiniteInRange(*input.Heading, 0, 360) {
			return LocationUpdateResult{}, &InvalidInput{Message: "heading must be between 0 and 360 degrees"}
		}
		h := *input.Heading
		heading = &h
	}

	var speedKmh *float64
	if input.SpeedKmh != nil {
		if !isFiniteInRange(*input.SpeedKmh, 0, 200) {
			return LocationUpdateResult{}, &InvalidInput{Message: "speed_kmh must be between 0 and 200"}
		}
		sp := *input.SpeedKmh
		speedKmh = &sp
	}

	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = StatusInService
	}
	switch status {
	case StatusInService, StatusDelayed, StatusLayover, StatusOutOfService:
	default:
		return LocationUpdateResult{}, &InvalidInput{Message: "status must be one of in_service, delayed, layover, or out_of_service"}
	}

	nowUTC := s.now().UTC()
	recordedAt := nowUTC
	if input.RecordedAt != nil {
		if input.RecordedAt.IsZero() {
			return LocationUpdateResult{}, &InvalidInput{Message: "recorded_at must be a valid RFC3339 timestamp"}
		}
		recordedAt = input.RecordedAt.UTC()
		if recordedAt.After(nowUTC.Add(MaxFutureSkew)) {
			return LocationUpdateResult{}, &InvalidInput{Message: "recorded_at cannot be far in the future"}
		}
	}

	sourceService := strings.TrimSpace(input.SourceService)
	if strings.ContainsRune(sourceService, 0) {
		return LocationUpdateResult{}, &InvalidInput{Message: "source_service contains invalid NUL bytes"}
	}
	if utf8.RuneCountInString(sourceService) > MaxSourceServiceRunes {
		return LocationUpdateResult{}, &InvalidInput{Message: "source_service must be at most 100 characters"}
	}
	if sourceService == "" {
		switch {
		case input.IsSimulated:
			sourceService = "simulator"
		case strings.TrimSpace(defaultSource) != "":
			sourceService = strings.TrimSpace(defaultSource)
		default:
			sourceService = "transit"
		}
		if utf8.RuneCountInString(sourceService) > MaxSourceServiceRunes {
			sourceService = string([]rune(sourceService)[:MaxSourceServiceRunes])
		}
	}

	isAccessible := false
	if input.IsAccessible != nil {
		isAccessible = *input.IsAccessible
	}

	return s.repo.UpsertLocation(ctx, LocationUpdateParams{
		VehicleID:     vehicleID,
		VehicleNumber: vehicleNumber,
		IsAccessible:  isAccessible,
		RouteID:       input.RouteID,
		TripID:        tripID,
		Latitude:      *input.Latitude,
		Longitude:     *input.Longitude,
		Heading:       heading,
		SpeedKmh:      speedKmh,
		Status:        status,
		SourceService: sourceService,
		IsSimulated:   input.IsSimulated,
		RecordedAt:    recordedAt,
		ReceivedAt:    nowUTC,
	})
}

// ListActiveVehicles returns recently reported active vehicles matching optional route or trip filters.
func (s *Service) ListActiveVehicles(ctx context.Context, query ActiveVehiclesQuery) ([]ActiveVehicle, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("vehicle repository is not configured")
	}
	if query.RouteID < 0 {
		return nil, &InvalidInput{Message: "route_id must be a positive 32-bit integer"}
	}
	if query.TripID < 0 {
		return nil, &InvalidInput{Message: "trip_id must be a positive 32-bit integer"}
	}

	maxAge := query.MaxAge
	if maxAge == 0 {
		maxAge = DefaultActiveMaxAge
	}
	if maxAge < MinActiveMaxAge || maxAge > MaxActiveMaxAge {
		return nil, &InvalidInput{Message: "max_age_seconds must be between 10 and 86400"}
	}

	routeNumber := strings.TrimSpace(query.RouteNumber)
	if strings.ContainsRune(routeNumber, 0) {
		return nil, &InvalidInput{Message: "route_number contains invalid NUL bytes"}
	}

	if query.RouteID > 0 {
		if err := s.repo.RouteExists(ctx, query.RouteID); err != nil {
			return nil, err
		}
	}
	if query.TripID > 0 {
		if err := s.repo.TripExists(ctx, query.TripID); err != nil {
			return nil, err
		}
	}

	cutoff := s.now().UTC().Add(-maxAge)
	list, err := s.repo.ListActiveVehicles(ctx, cutoff, query.RouteID, routeNumber, query.TripID)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []ActiveVehicle{}
	}
	return list, nil
}

// RouteGeometry returns the polyline coordinates used to display the requested route on a map.
func (s *Service) RouteGeometry(ctx context.Context, routeID int) (RouteGeometry, error) {
	if s == nil || s.repo == nil {
		return RouteGeometry{}, fmt.Errorf("vehicle repository is not configured")
	}
	if routeID <= 0 {
		return RouteGeometry{}, &InvalidInput{Message: "route_id must be a positive 32-bit integer"}
	}
	geom, err := s.repo.RouteGeometry(ctx, routeID)
	if err != nil {
		return RouteGeometry{}, err
	}
	if geom.Coordinates == nil {
		geom.Coordinates = []Coordinate{}
	}
	return geom, nil
}

// ActiveRoutesWithGeometries loads active routes with shapes for live simulation setup.
func (s *Service) ActiveRoutesWithGeometries(ctx context.Context, limit int) ([]SimulatedRoute, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("vehicle repository is not configured")
	}
	return s.repo.ActiveRoutesWithGeometries(ctx, limit)
}

// ClearSimulated deletes simulated vehicle location records while preserving real submissions.
func (s *Service) ClearSimulated(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("vehicle repository is not configured")
	}
	return s.repo.ClearSimulatedLocations(ctx)
}

func isFiniteInRange(value, minVal, maxVal float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minVal && value <= maxVal
}
