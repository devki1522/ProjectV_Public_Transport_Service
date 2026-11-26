package disruptions

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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

func TestCreateRoadClosureAddressValidUntilAndSpatialMatching(t *testing.T) {
	fixedNow := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	repo := &fakeDisruptionRepo{
		nearbyStops: []NearbyStop{
			{ID: 3, Name: "Midtown Hub", Latitude: 43.4505, Longitude: -80.4905},         // ~70m away -> matched
			{ID: 4, Name: "University Terminal", Latitude: 43.4700, Longitude: -80.5400}, // >4km away -> excluded
		},
	}
	svc := NewServiceWithClock(repo, func() time.Time { return fixedNow })

	// 1. Address with numeric streetNumber + duration valid-until (7200s) + spatial match.
	var addrNum RoadClosureAddress
	if err := json.Unmarshal([]byte(`{"streetNumber":123,"streetName":"King St N","city":"Waterloo","postalCode":"n2j 2x7"}`), &addrNum); err != nil {
		t.Fatalf("unmarshal numeric streetNumber address: %v", err)
	}
	if addrNum.Format() != "123 King St N, Waterloo, N2J 2X7" {
		t.Fatalf("Format() = %q", addrNum.Format())
	}

	extID := "UTIL-2026-900"
	lat := 43.4500
	lng := -80.4900
	radius := 350.0
	validDuration := uint64(7200)
	repo.createResult = DisruptionResponse{
		ID:                55,
		Type:              TypeRoadClosure,
		Title:             "Road closure: 123 King St N, Waterloo, N2J 2X7",
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

	res, err := svc.CreateRoadClosure(context.Background(), RoadClosureInput{
		Address:          &addrNum,
		ValidUntil:       &validDuration,
		ExternalID:       &extID,
		Latitude:         &lat,
		Longitude:        &lng,
		RadiusMeters:     &radius,
		AffectedSegments: []string{"King St between Union and Erb"},
	}, SourceUtilities)
	if err != nil {
		t.Fatalf("CreateRoadClosure failed: %v", err)
	}
	if res.ID != 55 || repo.createParams.SourceService != SourceUtilities || repo.createParams.Type != TypeRoadClosure {
		t.Fatalf("unexpected result/params: %+v params=%+v", res, repo.createParams)
	}
	if repo.createParams.Title != "Road closure: 123 King St N, Waterloo, N2J 2X7" {
		t.Fatalf("derived title = %q", repo.createParams.Title)
	}
	if repo.createParams.AffectedArea == nil || *repo.createParams.AffectedArea != "123 King St N, Waterloo, N2J 2X7" {
		t.Fatalf("derived affected_area = %v", repo.createParams.AffectedArea)
	}
	wantEnd := fixedNow.Add(7200 * time.Second)
	if repo.createParams.EndTime == nil || !repo.createParams.EndTime.Equal(wantEnd) {
		t.Fatalf("EndTime = %v, want %v", repo.createParams.EndTime, wantEnd)
	}
	if len(repo.createParams.StopIDs) != 1 || repo.createParams.StopIDs[0] != 3 {
		t.Fatalf("expected spatial match on stop 3 only, got %+v", repo.createParams.StopIDs)
	}

	// 2. String streetNumber + Unix epoch timestamp valid-until creates a new closure cleanly.
	var addrStr RoadClosureAddress
	if err := json.Unmarshal([]byte(`{"streetNumber":"45B","streetName":"Weber St E","city":"Kitchener","postalCode":"N2H 1C4"}`), &addrStr); err != nil {
		t.Fatalf("unmarshal string streetNumber address: %v", err)
	}
	epochEnd := uint64(fixedNow.Add(4 * time.Hour).Unix())
	repo.createResult = DisruptionResponse{
		ID:            56,
		Type:          TypeRoadClosure,
		Title:         "Collision closure",
		Status:        StatusActive,
		SourceService: SourceEmergency,
		ReceivedAt:    fixedNow,
	}
	res2, err := svc.CreateRoadClosure(context.Background(), RoadClosureInput{
		Address:    &addrStr,
		ValidUntil: &epochEnd,
		ExternalID: &extID,
		Title:      "Collision closure",
	}, SourceEmergency)
	if err != nil {
		t.Fatalf("CreateRoadClosure epoch create failed: %v", err)
	}
	if res2.ID != 56 || repo.createParams.EndTime == nil || repo.createParams.EndTime.Unix() != int64(epochEnd) {
		t.Fatalf("unexpected epoch create: res=%+v end=%v", res2, repo.createParams.EndTime)
	}

	// 3. Long formatted address safely truncates derived title to MaxTitleRunes (100).
	longStreet := strings.Repeat("LongStreetName", 7) // 98 chars
	repo.createResult = DisruptionResponse{}
	longRes, err := svc.CreateRoadClosure(context.Background(), RoadClosureInput{
		Address: &RoadClosureAddress{
			StreetNumber: "100",
			StreetName:   longStreet,
			City:         "Waterloo",
			PostalCode:   "N2L 3G1",
		},
	}, SourceSecurity)
	if err != nil {
		t.Fatalf("CreateRoadClosure long address failed: %v", err)
	}
	if longRes.ID == 0 || utf8.RuneCountInString(repo.createParams.Title) != MaxTitleRunes {
		t.Fatalf("expected derived title truncated to %d runes, got %d (%q)", MaxTitleRunes, utf8.RuneCountInString(repo.createParams.Title), repo.createParams.Title)
	}

	// 4. Address JSON validation errors.
	for _, badAddrJSON := range []string{
		`{"streetNumber":-5,"streetName":"King St","city":"Waterloo","postalCode":"N2J 2X7"}`,
		`{"streetNumber":12.5,"streetName":"King St","city":"Waterloo","postalCode":"N2J 2X7"}`,
		`{"streetNumber":"10","streetName":"King St","city":"Waterloo","extra":"nope"}`,
		`{"streetNumber":"10","street_number":"11","streetName":"King St","city":"Waterloo"}`,
	} {
		var bad RoadClosureAddress
		if err := json.Unmarshal([]byte(badAddrJSON), &bad); err == nil {
			t.Fatalf("expected address unmarshal error for %s", badAddrJSON)
		}
	}

	// 5. Service validation errors.
	zeroValidUntil := uint64(0)
	expiredEpoch := uint64(fixedNow.Add(-time.Minute).Unix())
	tooLongDuration := uint64(maxDurationSeconds + 1)
	endTimeVal := fixedNow.Add(time.Hour)
	badRadius := 20.0
	nanVal := math.NaN()

	for _, tc := range []struct {
		name  string
		input RoadClosureInput
	}{
		{"valid-until 0", RoadClosureInput{Title: "T", ValidUntil: &zeroValidUntil}},
		{"expired unix valid-until", RoadClosureInput{Title: "T", ValidUntil: &expiredEpoch}},
		{"valid-until duration > 10y", RoadClosureInput{Title: "T", ValidUntil: &tooLongDuration}},
		{"both valid-until and end_time", RoadClosureInput{Title: "T", ValidUntil: &validDuration, EndTime: &endTimeVal}},
		{"missing streetName", RoadClosureInput{Address: &RoadClosureAddress{StreetNumber: "1", City: "Waterloo"}}},
		{"missing city", RoadClosureInput{Address: &RoadClosureAddress{StreetNumber: "1", StreetName: "King St"}}},
		{"non-road_closure type", RoadClosureInput{Title: "T", Type: TypeDelay}},
		{"missing title and address", RoadClosureInput{ValidUntil: &validDuration}},
		{"half-specified lat without lng", RoadClosureInput{Title: "T", Latitude: &lat}},
		{"radius < 50", RoadClosureInput{Title: "T", Latitude: &lat, Longitude: &lng, RadiusMeters: &badRadius}},
		{"NaN latitude", RoadClosureInput{Title: "T", Latitude: &nanVal, Longitude: &lng}},
	} {
		if _, err := svc.CreateRoadClosure(context.Background(), tc.input, SourceTransitAdmin); err == nil {
			t.Fatalf("%s: expected validation error, got nil", tc.name)
		}
	}
}

func TestGetAndUpdateRoadClosure(t *testing.T) {
	fixedNow := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	repo := &fakeDisruptionRepo{
		getResult: DisruptionResponse{
			ID:        2,
			Type:      TypeRoadClosure,
			Title:     "Fire emergency",
			Status:    StatusActive,
			StartTime: fixedNow.Add(-time.Hour),
		},
		updateResult: DisruptionResponse{
			ID:     2,
			Type:   TypeRoadClosure,
			Title:  "Fire emergency",
			Status: StatusResolved,
		},
	}
	svc := NewServiceWithClock(repo, func() time.Time { return fixedNow })

	got, err := svc.GetRoadClosure(context.Background(), 2)
	if err != nil || got.ID != 2 || got.AffectedRoutes == nil {
		t.Fatalf("GetRoadClosure failed: got=%+v err=%v", got, err)
	}

	resolved := StatusResolved
	updated, err := svc.UpdateRoadClosure(context.Background(), 2, UpdateRoadClosureInput{
		Status: &resolved,
	})
	if err != nil || updated.Status != StatusResolved {
		t.Fatalf("UpdateRoadClosure resolve failed: updated=%+v err=%v", updated, err)
	}
	if repo.updateParams.Status == nil || *repo.updateParams.Status != StatusResolved || repo.updateParams.Title != nil {
		t.Fatalf("expected status-only update preserving other fields, got %+v", repo.updateParams)
	}

	// Address and valid-until updates translate into affected_area and end_time.
	extendSec := uint64(3600)
	_, err = svc.UpdateRoadClosure(context.Background(), 2, UpdateRoadClosureInput{
		Address: &RoadClosureAddress{
			StreetNumber: "10",
			StreetName:   "King St W",
			City:         "Cambridge",
			PostalCode:   "N1R 1P1",
		},
		ValidUntil: &extendSec,
	})
	if err != nil {
		t.Fatalf("UpdateRoadClosure address+valid-until failed: %v", err)
	}
	if !repo.updateParams.AffectedAreaSet || repo.updateParams.AffectedArea == nil || *repo.updateParams.AffectedArea != "10 King St W, Cambridge, N1R 1P1" {
		t.Fatalf("unexpected AffectedArea params: %+v", repo.updateParams)
	}
	if !repo.updateParams.EndTimeSet || repo.updateParams.EndTime == nil || !repo.updateParams.EndTime.Equal(fixedNow.Add(time.Hour)) {
		t.Fatalf("unexpected EndTime params: %+v", repo.updateParams)
	}

	// valid-until duration on a future-scheduled closure anchors to max(now, existing.StartTime).
	futureStart := fixedNow.Add(24 * time.Hour)
	repo.getResult.StartTime = futureStart
	if _, err := svc.UpdateRoadClosure(context.Background(), 2, UpdateRoadClosureInput{
		ValidUntil: &extendSec,
	}); err != nil {
		t.Fatalf("UpdateRoadClosure future valid-until failed: %v", err)
	}
	if repo.updateParams.EndTime == nil || !repo.updateParams.EndTime.Equal(futureStart.Add(time.Hour)) {
		t.Fatalf("expected future EndTime = %v, got %v", futureStart.Add(time.Hour), repo.updateParams.EndTime)
	}
	repo.getResult.StartTime = fixedNow.Add(-time.Hour)

	// Empty update and conflicting valid-until + end_time are rejected.
	if _, err := svc.UpdateRoadClosure(context.Background(), 2, UpdateRoadClosureInput{}); err == nil {
		t.Fatal("expected error on empty UpdateRoadClosureInput")
	}
	if _, err := svc.UpdateRoadClosure(context.Background(), 2, UpdateRoadClosureInput{ValidUntil: &extendSec, EndTimeSet: true}); err == nil {
		t.Fatal("expected error when both valid-until and end_time are provided")
	}

	// Non-road_closure disruptions return ErrNotFound for both GetRoadClosure and UpdateRoadClosure.
	repo.getResult.Type = TypeDetour
	if _, err := svc.GetRoadClosure(context.Background(), 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetRoadClosure non-road_closure = %v, want ErrNotFound", err)
	}
	if _, err := svc.UpdateRoadClosure(context.Background(), 2, UpdateRoadClosureInput{Status: &resolved}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateRoadClosure non-road_closure = %v, want ErrNotFound", err)
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
