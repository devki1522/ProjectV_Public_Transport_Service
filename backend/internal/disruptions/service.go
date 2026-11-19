package disruptions

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	earthRadiusMeters = 6371000.0
	metersPerLatDeg   = 111320.0
	maxInt32          = 2147483647
)

var allowedSeverities = map[string]bool{
	"low":      true,
	"medium":   true,
	"moderate": true,
	"high":     true,
	"critical": true,
}

// Service validates disruption operations, evaluates affected transit services, and computes operational statuses.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService constructs a disruption service using the real system clock.
func NewService(repo Repository) *Service {
	return NewServiceWithClock(repo, nil)
}

// NewServiceWithClock constructs a disruption service with an injectable clock for deterministic tests.
func NewServiceWithClock(repo Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, now: now}
}

func (s *Service) ensureRepo() error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("disruption repository unavailable")
	}
	return nil
}

// List returns disruptions matching the optional filter criteria.
func (s *Service) List(ctx context.Context, filter ListFilter) ([]DisruptionResponse, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	normalized, err := normalizeListFilter(filter)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.List(ctx, normalized)
	if err != nil {
		return nil, err
	}
	return normalizeDisruptionSlice(items), nil
}

// ListActive returns only active disruptions (`status='active'`) matching the optional filter criteria (US 194 / PT-FR-017).
func (s *Service) ListActive(ctx context.Context, filter ListFilter) ([]DisruptionResponse, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	if filter.Status != "" && strings.ToLower(strings.TrimSpace(filter.Status)) != StatusActive {
		return nil, &InvalidInput{Message: "active endpoint only supports status=active"}
	}
	filter.Status = StatusActive
	normalized, err := normalizeListFilter(filter)
	if err != nil {
		return nil, err
	}
	normalized.Status = StatusActive
	items, err := s.repo.List(ctx, normalized)
	if err != nil {
		return nil, err
	}
	// Defensive filter ensuring only active disruptions are ever returned.
	activeOnly := make([]DisruptionResponse, 0, len(items))
	for _, item := range items {
		if item.Status == StatusActive {
			activeOnly = append(activeOnly, normalizeDisruptionResponse(item))
		}
	}
	return activeOnly, nil
}

// Get loads a single disruption by identifier.
func (s *Service) Get(ctx context.Context, id int) (DisruptionResponse, error) {
	if err := s.ensureRepo(); err != nil {
		return DisruptionResponse{}, err
	}
	if err := validatePositiveID("id", id); err != nil {
		return DisruptionResponse{}, err
	}
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return DisruptionResponse{}, err
	}
	return normalizeDisruptionResponse(item), nil
}

// Create validates and creates a new transit disruption and its affected service links (US 190, 191 / PT-FR-014, PT-FR-015).
func (s *Service) Create(ctx context.Context, input CreateDisruptionInput, defaultSource string) (DisruptionResponse, error) {
	if err := s.ensureRepo(); err != nil {
		return DisruptionResponse{}, err
	}
	now := s.now().UTC()

	disruptionType := strings.ToLower(strings.TrimSpace(input.Type))
	if !IsValidType(disruptionType) {
		return DisruptionResponse{}, &InvalidInput{Message: "type must be one of delay, detour, cancellation, stop_closure, road_closure, construction, emergency, traffic_restriction, or accessibility"}
	}

	title, err := validateRequiredString("title", input.Title, MaxTitleRunes)
	if err != nil {
		return DisruptionResponse{}, err
	}
	description, err := validateOptionalString("description", input.Description, MaxDescriptionRunes)
	if err != nil {
		return DisruptionResponse{}, err
	}
	affectedArea, err := validateOptionalString("affected_area", input.AffectedArea, MaxAffectedAreaRunes)
	if err != nil {
		return DisruptionResponse{}, err
	}
	externalRef, err := validateOptionalString("external_reference", input.ExternalReference, MaxExternalRefRunes)
	if err != nil {
		return DisruptionResponse{}, err
	}

	source := strings.TrimSpace(input.SourceService)
	if source == "" {
		source = strings.TrimSpace(defaultSource)
	}
	if source == "" {
		source = SourceTransitAdmin
	}
	source, err = validateRequiredString("source_service", source, MaxSourceServiceRunes)
	if err != nil {
		return DisruptionResponse{}, err
	}

	startTime := now
	if input.StartTime != nil {
		if input.StartTime.IsZero() {
			return DisruptionResponse{}, &InvalidInput{Message: "start_time is invalid"}
		}
		startTime = input.StartTime.UTC()
	}

	var endTime *time.Time
	if input.EndTime != nil {
		if input.EndTime.IsZero() {
			return DisruptionResponse{}, &InvalidInput{Message: "end_time is invalid"}
		}
		utcEnd := input.EndTime.UTC()
		if utcEnd.Before(startTime) {
			return DisruptionResponse{}, &InvalidInput{Message: "end_time must be greater than or equal to start_time"}
		}
		endTime = &utcEnd
	}

	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		if startTime.After(now) {
			status = StatusScheduled
		} else {
			status = StatusActive
		}
	} else if !IsValidStatus(status) {
		return DisruptionResponse{}, &InvalidInput{Message: "status must be one of scheduled, active, or resolved"}
	}

	routeIDs, err := validateIDSlice("route_ids", input.RouteIDs)
	if err != nil {
		return DisruptionResponse{}, err
	}
	stopIDs, err := validateIDSlice("stop_ids", input.StopIDs)
	if err != nil {
		return DisruptionResponse{}, err
	}
	tripIDs, err := validateIDSlice("trip_ids", input.TripIDs)
	if err != nil {
		return DisruptionResponse{}, err
	}
	vehicleIDs, err := validateIDSlice("vehicle_ids", input.VehicleIDs)
	if err != nil {
		return DisruptionResponse{}, err
	}
	segments, err := validateSegments(input.AffectedSegments)
	if err != nil {
		return DisruptionResponse{}, err
	}

	created, err := s.repo.Create(ctx, SaveDisruptionParams{
		Type:              disruptionType,
		Title:             title,
		Description:       description,
		AffectedArea:      affectedArea,
		StartTime:         startTime,
		EndTime:           endTime,
		Status:            status,
		SourceService:     source,
		ExternalReference: externalRef,
		ReceivedAt:        now,
		RouteIDs:          routeIDs,
		StopIDs:           stopIDs,
		TripIDs:           tripIDs,
		VehicleIDs:        vehicleIDs,
		AffectedSegments:  segments,
	})
	if err != nil {
		return DisruptionResponse{}, err
	}
	return normalizeDisruptionResponse(created), nil
}

