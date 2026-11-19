package vehicles

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

type fakeVehicleRepo struct {
	upsertParams   LocationUpdateParams
	upsertResult   LocationUpdateResult
	upsertErr      error
	listCutoff     time.Time
	listRouteID    int
	listRouteNum   string
	listTripID     int
	listStopID     int
	listResult     []ActiveVehicle
	listErr        error
	routeExistsErr error
	tripExistsErr  error
	stopExistsErr  error
	geomResult     RouteGeometry
	geomErr        error
	simRoutes      []SimulatedRoute
	simRoutesErr   error
	clearedSim     bool
	clearErr       error
	tripStops      []TripStopPoint
	tripStopsErr   error
	routeStops     []RouteStopPoint
	routeStopsErr  error
}

func (f *fakeVehicleRepo) UpsertLocation(_ context.Context, p LocationUpdateParams) (LocationUpdateResult, error) {
	f.upsertParams = p
	return f.upsertResult, f.upsertErr
}

func (f *fakeVehicleRepo) ListActiveVehicles(_ context.Context, cutoff time.Time, routeID int, routeNumber string, tripID int) ([]ActiveVehicle, error) {
	f.listCutoff = cutoff
	f.listRouteID = routeID
	f.listRouteNum = routeNumber
	f.listTripID = tripID
	return f.listResult, f.listErr
}

func (f *fakeVehicleRepo) ListActiveVehiclesForStop(_ context.Context, cutoff time.Time, stopID, routeID int) ([]ActiveVehicle, error) {
	f.listCutoff = cutoff
	f.listStopID = stopID
	f.listRouteID = routeID
	return f.listResult, f.listErr
}

func (f *fakeVehicleRepo) RouteExists(context.Context, int) error { return f.routeExistsErr }
func (f *fakeVehicleRepo) TripExists(context.Context, int) error  { return f.tripExistsErr }
func (f *fakeVehicleRepo) StopExists(context.Context, int) error  { return f.stopExistsErr }
func (f *fakeVehicleRepo) RouteGeometry(context.Context, int) (RouteGeometry, error) {
	return f.geomResult, f.geomErr
}
func (f *fakeVehicleRepo) ActiveRoutesWithGeometries(context.Context, int) ([]SimulatedRoute, error) {
	return f.simRoutes, f.simRoutesErr
}
func (f *fakeVehicleRepo) ClearSimulatedLocations(context.Context) error {
	f.clearedSim = true
	return f.clearErr
}
func (f *fakeVehicleRepo) LoadTripStops(_ context.Context, tripIDs []int) ([]TripStopPoint, error) {
	if f.tripStopsErr != nil {
		return nil, f.tripStopsErr
	}
	allowed := make(map[int]struct{}, len(tripIDs))
	for _, id := range tripIDs {
		allowed[id] = struct{}{}
	}
	out := make([]TripStopPoint, 0, len(f.tripStops))
	for _, pt := range f.tripStops {
		if _, ok := allowed[pt.TripID]; ok {
			out = append(out, pt)
		}
	}
	return out, nil
}
func (f *fakeVehicleRepo) LoadRouteStops(_ context.Context, routeIDs []int) ([]RouteStopPoint, error) {
	if f.routeStopsErr != nil {
		return nil, f.routeStopsErr
	}
	allowed := make(map[int]struct{}, len(routeIDs))
	for _, id := range routeIDs {
		allowed[id] = struct{}{}
	}
	out := make([]RouteStopPoint, 0, len(f.routeStops))
	for _, pt := range f.routeStops {
		if _, ok := allowed[pt.RouteID]; ok {
			out = append(out, pt)
		}
	}
	return out, nil
}

func floatPtr(v float64) *float64 { return &v }
func intPtr(v int) *int           { return &v }

