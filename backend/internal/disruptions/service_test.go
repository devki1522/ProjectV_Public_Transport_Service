package disruptions

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

type fakeDisruptionRepo struct {
	listFilter          ListFilter
	listResult          []DisruptionResponse
	listErr             error
	getID               int
	getResult           DisruptionResponse
	getErr              error
	createParams        SaveDisruptionParams
	createResult        DisruptionResponse
	createErr           error
	updateID            int
	updateParams        PatchDisruptionParams
	updateResult        DisruptionResponse
	updateErr           error
	upsertParams        SaveDisruptionParams
	upsertResult        DisruptionResponse
	upsertCreated       bool
	upsertErr           error
	nearbyStops         []NearbyStop
	nearbyErr           error
	impactedTrips       []AffectedTripDetail
	impactedVehicles    []AffectedVehicleDetail
	routeExistsErr      error
	stopExistsErr       error
	tripRouteID         int
	tripRouteErr        error
	activeForTripResult []DisruptionResponse
	activeForTripErr    error
}

func (f *fakeDisruptionRepo) List(_ context.Context, filter ListFilter) ([]DisruptionResponse, error) {
	f.listFilter = filter
	return f.listResult, f.listErr
}

func (f *fakeDisruptionRepo) Get(_ context.Context, id int) (DisruptionResponse, error) {
	f.getID = id
	return f.getResult, f.getErr
}

func (f *fakeDisruptionRepo) Create(_ context.Context, params SaveDisruptionParams) (DisruptionResponse, error) {
	f.createParams = params
	if f.createErr != nil {
		return DisruptionResponse{}, f.createErr
	}
	if f.createResult.ID != 0 {
		return f.createResult, nil
	}
	return DisruptionResponse{
		ID:                101,
		Type:              params.Type,
		Title:             params.Title,
		Description:       params.Description,
		AffectedArea:      params.AffectedArea,
		StartTime:         params.StartTime,
		EndTime:           params.EndTime,
		Status:            params.Status,
		SourceService:     params.SourceService,
		ExternalReference: params.ExternalReference,
		ReceivedAt:        params.ReceivedAt,
		AffectedTrips:     params.TripIDs,
		AffectedVehicles:  params.VehicleIDs,
		AffectedSegments:  params.AffectedSegments,
	}, nil
}

func (f *fakeDisruptionRepo) Update(_ context.Context, id int, params PatchDisruptionParams) (DisruptionResponse, error) {
	f.updateID = id
	f.updateParams = params
	return f.updateResult, f.updateErr
}

func (f *fakeDisruptionRepo) UpsertByExternalRef(_ context.Context, params SaveDisruptionParams) (DisruptionResponse, bool, error) {
	f.upsertParams = params
	if f.upsertErr != nil {
		return DisruptionResponse{}, false, f.upsertErr
	}
	return f.upsertResult, f.upsertCreated, nil
}

func (f *fakeDisruptionRepo) FindStopsInBounds(_ context.Context, _, _, _, _ float64) ([]NearbyStop, error) {
	return f.nearbyStops, f.nearbyErr
}

func (f *fakeDisruptionRepo) ListActiveImpactedTrips(_ context.Context, _ []int) ([]AffectedTripDetail, error) {
	return f.impactedTrips, nil
}

func (f *fakeDisruptionRepo) ListActiveImpactedVehicles(_ context.Context, _ []int) ([]AffectedVehicleDetail, error) {
	return f.impactedVehicles, nil
}

func (f *fakeDisruptionRepo) RouteExists(_ context.Context, _ int) error {
	return f.routeExistsErr
}

func (f *fakeDisruptionRepo) StopExists(_ context.Context, _ int) error {
	return f.stopExistsErr
}

func (f *fakeDisruptionRepo) TripRouteID(_ context.Context, _ int) (int, error) {
	return f.tripRouteID, f.tripRouteErr
}

func (f *fakeDisruptionRepo) ListActiveForTrip(_ context.Context, _, _ int) ([]DisruptionResponse, error) {
	return f.activeForTripResult, f.activeForTripErr
}