// Update validates and applies partial updates to an existing disruption and its affected service links (US 190, 191, 192).
func (s *Service) Update(ctx context.Context, id int, input UpdateDisruptionInput) (DisruptionResponse, error) {
	if err := s.ensureRepo(); err != nil {
		return DisruptionResponse{}, err
	}
	if err := validatePositiveID("id", id); err != nil {
		return DisruptionResponse{}, err
	}

	if input.Type == nil &&
		input.Title == nil &&
		input.Description == nil && !input.DescriptionSet &&
		input.AffectedArea == nil && !input.AffectedAreaSet &&
		input.Status == nil &&
		input.StartTime == nil &&
		input.EndTime == nil && !input.EndTimeSet &&
		input.RouteIDs == nil &&
		input.StopIDs == nil &&
		input.TripIDs == nil &&
		input.VehicleIDs == nil &&
		input.AffectedSegments == nil {
		return DisruptionResponse{}, &InvalidInput{Message: "at least one field must be provided for update"}
	}

	var params PatchDisruptionParams

	if input.Type != nil {
		t := strings.ToLower(strings.TrimSpace(*input.Type))
		if !IsValidType(t) {
			return DisruptionResponse{}, &InvalidInput{Message: "type must be one of delay, detour, cancellation, stop_closure, road_closure, construction, emergency, traffic_restriction, or accessibility"}
		}
		params.Type = &t
	}
	if input.Title != nil {
		title, err := validateRequiredString("title", *input.Title, MaxTitleRunes)
		if err != nil {
			return DisruptionResponse{}, err
		}
		params.Title = &title
	}
	if input.Description != nil || input.DescriptionSet {
		desc, err := validateOptionalString("description", input.Description, MaxDescriptionRunes)
		if err != nil {
			return DisruptionResponse{}, err
		}
		params.Description = desc
		params.DescriptionSet = true
	}
	if input.AffectedArea != nil || input.AffectedAreaSet {
		area, err := validateOptionalString("affected_area", input.AffectedArea, MaxAffectedAreaRunes)
		if err != nil {
			return DisruptionResponse{}, err
		}
		params.AffectedArea = area
		params.AffectedAreaSet = true
	}
	if input.Status != nil {
		st := strings.ToLower(strings.TrimSpace(*input.Status))
		if !IsValidStatus(st) {
			return DisruptionResponse{}, &InvalidInput{Message: "status must be one of scheduled, active, or resolved"}
		}
		params.Status = &st
	}
	if input.StartTime != nil {
		if input.StartTime.IsZero() {
			return DisruptionResponse{}, &InvalidInput{Message: "start_time is invalid"}
		}
		utcStart := input.StartTime.UTC()
		params.StartTime = &utcStart
	}
	if input.EndTime != nil {
		if input.EndTime.IsZero() {
			return DisruptionResponse{}, &InvalidInput{Message: "end_time is invalid"}
		}
		utcEnd := input.EndTime.UTC()
		params.EndTime = &utcEnd
		params.EndTimeSet = true
	} else if input.EndTimeSet {
		params.EndTime = nil
		params.EndTimeSet = true
	}
	if params.StartTime != nil && params.EndTime != nil && params.EndTime.Before(*params.StartTime) {
		return DisruptionResponse{}, &InvalidInput{Message: "end_time must be greater than or equal to start_time"}
	}

	if input.RouteIDs != nil {
		ids, err := validateIDSlice("route_ids", *input.RouteIDs)
		if err != nil {
			return DisruptionResponse{}, err
		}
		params.RouteIDs = &ids
	}
	if input.StopIDs != nil {
		ids, err := validateIDSlice("stop_ids", *input.StopIDs)
		if err != nil {
			return DisruptionResponse{}, err
		}
		params.StopIDs = &ids
	}
	if input.TripIDs != nil {
		ids, err := validateIDSlice("trip_ids", *input.TripIDs)
		if err != nil {
			return DisruptionResponse{}, err
		}
		params.TripIDs = &ids
	}
	if input.VehicleIDs != nil {
		ids, err := validateIDSlice("vehicle_ids", *input.VehicleIDs)
		if err != nil {
			return DisruptionResponse{}, err
		}
		params.VehicleIDs = &ids
	}
	if input.AffectedSegments != nil {
		segs, err := validateSegments(*input.AffectedSegments)
		if err != nil {
			return DisruptionResponse{}, err
		}
		params.AffectedSegments = &segs
	}

	updated, err := s.repo.Update(ctx, id, params)
	if err != nil {
		return DisruptionResponse{}, err
	}
	return normalizeDisruptionResponse(updated), nil
}

