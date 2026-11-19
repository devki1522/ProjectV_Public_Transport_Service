package vehicles

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/transit"
)

func testWaterlooTripStops(tripID, routeID int, baseArrivalSec int64) []TripStopPoint {
	return []TripStopPoint{
		{
			TripID:       tripID,
			RouteID:      routeID,
			RouteNumber:  "7",
			RouteName:    "King",
			Direction:    "direction_0",
			RouteStopID:  101,
			StopID:       10,
			StopName:     "Charles Terminal",
			StopSequence: 1,
			Latitude:     43.4500,
			Longitude:    -80.4900,
			ArrivalSec:   baseArrivalSec,
			DepartureSec: baseArrivalSec,
		},
		{
			TripID:       tripID,
			RouteID:      routeID,
			RouteNumber:  "7",
			RouteName:    "King",
			Direction:    "direction_0",
			RouteStopID:  102,
			StopID:       20,
			StopName:     "Central Station",
			StopSequence: 2,
			Latitude:     43.4600,
			Longitude:    -80.4900,
			ArrivalSec:   baseArrivalSec + 300,
			DepartureSec: baseArrivalSec + 300,
		},
		{
			TripID:       tripID,
			RouteID:      routeID,
			RouteNumber:  "7",
			RouteName:    "King",
			Direction:    "direction_0",
			RouteStopID:  103,
			StopID:       30,
			StopName:     "University of Waterloo",
			StopSequence: 3,
			Latitude:     43.4700,
			Longitude:    -80.4900,
			ArrivalSec:   baseArrivalSec + 600,
			DepartureSec: baseArrivalSec + 600,
		},
	}
}