func TestSubmitLocationValidationAndDefaults(t *testing.T) {
	now := time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)
	fixedClock := func() time.Time { return now }

	t.Run("accepts valid payload and retains source_service and received_at", func(t *testing.T) {
		repo := &fakeVehicleRepo{
			upsertResult: LocationUpdateResult{
				Created: true,
				Vehicle: ActiveVehicle{VehicleID: 10, VehicleNumber: "GRT-201", RouteID: 7},
			},
		}
		svc := NewServiceWithClock(repo, fixedClock)
		recorded := now.Add(-15 * time.Second)

		res, err := svc.SubmitLocation(context.Background(), LocationUpdateInput{
			VehicleNumber: "  GRT-201 ",
			RouteID:       7,
			TripID:        intPtr(101),
			Latitude:      floatPtr(43.4516),
			Longitude:     floatPtr(-80.4925),
			Heading:       floatPtr(90),
			SpeedKmh:      floatPtr(36.5),
			RecordedAt:    &recorded,
		}, "dispatch-api")
		if err != nil {
			t.Fatal(err)
		}
		if !res.Created || res.Vehicle.VehicleNumber != "GRT-201" {
			t.Fatalf("unexpected result: %+v", res)
		}
		if repo.upsertParams.VehicleNumber != "GRT-201" || repo.upsertParams.Status != StatusInService {
			t.Fatalf("unexpected normalized params: %+v", repo.upsertParams)
		}
		if repo.upsertParams.SourceService != "dispatch-api" || !repo.upsertParams.ReceivedAt.Equal(now) {
			t.Fatalf("expected source_service and server received_at retained, got %+v", repo.upsertParams)
		}
		if !repo.upsertParams.RecordedAt.Equal(recorded) {
			t.Fatalf("recorded_at = %v, want %v", repo.upsertParams.RecordedAt, recorded)
		}
	})

	t.Run("simulator update defaults source_service to simulator", func(t *testing.T) {
		repo := &fakeVehicleRepo{}
		svc := NewServiceWithClock(repo, fixedClock)
		if _, err := svc.SubmitLocation(context.Background(), LocationUpdateInput{
			VehicleNumber: "SIM-001",
			RouteID:       1,
			Latitude:      floatPtr(43.45),
			Longitude:     floatPtr(-80.49),
			IsSimulated:   true,
		}, "ignored-fallback"); err != nil {
			t.Fatal(err)
		}
		if repo.upsertParams.SourceService != "simulator" {
			t.Fatalf("source_service = %q, want simulator", repo.upsertParams.SourceService)
		}
	})

	future := now.Add(5 * time.Minute)
	badCases := []struct {
		name  string
		input LocationUpdateInput
	}{
		{"missing vehicle id and number", LocationUpdateInput{RouteID: 1, Latitude: floatPtr(43.4), Longitude: floatPtr(-80.4)}},
		{"non-positive vehicle_id", LocationUpdateInput{VehicleID: intPtr(0), RouteID: 1, Latitude: floatPtr(43.4), Longitude: floatPtr(-80.4)}},
		{"vehicle_number too long", LocationUpdateInput{VehicleNumber: strings.Repeat("B", 21), RouteID: 1, Latitude: floatPtr(43.4), Longitude: floatPtr(-80.4)}},
		{"non-positive route_id", LocationUpdateInput{VehicleNumber: "B1", RouteID: 0, Latitude: floatPtr(43.4), Longitude: floatPtr(-80.4)}},
		{"non-positive trip_id", LocationUpdateInput{VehicleNumber: "B1", RouteID: 1, TripID: intPtr(0), Latitude: floatPtr(43.4), Longitude: floatPtr(-80.4)}},
		{"missing latitude", LocationUpdateInput{VehicleNumber: "B1", RouteID: 1, Longitude: floatPtr(-80.4)}},
		{"latitude out of range", LocationUpdateInput{VehicleNumber: "B1", RouteID: 1, Latitude: floatPtr(91), Longitude: floatPtr(-80.4)}},
		{"latitude NaN", LocationUpdateInput{VehicleNumber: "B1", RouteID: 1, Latitude: floatPtr(math.NaN()), Longitude: floatPtr(-80.4)}},
		{"missing longitude", LocationUpdateInput{VehicleNumber: "B1", RouteID: 1, Latitude: floatPtr(43.4)}},
		{"longitude out of range", LocationUpdateInput{VehicleNumber: "B1", RouteID: 1, Latitude: floatPtr(43.4), Longitude: floatPtr(-181)}},
		{"heading out of range", LocationUpdateInput{VehicleNumber: "B1", RouteID: 1, Latitude: floatPtr(43.4), Longitude: floatPtr(-80.4), Heading: floatPtr(361)}},
		{"speed negative", LocationUpdateInput{VehicleNumber: "B1", RouteID: 1, Latitude: floatPtr(43.4), Longitude: floatPtr(-80.4), SpeedKmh: floatPtr(-1)}},
		{"speed too high", LocationUpdateInput{VehicleNumber: "B1", RouteID: 1, Latitude: floatPtr(43.4), Longitude: floatPtr(-80.4), SpeedKmh: floatPtr(250)}},
		{"invalid status", LocationUpdateInput{VehicleNumber: "B1", RouteID: 1, Latitude: floatPtr(43.4), Longitude: floatPtr(-80.4), Status: "broken"}},
		{"future timestamp", LocationUpdateInput{VehicleNumber: "B1", RouteID: 1, Latitude: floatPtr(43.4), Longitude: floatPtr(-80.4), RecordedAt: &future}},
		{"source_service NUL byte", LocationUpdateInput{VehicleNumber: "B1", RouteID: 1, Latitude: floatPtr(43.4), Longitude: floatPtr(-80.4), SourceService: "bad\x00src"}},
	}

	for _, tc := range badCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeVehicleRepo{}
			svc := NewServiceWithClock(repo, fixedClock)
			_, err := svc.SubmitLocation(context.Background(), tc.input, "api")
			var invalid *InvalidInput
			if !errors.As(err, &invalid) {
				t.Fatalf("expected *InvalidInput for %s, got %v", tc.name, err)
			}
		})
	}
}