// IngestUtilitiesNotice processes a road closure or construction notice from the Utilities Service (US 186 / PT-FR-018).
func (s *Service) IngestUtilitiesNotice(ctx context.Context, input UtilitiesNoticeInput) (ExternalNoticeResult, error) {
	disruptionType, err := resolveExternalType("notice_type", input.NoticeType, input.Type, TypeRoadClosure, map[string]bool{
		TypeRoadClosure:  true,
		TypeConstruction: true,
	})
	if err != nil {
		return ExternalNoticeResult{}, err
	}
	extRef, err := resolveExternalReference(input.ExternalID, input.ExternalReference)
	if err != nil {
		return ExternalNoticeResult{}, err
	}
	return s.ingestExternalNotice(ctx, externalIngestionRequest{
		SourceService:     SourceUtilities,
		Type:              disruptionType,
		ExternalReference: extRef,
		Title:             input.Title,
		Description:       input.Description,
		AffectedArea:      input.AffectedArea,
		StartTime:         input.StartTime,
		EndTime:           input.EndTime,
		Status:            input.Status,
		RouteIDs:          input.RouteIDs,
		StopIDs:           input.StopIDs,
		TripIDs:           input.TripIDs,
		VehicleIDs:        input.VehicleIDs,
		AffectedSegments:  input.AffectedSegments,
		Latitude:          input.Latitude,
		Longitude:         input.Longitude,
		RadiusMeters:      input.RadiusMeters,
	})
}

// IngestEmergencyIncident processes an emergency incident or road restriction from Emergency Services (US 187 / PT-FR-019).
func (s *Service) IngestEmergencyIncident(ctx context.Context, input EmergencyIncidentInput) (ExternalNoticeResult, error) {
	disruptionType, err := resolveExternalType("incident_type", input.IncidentType, input.Type, TypeEmergency, map[string]bool{
		TypeEmergency:   true,
		TypeRoadClosure: true,
	})
	if err != nil {
		return ExternalNoticeResult{}, err
	}
	extRef, err := resolveExternalReference(input.ExternalID, input.ExternalReference)
	if err != nil {
		return ExternalNoticeResult{}, err
	}
	severity := strings.ToLower(strings.TrimSpace(input.Severity))
	if severity != "" {
		if strings.ContainsRune(severity, '\x00') || utf8.RuneCountInString(severity) > MaxSeverityRunes || !allowedSeverities[severity] {
			return ExternalNoticeResult{}, &InvalidInput{Message: "severity must be one of low, medium, moderate, high, or critical"}
		}
	}
	return s.ingestExternalNotice(ctx, externalIngestionRequest{
		SourceService:     SourceEmergency,
		Type:              disruptionType,
		Severity:          severity,
		ExternalReference: extRef,
		Title:             input.Title,
		Description:       input.Description,
		AffectedArea:      input.AffectedArea,
		StartTime:         input.StartTime,
		EndTime:           input.EndTime,
		Status:            input.Status,
		RouteIDs:          input.RouteIDs,
		StopIDs:           input.StopIDs,
		TripIDs:           input.TripIDs,
		VehicleIDs:        input.VehicleIDs,
		AffectedSegments:  input.AffectedSegments,
		Latitude:          input.Latitude,
		Longitude:         input.Longitude,
		RadiusMeters:      input.RadiusMeters,
	})
}

// IngestSecurityTrafficAlert processes a traffic restriction or road closure alert from Security / Police Services (US 188 / PT-FR-020).
func (s *Service) IngestSecurityTrafficAlert(ctx context.Context, input SecurityTrafficAlertInput) (ExternalNoticeResult, error) {
	disruptionType, err := resolveExternalType("alert_type", input.AlertType, input.Type, TypeTrafficRestriction, map[string]bool{
		TypeTrafficRestriction: true,
		TypeRoadClosure:        true,
	})
	if err != nil {
		return ExternalNoticeResult{}, err
	}
	extRef, err := resolveExternalReference(input.ExternalID, input.ExternalReference)
	if err != nil {
		return ExternalNoticeResult{}, err
	}
	return s.ingestExternalNotice(ctx, externalIngestionRequest{
		SourceService:     SourceSecurity,
		Type:              disruptionType,
		ExternalReference: extRef,
		Title:             input.Title,
		Description:       input.Description,
		AffectedArea:      input.AffectedArea,
		StartTime:         input.StartTime,
		EndTime:           input.EndTime,
		Status:            input.Status,
		RouteIDs:          input.RouteIDs,
		StopIDs:           input.StopIDs,
		TripIDs:           input.TripIDs,
		VehicleIDs:        input.VehicleIDs,
		AffectedSegments:  input.AffectedSegments,
		Latitude:          input.Latitude,
		Longitude:         input.Longitude,
		RadiusMeters:      input.RadiusMeters,
	})
}

type externalIngestionRequest struct {
	SourceService     string
	Type              string
	Severity          string
	ExternalReference *string
	Title             string
	Description       *string
	AffectedArea      *string
	StartTime         *time.Time
	EndTime           *time.Time
	Status            string
	RouteIDs          []int
	StopIDs           []int
	TripIDs           []int
	VehicleIDs        []int
	AffectedSegments  []string
	Latitude          *float64
	Longitude         *float64
	RadiusMeters      *float64
}