func TestUpcomingArrivalsValidationAndEmptyCases(t *testing.T) {
	now := time.Date(2026, 11, 18, 19, 0, 0, 0, time.UTC) // 14:00 America/Toronto
	fixedClock := func() time.Time { return now }

	t.Run("rejects missing or invalid IDs", func(t *testing.T) {
		svc := NewServiceWithClock(&fakeVehicleRepo{}, fixedClock)
		for _, tc := range []struct {
			tripID, stopID int
		}{
			{0, 0},
			{-1, 0},
			{0, -5},
		} {
			_, err := svc.UpcomingArrivals(context.Background(), tc.tripID, tc.stopID)
			var invalid *InvalidInput
			if !errors.As(err, &invalid) {
				t.Fatalf("(%d,%d): got %v, want *InvalidInput", tc.tripID, tc.stopID, err)
			}
		}
		if _, err := svc.StopArrivals(context.Background(), 0, 7); err == nil {
			t.Fatal("expected error for stopID=0")
		}
		if _, err := svc.StopArrivals(context.Background(), 10, -1); err == nil {
			t.Fatal("expected error for negative routeID")
		}
	})

	t.Run("returns ErrNotFound for unknown trip, stop, or route", func(t *testing.T) {
		svcMissingTrip := NewServiceWithClock(&fakeVehicleRepo{tripExistsErr: ErrNotFound}, fixedClock)
		if _, err := svcMissingTrip.UpcomingArrivals(context.Background(), 999, 0); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}

		svcMissingStop := NewServiceWithClock(&fakeVehicleRepo{stopExistsErr: ErrNotFound}, fixedClock)
		if _, err := svcMissingStop.UpcomingArrivals(context.Background(), 0, 999); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}

		svcMissingRoute := NewServiceWithClock(&fakeVehicleRepo{routeExistsErr: ErrNotFound}, fixedClock)
		if _, err := svcMissingRoute.StopArrivals(context.Background(), 10, 999); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("returns non-nil empty slice when no fresh active vehicles exist", func(t *testing.T) {
		staleTime := now.Add(-DefaultActiveMaxAge - time.Minute)
		repo := &fakeVehicleRepo{
			listResult: []ActiveVehicle{
				{
					VehicleID:  1,
					RouteID:    7,
					TripID:     intPtr(300),
					Latitude:   43.4550,
					Longitude:  -80.4900,
					Status:     StatusInService,
					RecordedAt: staleTime,
				},
				{
					VehicleID:  2,
					RouteID:    7,
					TripID:     intPtr(300),
					Latitude:   43.4550,
					Longitude:  -80.4900,
					Status:     StatusOutOfService,
					RecordedAt: now,
				},
			},
			tripStops: testWaterlooTripStops(300, 7, 14*3600),
		}
		svc := NewServiceWithClock(repo, fixedClock)
		got, err := svc.UpcomingArrivals(context.Background(), 300, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || len(got) != 0 {
			t.Fatalf("expected non-nil empty slice, got %#v", got)
		}
	})
}

func TestUpcomingArrivalsProjectionSpeedAndDelay(t *testing.T) {
	// 2026-11-18 19:00:00Z = 14:00:00 EST (America/Toronto)
	now := time.Date(2026, 11, 18, 19, 0, 0, 0, time.UTC)
	fixedClock := func() time.Time { return now }
	tripStops := testWaterlooTripStops(300, 7, 14*3600) // 14:00, 14:05, 14:10 EST

	t.Run("projects mid-trip vehicle onto upcoming stops only and preserves scheduled times", func(t *testing.T) {
		speed := 36.0 // 10 m/s
		repo := &fakeVehicleRepo{
			listResult: []ActiveVehicle{
				{
					VehicleID:     11,
					VehicleNumber: "GRT-101",
					RouteID:       7,
					RouteNumber:   "7",
					RouteName:     "King",
					Direction:     "direction_0",
					TripID:        intPtr(300),
					Latitude:      43.4550, // halfway between stop 1 (43.45) and stop 2 (43.46)
					Longitude:     -80.4900,
					SpeedKmh:      &speed,
					Status:        StatusInService,
					IsSimulated:   false,
					RecordedAt:    now,
				},
			},
			tripStops: tripStops,
		}
		svc := NewServiceWithClock(repo, fixedClock)

		arrivals, err := svc.UpcomingArrivals(context.Background(), 300, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(arrivals) != 2 {
			t.Fatalf("expected 2 upcoming stops (stop 1 passed), got %d: %+v", len(arrivals), arrivals)
		}
		if arrivals[0].StopID != 20 || arrivals[0].StopSequence != 2 {
			t.Fatalf("expected first upcoming stop to be Central Station (seq 2), got %+v", arrivals[0])
		}
		if arrivals[1].StopID != 30 || arrivals[1].StopSequence != 3 {
			t.Fatalf("expected second upcoming stop to be UW (seq 3), got %+v", arrivals[1])
		}
		if arrivals[0].DistanceMeters <= 0 || arrivals[1].DistanceMeters <= arrivals[0].DistanceMeters {
			t.Fatalf("expected strictly increasing distance_meters, got %d then %d", arrivals[0].DistanceMeters, arrivals[1].DistanceMeters)
		}
		if !arrivals[1].EstimatedArrivalTime.After(arrivals[0].EstimatedArrivalTime) {
			t.Fatalf("expected later ETA for stop 3 than stop 2: %v vs %v", arrivals[0].EstimatedArrivalTime, arrivals[1].EstimatedArrivalTime)
		}
		if arrivals[0].ScheduledArrivalTime == nil || !arrivals[0].ScheduledArrivalTime.Equal(now.Add(5*time.Minute)) {
			t.Fatalf("unexpected scheduled_arrival_time for stop 2: %v", arrivals[0].ScheduledArrivalTime)
		}
		expectedDelay := int(math.Round(arrivals[0].EstimatedArrivalTime.Sub(*arrivals[0].ScheduledArrivalTime).Minutes()))
		if arrivals[0].DelayMinutes != expectedDelay {
			t.Fatalf("delay_minutes = %d, want %d", arrivals[0].DelayMinutes, expectedDelay)
		}

		// Verify all required JSON field names are emitted.
		raw, err := json.Marshal(arrivals[0])
		if err != nil {
			t.Fatal(err)
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{
			"trip_id", "route_id", "route_number", "route_name", "direction",
			"stop_id", "stop_name", "stop_sequence", "vehicle_id", "vehicle_number",
			"vehicle_status", "scheduled_arrival_time", "estimated_arrival_time",
			"delay_minutes", "distance_meters", "is_simulated", "recorded_at",
		} {
			if _, ok := obj[field]; !ok {
				t.Errorf("missing JSON field %q in %s", field, string(raw))
			}
		}
	})

	t.Run("clamps speed to [15, 65] km/h, defaults to 28 km/h, and applies delayed factor", func(t *testing.T) {
		if got := effectiveSpeedKmh(nil); got != 28.0 {
			t.Fatalf("nil speed = %v, want 28", got)
		}
		if got := effectiveSpeedKmh(floatPtr(0)); got != 28.0 {
			t.Fatalf("zero speed = %v, want 28", got)
		}
		if got := effectiveSpeedKmh(floatPtr(5)); got != 15.0 {
			t.Fatalf("low speed = %v, want 15", got)
		}
		if got := effectiveSpeedKmh(floatPtr(120)); got != 65.0 {
			t.Fatalf("high speed = %v, want 65", got)
		}
		if got := effectiveSpeedKmh(floatPtr(36)); got != 36.0 {
			t.Fatalf("mid speed = %v, want 36", got)
		}

		baseVeh := ActiveVehicle{
			VehicleID:  11,
			SpeedKmh:   floatPtr(36.0), // 10 m/s -> 1000m = 100s
			Status:     StatusInService,
			RecordedAt: now,
		}
		normalETA := estimateArrivalTime(baseVeh, 1000.0, now)
		if !normalETA.Equal(now.Add(100 * time.Second)) {
			t.Fatalf("normal ETA = %v, want +100s", normalETA.Sub(now))
		}

		delayedVeh := baseVeh
		delayedVeh.Status = StatusDelayed
		delayedETA := estimateArrivalTime(delayedVeh, 1000.0, now)
		if !delayedETA.Equal(now.Add(125 * time.Second)) {
			t.Fatalf("delayed ETA = %v, want +125s", delayedETA.Sub(now))
		}

		layoverVeh := baseVeh
		layoverVeh.Status = StatusLayover
		layoverETA := estimateArrivalTime(layoverVeh, 1000.0, now)
		if !layoverETA.Equal(now.Add(100*time.Second + layoverDwellDuration)) {
			t.Fatalf("layover ETA = %v, want +%v", layoverETA.Sub(now), 100*time.Second+layoverDwellDuration)
		}
	})

	t.Run("includes all stops when approaching first stop and returns empty when well past terminus", func(t *testing.T) {
		repoBefore := &fakeVehicleRepo{
			listResult: []ActiveVehicle{{
				VehicleID:  12,
				RouteID:    7,
				TripID:     intPtr(300),
				Latitude:   43.4450, // before stop 1 (43.45)
				Longitude:  -80.4900,
				Status:     StatusInService,
				RecordedAt: now,
			}},
			tripStops: tripStops,
		}
		arrivalsBefore, err := NewServiceWithClock(repoBefore, fixedClock).UpcomingArrivals(context.Background(), 300, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(arrivalsBefore) != 3 || arrivalsBefore[0].StopID != 10 {
			t.Fatalf("expected all 3 upcoming stops, got %+v", arrivalsBefore)
		}

		repoPast := &fakeVehicleRepo{
			listResult: []ActiveVehicle{{
				VehicleID:  12,
				RouteID:    7,
				TripID:     intPtr(300),
				Latitude:   43.4900, // ~2.2km past final stop (43.47)
				Longitude:  -80.4900,
				Status:     StatusInService,
				RecordedAt: now,
			}},
			tripStops: tripStops,
		}
		arrivalsPast, err := NewServiceWithClock(repoPast, fixedClock).UpcomingArrivals(context.Background(), 300, 0)
		if err != nil {
			t.Fatal(err)
		}
		if arrivalsPast == nil || len(arrivalsPast) != 0 {
			t.Fatalf("expected empty non-nil slice past terminus, got %#v", arrivalsPast)
		}
	})

	t.Run("resolves after-midnight 25:10:00 interval on preceding service day", func(t *testing.T) {
		// 2026-11-19 06:00:00Z = 2026-11-19 01:00:00 EST
		lateNight := time.Date(2026, 11, 19, 6, 0, 0, 0, time.UTC)
		overnightStops := testWaterlooTripStops(301, 7, 25*3600+5*60) // 25:05, 25:10, 25:15 -> 01:05, 01:10, 01:15 EST
		repo := &fakeVehicleRepo{
			listResult: []ActiveVehicle{{
				VehicleID:  15,
				RouteID:    7,
				TripID:     intPtr(301),
				Latitude:   43.4550,
				Longitude:  -80.4900,
				Status:     StatusInService,
				RecordedAt: lateNight,
			}},
			tripStops: overnightStops,
		}
		arrivals, err := NewServiceWithClock(repo, func() time.Time { return lateNight }).UpcomingArrivals(context.Background(), 301, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(arrivals) != 2 || arrivals[0].ScheduledArrivalTime == nil {
			t.Fatalf("unexpected overnight arrivals: %+v", arrivals)
		}
		wantSched := time.Date(2026, 11, 19, 6, 10, 0, 0, time.UTC) // 01:10 EST
		if !arrivals[0].ScheduledArrivalTime.Equal(wantSched) {
			t.Fatalf("overnight scheduled_arrival_time = %v, want %v", arrivals[0].ScheduledArrivalTime, wantSched)
		}
	})
}

func TestStopArrivalsMultipleVehiclesAndRouteFallback(t *testing.T) {
	now := time.Date(2026, 11, 18, 19, 0, 0, 0, time.UTC)
	fixedClock := func() time.Time { return now }

	repo := &fakeVehicleRepo{
		listResult: []ActiveVehicle{
			// Bus with trip_id approaching stop 20 (further away)
			{
				VehicleID:     1,
				VehicleNumber: "GRT-101",
				RouteID:       7,
				RouteNumber:   "7",
				RouteName:     "King",
				Direction:     "direction_0",
				TripID:        intPtr(300),
				Latitude:      43.4520,
				Longitude:     -80.4900,
				SpeedKmh:      floatPtr(30),
				Status:        StatusInService,
				RecordedAt:    now,
			},
			// Route-only bus (TripID == nil) closer to stop 20
			{
				VehicleID:     2,
				VehicleNumber: "GRT-202",
				RouteID:       8,
				RouteNumber:   "8",
				RouteName:     "Weber",
				Direction:     "direction_0",
				TripID:        nil,
				Latitude:      43.4580,
				Longitude:     -80.4900,
				SpeedKmh:      floatPtr(36),
				Status:        StatusInService,
				RecordedAt:    now,
			},
			// Bus on route 7 that already passed stop 20 (between stop 20 and stop 30)
			{
				VehicleID:     3,
				VehicleNumber: "GRT-103",
				RouteID:       7,
				RouteNumber:   "7",
				RouteName:     "King",
				Direction:     "direction_0",
				TripID:        intPtr(300),
				Latitude:      43.4650,
				Longitude:     -80.4900,
				SpeedKmh:      floatPtr(30),
				Status:        StatusInService,
				RecordedAt:    now,
			},
		},
		tripStops: testWaterlooTripStops(300, 7, 14*3600),
		routeStops: []RouteStopPoint{
			{RouteID: 8, RouteNumber: "8", RouteName: "Weber", Direction: "direction_0", RouteStopID: 201, StopID: 15, StopName: "Start", StopSequence: 1, Latitude: 43.4500, Longitude: -80.4900},
			{RouteID: 8, RouteNumber: "8", RouteName: "Weber", Direction: "direction_0", RouteStopID: 202, StopID: 20, StopName: "Central Station", StopSequence: 2, Latitude: 43.4600, Longitude: -80.4900},
		},
	}

	svc := NewServiceWithClock(repo, fixedClock)
	arrivals, err := svc.StopArrivals(context.Background(), 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(arrivals) != 2 {
		t.Fatalf("expected 2 approaching vehicles (passed bus excluded), got %d: %+v", len(arrivals), arrivals)
	}
	// Sorted earliest ETA first: GRT-202 is closer & faster than GRT-101.
	if arrivals[0].VehicleNumber != "GRT-202" || arrivals[0].TripID != 0 || arrivals[0].ScheduledArrivalTime != nil {
		t.Fatalf("expected closer route-only bus first, got %+v", arrivals[0])
	}
	if arrivals[1].VehicleNumber != "GRT-101" || arrivals[1].TripID != 300 || arrivals[1].ScheduledArrivalTime == nil {
		t.Fatalf("expected trip-matched bus second, got %+v", arrivals[1])
	}
	if !arrivals[0].EstimatedArrivalTime.Before(arrivals[1].EstimatedArrivalTime) {
		t.Fatalf("expected arrivals sorted by estimated_arrival_time: %v vs %v", arrivals[0].EstimatedArrivalTime, arrivals[1].EstimatedArrivalTime)
	}
}

func TestPostgresRepositoryUpcomingArrivalsSQLMock(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("loads trip arrivals via PostgreSQL queries", func(t *testing.T) {
		repo, mock := mockVehicleRepository(t)

		mock.ExpectQuery(`SELECT "id" FROM "trips"`).
			WithArgs(300, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(300))

		mock.ExpectQuery(`SELECT vl.vehicle_id`).
			WithArgs(sqlmock.AnyArg(), 0, 0, "", "", 300, 300).
			WillReturnRows(sqlmock.NewRows([]string{
				"vehicle_id", "vehicle_number", "is_accessible",
				"route_id", "route_number", "route_name", "direction",
				"trip_id", "latitude", "longitude", "heading", "speed_kmh",
				"status", "source_service", "is_simulated", "recorded_at", "received_at",
			}).AddRow(11, "SIM-001", true, 7, "7", "King", "direction_0", 300, 43.4550, -80.4900, 0.0, 36.0, "in_service", "simulator", true, now, now))

		mock.ExpectQuery(`SELECT t.id AS trip_id`).
			WithArgs(300).
			WillReturnRows(sqlmock.NewRows([]string{
				"trip_id", "route_id", "route_number", "route_name", "direction",
				"route_stop_id", "stop_id", "stop_name", "stop_sequence",
				"latitude", "longitude", "arrival_sec", "departure_sec",
			}).
				AddRow(300, 7, "7", "King", "direction_0", 101, 10, "Charles Terminal", 1, 43.4500, -80.4900, int64(50400), int64(50400)).
				AddRow(300, 7, "7", "King", "direction_0", 102, 20, "Central Station", 2, 43.4600, -80.4900, int64(50700), int64(50700)))

		arrivals, err := repo.UpcomingArrivals(context.Background(), 300, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(arrivals) != 1 || arrivals[0].StopID != 20 || arrivals[0].VehicleNumber != "SIM-001" || !arrivals[0].IsSimulated {
			t.Fatalf("unexpected repo arrivals: %+v", arrivals)
		}
	})

	t.Run("returns ErrNotFound for unknown trip in PostgreSQL", func(t *testing.T) {
		repo, mock := mockVehicleRepository(t)

		mock.ExpectQuery(`SELECT "id" FROM "trips"`).
			WithArgs(999, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		_, err := repo.UpcomingArrivals(context.Background(), 999, 0)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("loads stop arrivals via ListActiveVehiclesForStop", func(t *testing.T) {
		repo, mock := mockVehicleRepository(t)

		mock.ExpectQuery(`SELECT "id" FROM "stops"`).
			WithArgs(20, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(20))

		mock.ExpectQuery(`SELECT vl.vehicle_id`).
			WithArgs(sqlmock.AnyArg(), 0, 0, 20).
			WillReturnRows(sqlmock.NewRows([]string{
				"vehicle_id", "vehicle_number", "is_accessible",
				"route_id", "route_number", "route_name", "direction",
				"trip_id", "latitude", "longitude", "heading", "speed_kmh",
				"status", "source_service", "is_simulated", "recorded_at", "received_at",
			}).AddRow(11, "SIM-001", true, 7, "7", "King", "direction_0", 300, 43.4550, -80.4900, 0.0, 36.0, "delayed", "simulator", true, now, now))

		mock.ExpectQuery(`SELECT t.id AS trip_id`).
			WithArgs(300).
			WillReturnRows(sqlmock.NewRows([]string{
				"trip_id", "route_id", "route_number", "route_name", "direction",
				"route_stop_id", "stop_id", "stop_name", "stop_sequence",
				"latitude", "longitude", "arrival_sec", "departure_sec",
			}).
				AddRow(300, 7, "7", "King", "direction_0", 101, 10, "Charles Terminal", 1, 43.4500, -80.4900, int64(50400), int64(50400)).
				AddRow(300, 7, "7", "King", "direction_0", 102, 20, "Central Station", 2, 43.4600, -80.4900, int64(50700), int64(50700)).
				AddRow(300, 7, "7", "King", "direction_0", 103, 30, "UW", 3, 43.4700, -80.4900, int64(51000), int64(51000)))

		arrivals, err := repo.UpcomingArrivals(context.Background(), 0, 20)
		if err != nil {
			t.Fatal(err)
		}
		if len(arrivals) != 1 || arrivals[0].StopID != 20 || arrivals[0].VehicleStatus != StatusDelayed {
			t.Fatalf("unexpected stop arrivals: %+v", arrivals)
		}
	})
}

func TestLiveEstimateProvider(t *testing.T) {
	now := time.Date(2026, 11, 18, 19, 0, 0, 0, time.UTC)
	fixedClock := func() time.Time { return now }

	t.Run("nil db returns empty map without error", func(t *testing.T) {
		provider := NewLiveEstimateProvider(nil)
		got, err := provider.Estimates(context.Background(), []transit.Departure{
			{TripID: 300, Route: transit.Route{ID: 7}, RouteStopID: 102, StopID: 20, ServiceDate: "2026-11-18", ScheduledDeparture: now.Add(5 * time.Minute)},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("expected empty estimates map for nil db, got %+v", got)
		}
	})

	t.Run("never fabricates estimates when no active vehicles exist", func(t *testing.T) {
		provider := NewLiveEstimateProviderWithRepository(&fakeVehicleRepo{}, fixedClock)
		got, err := provider.Estimates(context.Background(), []transit.Departure{
			{TripID: 300, Route: transit.Route{ID: 7}, RouteStopID: 102, StopID: 20, ServiceDate: "2026-11-18", ScheduledDeparture: now.Add(5 * time.Minute)},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("expected empty map with no active vehicles, got %+v", got)
		}
	})

	t.Run("matches exact trip_id and pairs route-level simulated vehicle without fabricating for later trips", func(t *testing.T) {
		repo := &fakeVehicleRepo{
			listResult: []ActiveVehicle{
				// Exact trip match for Trip 300 on Route 7
				{
					VehicleID:   11,
					RouteID:     7,
					TripID:      intPtr(300),
					Latitude:    43.4550,
					Longitude:   -80.4900,
					SpeedKmh:    floatPtr(36),
					Status:      StatusInService,
					RecordedAt:  now,
					IsSimulated: false,
				},
				// Simulated vehicle on Route 8 carrying SampleTripID 999 (different from current-hour Trip 800/801)
				{
					VehicleID:   22,
					RouteID:     8,
					TripID:      intPtr(999),
					Latitude:    43.4550,
					Longitude:   -80.4900,
					SpeedKmh:    floatPtr(36),
					Status:      StatusDelayed,
					RecordedAt:  now,
					IsSimulated: true,
				},
			},
			tripStops: append(
				testWaterlooTripStops(300, 7, 14*3600),
				TripStopPoint{TripID: 999, RouteID: 8, RouteStopID: 201, StopID: 10, StopName: "Start", StopSequence: 1, Latitude: 43.4500, Longitude: -80.4900, ArrivalSec: 21600, DepartureSec: 21600},
				TripStopPoint{TripID: 999, RouteID: 8, RouteStopID: 202, StopID: 20, StopName: "Mid", StopSequence: 2, Latitude: 43.4600, Longitude: -80.4900, ArrivalSec: 21900, DepartureSec: 21900},
				TripStopPoint{TripID: 999, RouteID: 8, RouteStopID: 203, StopID: 30, StopName: "End", StopSequence: 3, Latitude: 43.4700, Longitude: -80.4900, ArrivalSec: 22200, DepartureSec: 22200},
			),
		}

		deps := []transit.Departure{
			// Exact trip 300 at upcoming stop 20 -> SHOULD match vehicle 11
			{TripID: 300, Route: transit.Route{ID: 7}, RouteStopID: 102, StopID: 20, StopSequence: 2, ServiceDate: "2026-11-18", ScheduledDeparture: now.Add(3 * time.Minute)},
			// Later trip 301 on route 7 -> should stay NULL (vehicle 11 already consumed by trip 300)
			{TripID: 301, Route: transit.Route{ID: 7}, RouteStopID: 102, StopID: 20, StopSequence: 2, ServiceDate: "2026-11-18", ScheduledDeparture: now.Add(20 * time.Minute)},
			// Tomorrow occurrence of trip 300 -> outside live horizon, should stay NULL
			{TripID: 300, Route: transit.Route{ID: 7}, RouteStopID: 102, StopID: 20, StopSequence: 2, ServiceDate: "2026-11-19", ScheduledDeparture: now.Add(24 * time.Hour)},
			// Imminent journey leg on Route 8 (Trip 800, stops 20 -> 30) -> SHOULD match simulated vehicle 22 at BOTH stops
			{TripID: 800, Route: transit.Route{ID: 8}, RouteStopID: 202, StopID: 20, StopSequence: 2, ServiceDate: "2026-11-18", ScheduledDeparture: now.Add(2 * time.Minute)},
			{TripID: 800, Route: transit.Route{ID: 8}, RouteStopID: 203, StopID: 30, StopSequence: 3, ServiceDate: "2026-11-18", ScheduledDeparture: now.Add(6 * time.Minute)},
			// Later trip 801 on Route 8 -> should stay NULL (vehicle 22 already paired to Trip 800)
			{TripID: 801, Route: transit.Route{ID: 8}, RouteStopID: 202, StopID: 20, StopSequence: 2, ServiceDate: "2026-11-18", ScheduledDeparture: now.Add(30 * time.Minute)},
			// Route 9 with no active vehicles -> should stay NULL
			{TripID: 900, Route: transit.Route{ID: 9}, RouteStopID: 301, StopID: 20, StopSequence: 1, ServiceDate: "2026-11-18", ScheduledDeparture: now.Add(5 * time.Minute)},
		}

		provider := NewLiveEstimateProviderWithRepository(repo, fixedClock)
		estimates, err := provider.Estimates(context.Background(), deps)
		if err != nil {
			t.Fatal(err)
		}

		k300Today := transit.DepartureKey{TripID: 300, RouteStopID: 102, ServiceDate: "2026-11-18"}
		k301Today := transit.DepartureKey{TripID: 301, RouteStopID: 102, ServiceDate: "2026-11-18"}
		k300Tomorrow := transit.DepartureKey{TripID: 300, RouteStopID: 102, ServiceDate: "2026-11-19"}
		k800Board := transit.DepartureKey{TripID: 800, RouteStopID: 202, ServiceDate: "2026-11-18"}
		k800Alight := transit.DepartureKey{TripID: 800, RouteStopID: 203, ServiceDate: "2026-11-18"}
		k801Later := transit.DepartureKey{TripID: 801, RouteStopID: 202, ServiceDate: "2026-11-18"}
		k900NoBus := transit.DepartureKey{TripID: 900, RouteStopID: 301, ServiceDate: "2026-11-18"}

		if _, ok := estimates[k300Today]; !ok {
			t.Fatalf("expected live estimate for exact trip 300 today, got %+v", estimates)
		}
		if _, ok := estimates[k301Today]; ok {
			t.Fatalf("did not expect estimate for later trip 301 on route 7")
		}
		if _, ok := estimates[k300Tomorrow]; ok {
			t.Fatalf("did not expect live estimate for tomorrow occurrence of trip 300")
		}
		boardETA, okBoard := estimates[k800Board]
		alightETA, okAlight := estimates[k800Alight]
		if !okBoard || !okAlight || !alightETA.After(boardETA) {
			t.Fatalf("expected coherent boarding (%v,%v) and alighting (%v,%v) ETAs for trip 800", okBoard, boardETA, okAlight, alightETA)
		}
		if _, ok := estimates[k801Later]; ok {
			t.Fatalf("did not expect estimate for later trip 801 on route 8")
		}
		if _, ok := estimates[k900NoBus]; ok {
			t.Fatalf("did not expect estimate for route 9 with no active vehicles")
		}
	})

	t.Run("loads active vehicles once per call over PostgreSQL", func(t *testing.T) {
		repo, mock := mockVehicleRepository(t)
		liveNow := time.Now().UTC().Truncate(time.Second)

		mock.ExpectQuery(`SELECT vl.vehicle_id`).
			WithArgs(sqlmock.AnyArg(), 0, 0, "", "", 0, 0).
			WillReturnRows(sqlmock.NewRows([]string{
				"vehicle_id", "vehicle_number", "is_accessible",
				"route_id", "route_number", "route_name", "direction",
				"trip_id", "latitude", "longitude", "heading", "speed_kmh",
				"status", "source_service", "is_simulated", "recorded_at", "received_at",
			}).AddRow(11, "SIM-001", true, 7, "7", "King", "direction_0", 300, 43.4550, -80.4900, 0.0, 36.0, "in_service", "simulator", true, liveNow, liveNow))

		mock.ExpectQuery(`SELECT t.id AS trip_id`).
			WithArgs(300).
			WillReturnRows(sqlmock.NewRows([]string{
				"trip_id", "route_id", "route_number", "route_name", "direction",
				"route_stop_id", "stop_id", "stop_name", "stop_sequence",
				"latitude", "longitude", "arrival_sec", "departure_sec",
			}).
				AddRow(300, 7, "7", "King", "direction_0", 101, 10, "Start", 1, 43.4500, -80.4900, int64(50400), int64(50400)).
				AddRow(300, 7, "7", "King", "direction_0", 102, 20, "Mid", 2, 43.4600, -80.4900, int64(50700), int64(50700)))

		provider := NewLiveEstimateProvider(repo.db)
		estimates, err := provider.Estimates(context.Background(), []transit.Departure{
			{TripID: 300, Route: transit.Route{ID: 7}, RouteStopID: 102, StopID: 20, StopSequence: 2, ServiceDate: liveNow.Format(time.DateOnly), ScheduledDeparture: liveNow.Add(3 * time.Minute)},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(estimates) != 1 {
			t.Fatalf("expected 1 live estimate, got %+v", estimates)
		}
	})
}