func TestServiceCreateValidationAndDefaults(t *testing.T) {
	fixedNow := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	repo := &fakeDisruptionRepo{}
	svc := NewServiceWithClock(repo, func() time.Time { return fixedNow })

	desc := "  King St closed between University and Columbia  "
	area := "  Waterloo North  "
	res, err := svc.Create(context.Background(), CreateDisruptionInput{
		Type:             "  DETOUR ",
		Title:            "  King Street Detour  ",
		Description:      &desc,
		AffectedArea:     &area,
		RouteIDs:         []int{9, 7, 7},
		StopIDs:          []int{42, 12, 42},
		TripIDs:          []int{300, 100},
		VehicleIDs:       []int{22, 11},
		AffectedSegments: []string{" King St N ", "King St N", "University Ave W"},
	}, "")
	if err != nil {
		t.Fatalf("unexpected create error: %v", err)
	}
	if res.Type != TypeDetour || res.Status != StatusActive || res.SourceService != SourceTransitAdmin {
		t.Fatalf("unexpected created disruption defaults: %+v", res)
	}
	if !res.ReceivedAt.Equal(fixedNow) || !res.StartTime.Equal(fixedNow) {
		t.Fatalf("expected received_at and start_time = %v, got %v / %v", fixedNow, res.ReceivedAt, res.StartTime)
	}
	if len(repo.createParams.RouteIDs) != 2 || repo.createParams.RouteIDs[0] != 7 || repo.createParams.RouteIDs[1] != 9 {
		t.Fatalf("expected deduplicated sorted route_ids [7 9], got %+v", repo.createParams.RouteIDs)
	}
	if len(repo.createParams.AffectedSegments) != 2 {
		t.Fatalf("expected deduplicated segments, got %+v", repo.createParams.AffectedSegments)
	}

	// Future start_time defaults status to scheduled when omitted.
	futureStart := fixedNow.Add(2 * time.Hour)
	futureEnd := fixedNow.Add(4 * time.Hour)
	futureRes, err := svc.Create(context.Background(), CreateDisruptionInput{
		Type:      TypeConstruction,
		Title:     "Overnight Track Maintenance",
		StartTime: &futureStart,
		EndTime:   &futureEnd,
	}, "scheduler")
	if err != nil {
		t.Fatalf("future create failed: %v", err)
	}
	if futureRes.Status != StatusScheduled || futureRes.SourceService != "scheduler" {
		t.Fatalf("expected status=scheduled source=scheduler, got %+v", futureRes)
	}

	// Strict validation failures (PT-NFR-006).
	pastEnd := fixedNow.Add(-time.Minute)
	longTitle := strings.Repeat("a", 101)
	nulString := "bad\x00value"
	for _, tc := range []struct {
		name  string
		input CreateDisruptionInput
	}{
		{"invalid type", CreateDisruptionInput{Type: "meteor", Title: "Valid"}},
		{"empty title", CreateDisruptionInput{Type: TypeDelay, Title: "   "}},
		{"overlong title", CreateDisruptionInput{Type: TypeDelay, Title: longTitle}},
		{"nul in title", CreateDisruptionInput{Type: TypeDelay, Title: nulString}},
		{"invalid status", CreateDisruptionInput{Type: TypeDelay, Title: "Valid", Status: "archived"}},
		{"end before start", CreateDisruptionInput{Type: TypeDelay, Title: "Valid", StartTime: &fixedNow, EndTime: &pastEnd}},
		{"negative route_id", CreateDisruptionInput{Type: TypeDelay, Title: "Valid", RouteIDs: []int{0}}},
		{"negative stop_id", CreateDisruptionInput{Type: TypeDelay, Title: "Valid", StopIDs: []int{-3}}},
		{"negative trip_id", CreateDisruptionInput{Type: TypeDelay, Title: "Valid", TripIDs: []int{0}}},
		{"negative vehicle_id", CreateDisruptionInput{Type: TypeDelay, Title: "Valid", VehicleIDs: []int{-1}}},
		{"empty segment", CreateDisruptionInput{Type: TypeDelay, Title: "Valid", AffectedSegments: []string{"  "}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), tc.input, "admin")
			var invalid *InvalidInput
			if !errors.As(err, &invalid) {
				t.Fatalf("expected *InvalidInput for %s, got %v", tc.name, err)
			}
		})
	}
}