func (s *Service) ingestExternalNotice(ctx context.Context, req externalIngestionRequest) (ExternalNoticeResult, error) {
	if err := s.ensureRepo(); err != nil {
		return ExternalNoticeResult{}, err
	}
	now := s.now().UTC()

	title, err := validateRequiredString("title", req.Title, MaxTitleRunes)
	if err != nil {
		return ExternalNoticeResult{}, err
	}
	description, err := validateOptionalString("description", req.Description, MaxDescriptionRunes)
	if err != nil {
		return ExternalNoticeResult{}, err
	}
	affectedArea, err := validateOptionalString("affected_area", req.AffectedArea, MaxAffectedAreaRunes)
	if err != nil {
		return ExternalNoticeResult{}, err
	}

	startTime := now
	if req.StartTime != nil {
		if req.StartTime.IsZero() {
			return ExternalNoticeResult{}, &InvalidInput{Message: "start_time is invalid"}
		}
		startTime = req.StartTime.UTC()
	}

	var endTime *time.Time
	if req.EndTime != nil {
		if req.EndTime.IsZero() {
			return ExternalNoticeResult{}, &InvalidInput{Message: "end_time is invalid"}
		}
		utcEnd := req.EndTime.UTC()
		if utcEnd.Before(startTime) {
			return ExternalNoticeResult{}, &InvalidInput{Message: "end_time must be greater than or equal to start_time"}
		}
		endTime = &utcEnd
	}

	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status == "" {
		if startTime.After(now) {
			status = StatusScheduled
		} else {
			status = StatusActive
		}
	} else if !IsValidStatus(status) {
		return ExternalNoticeResult{}, &InvalidInput{Message: "status must be one of scheduled, active, or resolved"}
	}

	routeIDs, err := validateIDSlice("route_ids", req.RouteIDs)
	if err != nil {
		return ExternalNoticeResult{}, err
	}
	stopIDs, err := validateIDSlice("stop_ids", req.StopIDs)
	if err != nil {
		return ExternalNoticeResult{}, err
	}
	tripIDs, err := validateIDSlice("trip_ids", req.TripIDs)
	if err != nil {
		return ExternalNoticeResult{}, err
	}
	vehicleIDs, err := validateIDSlice("vehicle_ids", req.VehicleIDs)
	if err != nil {
		return ExternalNoticeResult{}, err
	}
	segments, err := validateSegments(req.AffectedSegments)
	if err != nil {
		return ExternalNoticeResult{}, err
	}

	radius := DefaultRadiusMeters
	if req.RadiusMeters != nil {
		if !isFiniteInRange(*req.RadiusMeters, MinRadiusMeters, MaxRadiusMeters) {
			return ExternalNoticeResult{}, &InvalidInput{Message: "radius_meters must be a finite number between 50 and 5000"}
		}
		radius = *req.RadiusMeters
	}

	if (req.Latitude != nil) != (req.Longitude != nil) {
		return ExternalNoticeResult{}, &InvalidInput{Message: "latitude and longitude must be provided together"}
	}
	if req.Latitude != nil && req.Longitude != nil {
		lat := *req.Latitude
		lng := *req.Longitude
		if !isFiniteInRange(lat, -90, 90) {
			return ExternalNoticeResult{}, &InvalidInput{Message: "latitude must be between -90 and 90"}
		}
		if !isFiniteInRange(lng, -180, 180) {
			return ExternalNoticeResult{}, &InvalidInput{Message: "longitude must be between -180 and 180"}
		}
		minLat, maxLat, minLng, maxLng := boundingBox(lat, lng, radius)
		candidates, err := s.repo.FindStopsInBounds(ctx, minLat, maxLat, minLng, maxLng)
		if err != nil {
			return ExternalNoticeResult{}, err
		}
		for _, candidate := range candidates {
			if haversineMeters(lat, lng, candidate.Latitude, candidate.Longitude) <= radius {
				stopIDs = append(stopIDs, candidate.ID)
			}
		}
		stopIDs = uniqueSortedPositive(stopIDs)
	}

	saveParams := SaveDisruptionParams{
		Type:              req.Type,
		Title:             title,
		Description:       description,
		AffectedArea:      affectedArea,
		StartTime:         startTime,
		EndTime:           endTime,
		Status:            status,
		SourceService:     req.SourceService,
		ExternalReference: req.ExternalReference,
		ReceivedAt:        now,
		RouteIDs:          routeIDs,
		StopIDs:           stopIDs,
		TripIDs:           tripIDs,
		VehicleIDs:        vehicleIDs,
		AffectedSegments:  segments,
	}

	var (
		disruption DisruptionResponse
		created    bool
	)
	if req.ExternalReference != nil {
		disruption, created, err = s.repo.UpsertByExternalRef(ctx, saveParams)
	} else {
		disruption, err = s.repo.Create(ctx, saveParams)
		created = true
	}
	if err != nil {
		return ExternalNoticeResult{}, err
	}
	disruption = normalizeDisruptionResponse(disruption)

	matchedRouteIDs := make([]int, 0, len(disruption.AffectedRoutes))
	for _, r := range disruption.AffectedRoutes {
		matchedRouteIDs = append(matchedRouteIDs, r.RouteID)
	}
	matchedRouteIDs = uniqueSortedPositive(matchedRouteIDs)

	matchedStopIDs := make([]int, 0, len(disruption.AffectedStops))
	for _, st := range disruption.AffectedStops {
		matchedStopIDs = append(matchedStopIDs, st.StopID)
	}
	matchedStopIDs = uniqueSortedPositive(matchedStopIDs)

	impact := evaluateTransitImpact(
		disruption.Type,
		disruption.Status,
		req.Severity,
		len(matchedRouteIDs),
		len(matchedStopIDs),
		len(disruption.AffectedTrips),
		len(disruption.AffectedSegments),
	)

	return ExternalNoticeResult{
		Created:         created,
		Disruption:      disruption,
		MatchedRouteIDs: matchedRouteIDs,
		MatchedStopIDs:  matchedStopIDs,
		TransitImpact:   impact,
	}, nil
}