func TestListActiveVehiclesValidationAndLookups(t *testing.T) {
	now := time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)
	fixedClock := func() time.Time { return now }

	t.Run("missing route returns ErrNotFound", func(t *testing.T) {
		repo := &fakeVehicleRepo{routeExistsErr: ErrNotFound}
		svc := NewServiceWithClock(repo, fixedClock)
		_, err := svc.ListActiveVehicles(context.Background(), ActiveVehiclesQuery{RouteID: 999})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("missing trip returns ErrNotFound", func(t *testing.T) {
		repo := &fakeVehicleRepo{tripExistsErr: ErrNotFound}
		svc := NewServiceWithClock(repo, fixedClock)
		_, err := svc.ListActiveVehicles(context.Background(), ActiveVehiclesQuery{TripID: 888})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("applies default freshness cutoff and non-nil slice", func(t *testing.T) {
		repo := &fakeVehicleRepo{}
		svc := NewServiceWithClock(repo, fixedClock)
		list, err := svc.ListActiveVehicles(context.Background(), ActiveVehiclesQuery{RouteNumber: " 7 "})
		if err != nil {
			t.Fatal(err)
		}
		if list == nil || len(list) != 0 {
			t.Fatalf("expected non-nil empty slice, got %#v", list)
		}
		if !repo.listCutoff.Equal(now.Add(-DefaultActiveMaxAge)) || repo.listRouteNum != "7" {
			t.Fatalf("unexpected query params: cutoff=%v routeNum=%q", repo.listCutoff, repo.listRouteNum)
		}
	})

	t.Run("rejects invalid max_age and negative IDs", func(t *testing.T) {
		svc := NewServiceWithClock(&fakeVehicleRepo{}, fixedClock)
		for _, q := range []ActiveVehiclesQuery{
			{RouteID: -1},
			{TripID: -2},
			{MaxAge: 5 * time.Second},
			{MaxAge: 25 * time.Hour},
		} {
			_, err := svc.ListActiveVehicles(context.Background(), q)
			var invalid *InvalidInput
			if !errors.As(err, &invalid) {
				t.Fatalf("query %+v: got %v, want *InvalidInput", q, err)
			}
		}
	})
}