func TestServiceUpdateAndListActive(t *testing.T) {
	fixedNow := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	repo := &fakeDisruptionRepo{
		listResult: []DisruptionResponse{
			{ID: 1, Type: TypeDelay, Title: "Active Delay", Status: StatusActive},
			{ID: 2, Type: TypeDetour, Title: "Resolved Detour", Status: StatusResolved},
		},
		updateResult: DisruptionResponse{
			ID:     1,
			Type:   TypeDelay,
			Title:  "Resolved Delay",
			Status: StatusResolved,
		},
	}
	svc := NewServiceWithClock(repo, func() time.Time { return fixedNow })

	// ListActive enforces status=active and filters out any non-active rows defensively.
	active, err := svc.ListActive(context.Background(), ListFilter{Type: TypeDelay})
	if err != nil {
		t.Fatal(err)
	}
	if repo.listFilter.Status != StatusActive || len(active) != 1 || active[0].ID != 1 {
		t.Fatalf("unexpected ListActive result: filter=%+v items=%+v", repo.listFilter, active)
	}

	// Passing status=resolved to ListActive is rejected.
	if _, err := svc.ListActive(context.Background(), ListFilter{Status: StatusResolved}); err == nil {
		t.Fatal("expected error when requesting non-active status from ListActive")
	}

	// Empty update payload is rejected.
	if _, err := svc.Update(context.Background(), 1, UpdateDisruptionInput{}); err == nil {
		t.Fatal("expected error on empty UpdateDisruptionInput")
	}

	// Valid partial status update.
	resolved := StatusResolved
	updated, err := svc.Update(context.Background(), 1, UpdateDisruptionInput{Status: &resolved, EndTimeSet: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != StatusResolved || !repo.updateParams.EndTimeSet || repo.updateParams.EndTime != nil {
		t.Fatalf("unexpected update result: %+v params=%+v", updated, repo.updateParams)
	}
}

func TestExternalNoticeIngestionSpatialMatchingAndUpsert(t *testing.T) {
	fixedNow := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	repo := &fakeDisruptionRepo{
		nearbyStops: []NearbyStop{
			{ID: 3, Name: "Midtown Hub", Latitude: 43.4505, Longitude: -80.4905},         // ~70m away -> matched
			{ID: 4, Name: "University Terminal", Latitude: 43.4700, Longitude: -80.5400}, // >4km away -> excluded
		},
	}
	svc := NewServiceWithClock(repo, func() time.Time { return fixedNow })

	extID := "UTIL-2026-900"
	lat := 43.4500
	lng := -80.4900
	radius := 350.0
	repo.upsertCreated = true
	repo.upsertResult = DisruptionResponse{
		ID:                55,
		Type:              TypeRoadClosure,
		Title:             "Watermain Break on King St",
		Status:            StatusActive,
		SourceService:     SourceUtilities,
		ExternalReference: &extID,
		ReceivedAt:        fixedNow,
		AffectedRoutes: []AffectedRoute{
			{RouteID: 7, RouteNumber: "7", RouteName: "King", Direction: "direction_0"},
		},
		AffectedStops: []AffectedStop{
			{StopID: 3, StopName: "Midtown Hub", RouteID: 7},
		},
		AffectedSegments: []string{"King St between Union and Erb"},
	}

	// US 186 Utilities notice ingestion with spatial matching.
	res, err := svc.IngestUtilitiesNotice(context.Background(), UtilitiesNoticeInput{
		ExternalID:       &extID,
		NoticeType:       TypeRoadClosure,
		Title:            "Watermain Break on King St",
		Latitude:         &lat,
		Longitude:        &lng,
		RadiusMeters:     &radius,
		AffectedSegments: []string{"King St between Union and Erb"},
	})
	if err != nil {
		t.Fatalf("IngestUtilitiesNotice failed: %v", err)
	}
	if !res.Created || res.Disruption.SourceService != SourceUtilities || !res.Disruption.ReceivedAt.Equal(fixedNow) {
		t.Fatalf("unexpected utilities result: %+v", res)
	}
	if len(repo.upsertParams.StopIDs) != 1 || repo.upsertParams.StopIDs[0] != 3 {
		t.Fatalf("expected spatial match on stop 3 only, got %+v", repo.upsertParams.StopIDs)
	}
	if len(res.MatchedRouteIDs) != 1 || res.MatchedRouteIDs[0] != 7 || len(res.MatchedStopIDs) != 1 || res.MatchedStopIDs[0] != 3 {
		t.Fatalf("unexpected matched IDs: routes=%v stops=%v", res.MatchedRouteIDs, res.MatchedStopIDs)
	}
	if !strings.HasPrefix(res.TransitImpact, "high:") {
		t.Fatalf("expected high transit impact for road_closure, got %q", res.TransitImpact)
	}

	// US 187 Emergency incident idempotent update (created=false) with critical severity.
	repo.upsertCreated = false
	repo.upsertResult.SourceService = SourceEmergency
	repo.upsertResult.Type = TypeEmergency
	emRes, err := svc.IngestEmergencyIncident(context.Background(), EmergencyIncidentInput{
		ExternalID:   &extID,
		IncidentType: TypeEmergency,
		Severity:     "critical",
		Title:        "Multi-vehicle collision",
		RouteIDs:     []int{7},
	})
	if err != nil {
		t.Fatalf("IngestEmergencyIncident failed: %v", err)
	}
	if emRes.Created || !strings.HasPrefix(emRes.TransitImpact, "critical:") {
		t.Fatalf("expected created=false and critical impact, got %+v", emRes)
	}

	// US 188 Security traffic alert defaults to traffic_restriction.
	repo.upsertCreated = true
	repo.upsertResult.SourceService = SourceSecurity
	repo.upsertResult.Type = TypeTrafficRestriction
	secRes, err := svc.IngestSecurityTrafficAlert(context.Background(), SecurityTrafficAlertInput{
		Title:    "Police motorcade restriction",
		RouteIDs: []int{7},
	})
	if err != nil {
		t.Fatalf("IngestSecurityTrafficAlert failed: %v", err)
	}
	if !secRes.Created || repo.createParams.SourceService != SourceSecurity || repo.createParams.Type != TypeTrafficRestriction {
		t.Fatalf("unexpected security alert result: %+v params=%+v", secRes, repo.createParams)
	}

	// Reject disallowed feed types, half-specified coordinates, and out-of-range radius.
	if _, err := svc.IngestUtilitiesNotice(context.Background(), UtilitiesNoticeInput{
		NoticeType: TypeEmergency,
		Title:      "Invalid utilities type",
	}); err == nil {
		t.Fatal("expected error for utilities notice with type=emergency")
	}
	if _, err := svc.IngestEmergencyIncident(context.Background(), EmergencyIncidentInput{
		Title:    "Missing longitude",
		Latitude: &lat,
	}); err == nil {
		t.Fatal("expected error when latitude provided without longitude")
	}
	badRadius := 20.0
	if _, err := svc.IngestSecurityTrafficAlert(context.Background(), SecurityTrafficAlertInput{
		Title:        "Radius too small",
		Latitude:     &lat,
		Longitude:    &lng,
		RadiusMeters: &badRadius,
	}); err == nil {
		t.Fatal("expected error for radius_meters < 50")
	}
	nanVal := math.NaN()
	if _, err := svc.IngestSecurityTrafficAlert(context.Background(), SecurityTrafficAlertInput{
		Title:     "NaN lat",
		Latitude:  &nanVal,
		Longitude: &lng,
	}); err == nil {
		t.Fatal("expected error for NaN latitude")
	}
}

func TestServiceStatusAffectedServicesAndEntityStatuses(t *testing.T) {
	fixedNow := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	repo := &fakeDisruptionRepo{
		listResult: []DisruptionResponse{
			{
				ID:            10,
				Type:          TypeDetour,
				Title:         "King Detour",
				Status:        StatusActive,
				SourceService: SourceUtilities,
				StartTime:     fixedNow.Add(-time.Hour),
				ReceivedAt:    fixedNow.Add(-time.Hour),
				AffectedRoutes: []AffectedRoute{
					{RouteID: 7, RouteNumber: "7", RouteName: "King", Direction: "direction_0"},
				},
				AffectedStops: []AffectedStop{
					{StopID: 3, StopName: "Midtown Hub", RouteID: 7},
				},
				AffectedTrips:    []int{100},
				AffectedVehicles: []int{55},
				AffectedSegments: []string{"King St & Erb St"},
			},
			{
				ID:            11,
				Type:          TypeStopClosure,
				Title:         "Midtown Platform Closure",
				Status:        StatusActive,
				SourceService: SourceTransitAdmin,
				StartTime:     fixedNow.Add(-30 * time.Minute),
				ReceivedAt:    fixedNow.Add(-30 * time.Minute),
				AffectedRoutes: []AffectedRoute{
					{RouteID: 7, RouteNumber: "7", RouteName: "King", Direction: "direction_0"},
				},
				AffectedStops: []AffectedStop{
					{StopID: 3, StopName: "Midtown Hub", RouteID: 7},
				},
			},
		},
		impactedTrips: []AffectedTripDetail{
			{DisruptionID: 10, TripID: 100, RouteID: 7},
		},
		impactedVehicles: []AffectedVehicleDetail{
			{DisruptionID: 10, VehicleID: 55, VehicleNumber: "GRT-301"},
		},
		tripRouteID: 7,
		activeForTripResult: []DisruptionResponse{
			{ID: 10, Type: TypeDetour, Title: "King Detour", Status: StatusActive},
		},
	}
	svc := NewServiceWithClock(repo, func() time.Time { return fixedNow })

	status, err := svc.ServiceStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.OverallStatus != OverallStatusDisrupted || status.ActiveDisruptionCount != 2 || len(status.Detours) != 1 {
		t.Fatalf("unexpected service status: %+v", status)
	}
	if len(status.AffectedRoutes) != 1 || status.AffectedRoutes[0].RouteID != 7 || len(status.AffectedRoutes[0].DisruptionIDs) != 2 {
		t.Fatalf("unexpected affected routes: %+v", status.AffectedRoutes)
	}
	// Stop 3 has both detour and stop_closure; stop operational status must resolve to "closed".
	if len(status.AffectedStops) != 1 || status.AffectedStops[0].OperationalStatus != OpStatusClosed {
		t.Fatalf("expected stop operational_status=closed, got %+v", status.AffectedStops)
	}
	if len(status.AffectedTrips) != 1 || status.AffectedTrips[0].OperationalStatus != OpStatusDetoured {
		t.Fatalf("expected trip operational_status=detoured, got %+v", status.AffectedTrips)
	}
	if len(status.AffectedVehicles) != 1 || status.AffectedVehicles[0].VehicleNumber != "GRT-301" {
		t.Fatalf("unexpected affected vehicles: %+v", status.AffectedVehicles)
	}
	if len(status.AffectedSegments) != 1 || status.AffectedSegments[0].RouteID == nil || *status.AffectedSegments[0].RouteID != 7 {
		t.Fatalf("unexpected affected segments: %+v", status.AffectedSegments)
	}

	// Entity status endpoints.
	routeSt, err := svc.RouteStatus(context.Background(), 7)
	if err != nil || routeSt.EntityType != "route" || routeSt.OperationalStatus != OpStatusDisrupted {
		t.Fatalf("unexpected route status: err=%v status=%+v", err, routeSt)
	}
	stopSt, err := svc.StopStatus(context.Background(), 3)
	if err != nil || stopSt.EntityType != "stop" || stopSt.OperationalStatus != OpStatusClosed {
		t.Fatalf("unexpected stop status: err=%v status=%+v", err, stopSt)
	}
	tripSt, err := svc.TripStatus(context.Background(), 100)
	if err != nil || tripSt.EntityType != "trip" || tripSt.OperationalStatus != OpStatusDetoured {
		t.Fatalf("unexpected trip status: err=%v status=%+v", err, tripSt)
	}

	// Missing entities return ErrNotFound.
	repo.routeExistsErr = ErrNotFound
	if _, err := svc.RouteStatus(context.Background(), 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing route, got %v", err)
	}
}