// AffectedServices computes a consolidated summary of all routes, stops, trips, vehicles, and segments affected by active disruptions (US 191, 195).
func (s *Service) AffectedServices(ctx context.Context) (AffectedServicesSummary, error) {
	status, err := s.ServiceStatus(ctx)
	if err != nil {
		return AffectedServicesSummary{}, err
	}
	return AffectedServicesSummary{
		GeneratedAt:           status.GeneratedAt,
		ActiveDisruptionCount: status.ActiveDisruptionCount,
		AffectedRoutes:        status.AffectedRoutes,
		AffectedStops:         status.AffectedStops,
		AffectedTrips:         status.AffectedTrips,
		AffectedVehicles:      status.AffectedVehicles,
		AffectedSegments:      status.AffectedSegments,
	}, nil
}

// ServiceStatus returns the complete active disruption and service status snapshot for residents and enterprise integrations (US 194, 195, 196).
func (s *Service) ServiceStatus(ctx context.Context) (ServiceStatusResponse, error) {
	if err := s.ensureRepo(); err != nil {
		return ServiceStatusResponse{}, err
	}
	now := s.now().UTC()
	active, err := s.ListActive(ctx, ListFilter{Status: StatusActive})
	if err != nil {
		return ServiceStatusResponse{}, err
	}

	overallStatus := OverallStatusNormal
	if len(active) > 0 {
		overallStatus = OverallStatusDisrupted
	}

	detours := make([]DisruptionResponse, 0)
	disruptionIDs := make([]int, 0, len(active))
	typeByDisruptionID := make(map[int]string, len(active))

	for _, d := range active {
		disruptionIDs = append(disruptionIDs, d.ID)
		typeByDisruptionID[d.ID] = d.Type
		if d.Type == TypeDetour {
			detours = append(detours, d)
		}
	}

	var (
		tripDetails    []AffectedTripDetail
		vehicleDetails []AffectedVehicleDetail
	)
	if len(disruptionIDs) > 0 {
		tripDetails, err = s.repo.ListActiveImpactedTrips(ctx, disruptionIDs)
		if err != nil {
			return ServiceStatusResponse{}, err
		}
		vehicleDetails, err = s.repo.ListActiveImpactedVehicles(ctx, disruptionIDs)
		if err != nil {
			return ServiceStatusResponse{}, err
		}
	}

	type routeAcc struct {
		route         AffectedRoute
		disruptionIDs []int
		types         []string
	}
	type stopAcc struct {
		stopID        int
		stopName      string
		disruptionIDs []int
		types         []string
	}
	type tripAcc struct {
		tripID        int
		routeID       int
		disruptionIDs []int
		types         []string
	}
	type vehAcc struct {
		vehicleID     int
		vehicleNumber string
		disruptionIDs []int
		types         []string
	}

	routesMap := make(map[int]*routeAcc)
	stopsMap := make(map[int]*stopAcc)
	tripsMap := make(map[int]*tripAcc)
	vehiclesMap := make(map[int]*vehAcc)
	segments := make([]AffectedSegmentSummary, 0)

	for _, d := range active {
		for _, r := range d.AffectedRoutes {
			acc, exists := routesMap[r.RouteID]
			if !exists {
				acc = &routeAcc{route: r}
				routesMap[r.RouteID] = acc
			}
			acc.disruptionIDs = append(acc.disruptionIDs, d.ID)
			acc.types = append(acc.types, d.Type)
		}
		for _, st := range d.AffectedStops {
			acc, exists := stopsMap[st.StopID]
			if !exists {
				acc = &stopAcc{stopID: st.StopID, stopName: st.StopName}
				stopsMap[st.StopID] = acc
			}
			acc.disruptionIDs = append(acc.disruptionIDs, d.ID)
			acc.types = append(acc.types, d.Type)
		}

		var primaryRouteID *int
		if len(d.AffectedRoutes) > 0 {
			rid := d.AffectedRoutes[0].RouteID
			primaryRouteID = &rid
		}
		segOpStatus := deriveOperationalStatus("segment", []string{d.Type})
		for _, seg := range d.AffectedSegments {
			var ridCopy *int
			if primaryRouteID != nil {
				v := *primaryRouteID
				ridCopy = &v
			}
			segments = append(segments, AffectedSegmentSummary{
				DisruptionID:      d.ID,
				RouteID:           ridCopy,
				Segment:           seg,
				OperationalStatus: segOpStatus,
			})
		}
	}

	for _, td := range tripDetails {
		dType, ok := typeByDisruptionID[td.DisruptionID]
		if !ok {
			continue
		}
		acc, exists := tripsMap[td.TripID]
		if !exists {
			acc = &tripAcc{tripID: td.TripID, routeID: td.RouteID}
			tripsMap[td.TripID] = acc
		}
		acc.disruptionIDs = append(acc.disruptionIDs, td.DisruptionID)
		acc.types = append(acc.types, dType)
	}

	for _, vd := range vehicleDetails {
		dType, ok := typeByDisruptionID[vd.DisruptionID]
		if !ok {
			continue
		}
		acc, exists := vehiclesMap[vd.VehicleID]
		if !exists {
			acc = &vehAcc{vehicleID: vd.VehicleID, vehicleNumber: vd.VehicleNumber}
			vehiclesMap[vd.VehicleID] = acc
		}
		acc.disruptionIDs = append(acc.disruptionIDs, vd.DisruptionID)
		acc.types = append(acc.types, dType)
	}

	affectedRoutes := make([]AffectedRouteSummary, 0, len(routesMap))
	for _, acc := range routesMap {
		affectedRoutes = append(affectedRoutes, AffectedRouteSummary{
			RouteID:           acc.route.RouteID,
			RouteNumber:       acc.route.RouteNumber,
			RouteName:         acc.route.RouteName,
			Direction:         acc.route.Direction,
			OperationalStatus: deriveOperationalStatus("route", acc.types),
			DisruptionIDs:     uniqueSortedPositive(acc.disruptionIDs),
		})
	}
	sort.Slice(affectedRoutes, func(i, j int) bool {
		if affectedRoutes[i].RouteNumber != affectedRoutes[j].RouteNumber {
			return affectedRoutes[i].RouteNumber < affectedRoutes[j].RouteNumber
		}
		if affectedRoutes[i].Direction != affectedRoutes[j].Direction {
			return affectedRoutes[i].Direction < affectedRoutes[j].Direction
		}
		return affectedRoutes[i].RouteID < affectedRoutes[j].RouteID
	})

	affectedStops := make([]AffectedStopSummary, 0, len(stopsMap))
	for _, acc := range stopsMap {
		affectedStops = append(affectedStops, AffectedStopSummary{
			StopID:            acc.stopID,
			StopName:          acc.stopName,
			OperationalStatus: deriveOperationalStatus("stop", acc.types),
			DisruptionIDs:     uniqueSortedPositive(acc.disruptionIDs),
		})
	}
	sort.Slice(affectedStops, func(i, j int) bool {
		if affectedStops[i].StopName != affectedStops[j].StopName {
			return affectedStops[i].StopName < affectedStops[j].StopName
		}
		return affectedStops[i].StopID < affectedStops[j].StopID
	})

	affectedTrips := make([]AffectedTripSummary, 0, len(tripsMap))
	for _, acc := range tripsMap {
		affectedTrips = append(affectedTrips, AffectedTripSummary{
			TripID:            acc.tripID,
			RouteID:           acc.routeID,
			OperationalStatus: deriveOperationalStatus("trip", acc.types),
			DisruptionIDs:     uniqueSortedPositive(acc.disruptionIDs),
		})
	}
	sort.Slice(affectedTrips, func(i, j int) bool {
		return affectedTrips[i].TripID < affectedTrips[j].TripID
	})

	affectedVehicles := make([]AffectedVehicleSummary, 0, len(vehiclesMap))
	for _, acc := range vehiclesMap {
		affectedVehicles = append(affectedVehicles, AffectedVehicleSummary{
			VehicleID:         acc.vehicleID,
			VehicleNumber:     acc.vehicleNumber,
			OperationalStatus: deriveOperationalStatus("vehicle", acc.types),
			DisruptionIDs:     uniqueSortedPositive(acc.disruptionIDs),
		})
	}
	sort.Slice(affectedVehicles, func(i, j int) bool {
		if affectedVehicles[i].VehicleNumber != affectedVehicles[j].VehicleNumber {
			return affectedVehicles[i].VehicleNumber < affectedVehicles[j].VehicleNumber
		}
		return affectedVehicles[i].VehicleID < affectedVehicles[j].VehicleID
	})

	return ServiceStatusResponse{
		GeneratedAt:           now,
		OverallStatus:         overallStatus,
		ActiveDisruptionCount: len(active),
		ActiveDisruptions:     active,
		Detours:               detours,
		AffectedRoutes:        affectedRoutes,
		AffectedStops:         affectedStops,
		AffectedTrips:         affectedTrips,
		AffectedVehicles:      affectedVehicles,
		AffectedSegments:      segments,
	}, nil
}

// RouteStatus returns the operational status and active disruptions for a route (US 192 / PT-FR-016).
func (s *Service) RouteStatus(ctx context.Context, routeID int) (EntityOperationalStatus, error) {
	if err := s.ensureRepo(); err != nil {
		return EntityOperationalStatus{}, err
	}
	if err := validatePositiveID("id", routeID); err != nil {
		return EntityOperationalStatus{}, err
	}
	if err := s.repo.RouteExists(ctx, routeID); err != nil {
		return EntityOperationalStatus{}, err
	}
	active, err := s.ListActive(ctx, ListFilter{Status: StatusActive, RouteID: routeID})
	if err != nil {
		return EntityOperationalStatus{}, err
	}
	return buildEntityOperationalStatus("route", routeID, active), nil
}

// StopStatus returns the operational status and active disruptions for a stop (US 192 / PT-FR-016).
func (s *Service) StopStatus(ctx context.Context, stopID int) (EntityOperationalStatus, error) {
	if err := s.ensureRepo(); err != nil {
		return EntityOperationalStatus{}, err
	}
	if err := validatePositiveID("id", stopID); err != nil {
		return EntityOperationalStatus{}, err
	}
	if err := s.repo.StopExists(ctx, stopID); err != nil {
		return EntityOperationalStatus{}, err
	}
	active, err := s.ListActive(ctx, ListFilter{Status: StatusActive, StopID: stopID})
	if err != nil {
		return EntityOperationalStatus{}, err
	}
	return buildEntityOperationalStatus("stop", stopID, active), nil
}

// TripStatus returns the operational status and active disruptions for a trip (US 192 / PT-FR-016).
func (s *Service) TripStatus(ctx context.Context, tripID int) (EntityOperationalStatus, error) {
	if err := s.ensureRepo(); err != nil {
		return EntityOperationalStatus{}, err
	}
	if err := validatePositiveID("id", tripID); err != nil {
		return EntityOperationalStatus{}, err
	}
	routeID, err := s.repo.TripRouteID(ctx, tripID)
	if err != nil {
		return EntityOperationalStatus{}, err
	}
	active, err := s.repo.ListActiveForTrip(ctx, tripID, routeID)
	if err != nil {
		return EntityOperationalStatus{}, err
	}
	activeOnly := make([]DisruptionResponse, 0, len(active))
	for _, item := range active {
		if item.Status == StatusActive {
			activeOnly = append(activeOnly, normalizeDisruptionResponse(item))
		}
	}
	return buildEntityOperationalStatus("trip", tripID, activeOnly), nil
}

func buildEntityOperationalStatus(entityType string, entityID int, active []DisruptionResponse) EntityOperationalStatus {
	types := make([]string, 0, len(active))
	for _, d := range active {
		types = append(types, d.Type)
	}
	if active == nil {
		active = []DisruptionResponse{}
	}
	return EntityOperationalStatus{
		EntityType:        entityType,
		EntityID:          entityID,
		OperationalStatus: deriveOperationalStatus(entityType, types),
		ActiveDisruptions: active,
	}
}

func deriveOperationalStatus(entityType string, disruptionTypes []string) string {
	bestStatus := OpStatusNormal
	bestPriority := 0

	for _, t := range disruptionTypes {
		status := OpStatusDisrupted
		priority := 1
		switch t {
		case TypeCancellation:
			status = OpStatusCancelled
			priority = 80
		case TypeStopClosure:
			if entityType == "stop" {
				status = OpStatusClosed
				priority = 75
			} else {
				status = OpStatusDisrupted
				priority = 50
			}
		case TypeEmergency:
			status = OpStatusDisrupted
			priority = 70
		case TypeRoadClosure:
			status = OpStatusDisrupted
			priority = 60
		case TypeDetour:
			status = OpStatusDetoured
			priority = 40
		case TypeConstruction:
			status = OpStatusDisrupted
			priority = 30
		case TypeTrafficRestriction:
			status = OpStatusRestricted
			priority = 25
		case TypeAccessibility:
			status = OpStatusRestricted
			priority = 20
		case TypeDelay:
			status = OpStatusDelayed
			priority = 15
		}
		if priority > bestPriority {
			bestPriority = priority
			bestStatus = status
		}
	}
	return bestStatus
}

func evaluateTransitImpact(disruptionType, status, severity string, routeCount, stopCount, tripCount, segmentCount int) string {
	if status == StatusResolved {
		return "resolved: disruption cleared; affected transit services returning to normal operation"
	}
	if routeCount == 0 && stopCount == 0 && tripCount == 0 && segmentCount == 0 {
		return "none: no active transit routes, stops, or trips matched within affected area"
	}
	tier := "moderate"
	switch {
	case severity == "critical":
		tier = "critical"
	case severity == "high" ||
		disruptionType == TypeEmergency ||
		disruptionType == TypeRoadClosure ||
		disruptionType == TypeCancellation ||
		routeCount >= 3 ||
		stopCount >= 5:
		tier = "high"
	case severity == "low" && routeCount <= 1 && stopCount <= 1:
		tier = "low"
	}

	parts := make([]string, 0, 4)
	if routeCount > 0 {
		parts = append(parts, fmt.Sprintf("%d route(s)", routeCount))
	}
	if stopCount > 0 {
		parts = append(parts, fmt.Sprintf("%d stop(s)", stopCount))
	}
	if tripCount > 0 {
		parts = append(parts, fmt.Sprintf("%d trip(s)", tripCount))
	}
	if segmentCount > 0 {
		parts = append(parts, fmt.Sprintf("%d corridor segment(s)", segmentCount))
	}
	return fmt.Sprintf("%s: %s affected", tier, strings.Join(parts, ", "))
}

func resolveExternalType(fieldName, primary, fallback, defaultType string, allowed map[string]bool) (string, error) {
	p := strings.ToLower(strings.TrimSpace(primary))
	f := strings.ToLower(strings.TrimSpace(fallback))
	if p != "" && f != "" && p != f {
		return "", &InvalidInput{Message: fieldName + " conflicts with type"}
	}
	chosen := p
	if chosen == "" {
		chosen = f
	}
	if chosen == "" {
		chosen = defaultType
	}
	if !allowed[chosen] {
		keys := make([]string, 0, len(allowed))
		for k := range allowed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return "", &InvalidInput{Message: fmt.Sprintf("%s must be one of %s", fieldName, strings.Join(keys, " or "))}
	}
	return chosen, nil
}

func resolveExternalReference(externalID, externalRef *string) (*string, error) {
	idVal, err := validateOptionalString("external_id", externalID, MaxExternalRefRunes)
	if err != nil {
		return nil, err
	}
	refVal, err := validateOptionalString("external_reference", externalRef, MaxExternalRefRunes)
	if err != nil {
		return nil, err
	}
	if idVal != nil && refVal != nil && *idVal != *refVal {
		return nil, &InvalidInput{Message: "external_id conflicts with external_reference"}
	}
	if idVal != nil {
		return idVal, nil
	}
	return refVal, nil
}

func normalizeListFilter(filter ListFilter) (ListFilter, error) {
	var out ListFilter
	if s := strings.ToLower(strings.TrimSpace(filter.Status)); s != "" {
		if !IsValidStatus(s) {
			return out, &InvalidInput{Message: "status must be one of scheduled, active, or resolved"}
		}
		out.Status = s
	}
	if t := strings.ToLower(strings.TrimSpace(filter.Type)); t != "" {
		if !IsValidType(t) {
			return out, &InvalidInput{Message: "type must be one of delay, detour, cancellation, stop_closure, road_closure, construction, emergency, traffic_restriction, or accessibility"}
		}
		out.Type = t
	}
	for _, check := range []struct {
		name string
		val  int
	}{
		{"route_id", filter.RouteID},
		{"stop_id", filter.StopID},
		{"trip_id", filter.TripID},
		{"vehicle_id", filter.VehicleID},
	} {
		if check.val < 0 || check.val > maxInt32 {
			return out, &InvalidInput{Message: check.name + " must be a positive 32-bit integer"}
		}
	}
	out.RouteID = filter.RouteID
	out.StopID = filter.StopID
	out.TripID = filter.TripID
	out.VehicleID = filter.VehicleID

	if src := strings.TrimSpace(filter.SourceService); src != "" {
		validated, err := validateRequiredString("source_service", src, MaxSourceServiceRunes)
		if err != nil {
			return out, err
		}
		out.SourceService = validated
	}
	return out, nil
}

func validateRequiredString(name, raw string, maxRunes int) (string, error) {
	if strings.ContainsRune(raw, '\x00') {
		return "", &InvalidInput{Message: name + " cannot contain NUL characters"}
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", &InvalidInput{Message: name + " is required"}
	}
	if utf8.RuneCountInString(trimmed) > maxRunes {
		return "", &InvalidInput{Message: fmt.Sprintf("%s must be at most %d characters", name, maxRunes)}
	}
	return trimmed, nil
}

func validateOptionalString(name string, raw *string, maxRunes int) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	if strings.ContainsRune(*raw, '\x00') {
		return nil, &InvalidInput{Message: name + " cannot contain NUL characters"}
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(trimmed) > maxRunes {
		return nil, &InvalidInput{Message: fmt.Sprintf("%s must be at most %d characters", name, maxRunes)}
	}
	return &trimmed, nil
}

func validatePositiveID(name string, id int) error {
	if id <= 0 || id > maxInt32 {
		return &InvalidInput{Message: name + " must be a positive 32-bit integer"}
	}
	return nil
}

func validateIDSlice(name string, ids []int) ([]int, error) {
	if len(ids) > MaxListItems {
		return nil, &InvalidInput{Message: fmt.Sprintf("%s cannot exceed %d items", name, MaxListItems)}
	}
	seen := make(map[int]struct{}, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || id > maxInt32 {
			return nil, &InvalidInput{Message: name + " must contain only positive 32-bit integers"}
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Ints(out)
	return out, nil
}

func validateSegments(segments []string) ([]string, error) {
	if len(segments) > MaxListItems {
		return nil, &InvalidInput{Message: fmt.Sprintf("affected_segments cannot exceed %d items", MaxListItems)}
	}
	seen := make(map[string]struct{}, len(segments))
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		if strings.ContainsRune(seg, '\x00') {
			return nil, &InvalidInput{Message: "affected_segments cannot contain NUL characters"}
		}
		trimmed := strings.TrimSpace(seg)
		if trimmed == "" {
			return nil, &InvalidInput{Message: "affected_segments cannot contain empty strings"}
		}
		if utf8.RuneCountInString(trimmed) > MaxSegmentRunes {
			return nil, &InvalidInput{Message: fmt.Sprintf("each affected segment must be at most %d characters", MaxSegmentRunes)}
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out, nil
}

func uniqueSortedPositive(ids []int) []int {
	if len(ids) == 0 {
		return []int{}
	}
	seen := make(map[int]struct{}, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

func normalizeDisruptionResponse(d DisruptionResponse) DisruptionResponse {
	if d.AffectedRoutes == nil {
		d.AffectedRoutes = []AffectedRoute{}
	}
	if d.AffectedStops == nil {
		d.AffectedStops = []AffectedStop{}
	}
	if d.AffectedTrips == nil {
		d.AffectedTrips = []int{}
	}
	if d.AffectedVehicles == nil {
		d.AffectedVehicles = []int{}
	}
	if d.AffectedSegments == nil {
		d.AffectedSegments = []string{}
	}
	return d
}

func normalizeDisruptionSlice(items []DisruptionResponse) []DisruptionResponse {
	if len(items) == 0 {
		return []DisruptionResponse{}
	}
	out := make([]DisruptionResponse, len(items))
	for i, item := range items {
		out[i] = normalizeDisruptionResponse(item)
	}
	return out
}

func isFiniteInRange(val, minVal, maxVal float64) bool {
	return !math.IsNaN(val) && !math.IsInf(val, 0) && val >= minVal && val <= maxVal
}

func boundingBox(lat, lng, radiusMeters float64) (minLat, maxLat, minLng, maxLng float64) {
	latDelta := (radiusMeters / metersPerLatDeg) + 0.0005
	cosLat := math.Cos(lat * math.Pi / 180.0)
	if math.Abs(cosLat) < 1e-6 {
		cosLat = 1e-6
	}
	lngDelta := (radiusMeters / (metersPerLatDeg * math.Abs(cosLat))) + 0.0005
	minLat = math.Max(-90, lat-latDelta)
	maxLat = math.Min(90, lat+latDelta)
	minLng = math.Max(-180, lng-lngDelta)
	maxLng = math.Min(180, lng+lngDelta)
	return minLat, maxLat, minLng, maxLng
}

func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLng := (lng2 - lng1) * math.Pi / 180.0
	rLat1 := lat1 * math.Pi / 180.0
	rLat2 := lat2 * math.Pi / 180.0

	sinLat := math.Sin(dLat / 2)
	sinLng := math.Sin(dLng / 2)
	a := sinLat*sinLat + math.Cos(rLat1)*math.Cos(rLat2)*sinLng*sinLng
	if a > 1 {
		a = 1
	} else if a < 0 {
		a = 0
	}
	return 2 * earthRadiusMeters * math.Asin(math.Sqrt(a))
}
