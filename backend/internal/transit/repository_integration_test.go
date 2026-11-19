package transit

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Optional real-PostgreSQL regression test. All fixtures are temporary tables in
// a rolled-back transaction; existing application tables are never modified.
func TestPostgresSchedules(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration checks")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	fixture, err := os.ReadFile("testdata/schedules.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(string(fixture)).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewPostgresRepository(tx)
	ctx := context.Background()
	for _, tc := range []struct {
		date string
		ids  []int
	}{
		{"2026-11-13", []int{12, 10, 15}}, {"2026-11-14", []int{11, 12, 15}},
	} {
		trips, err := repo.Timetable(ctx, 7, tc.date)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]int, 0, len(trips))
		for _, trip := range trips {
			ids = append(ids, trip.ID)
		}
		if !reflect.DeepEqual(ids, tc.ids) {
			t.Fatalf("%s: trips=%v want=%v", tc.date, ids, tc.ids)
		}
		if trips[len(trips)-1].StopTimes == nil {
			t.Fatal("empty stop times must be []")
		}
		if tc.date == "2026-11-13" {
			loop := trips[1].StopTimes
			if len(loop) != 2 || loop[0].StopID != loop[1].StopID || loop[0].RouteStopID == loop[1].RouteStopID {
				t.Fatalf("loop lost: %+v", loop)
			}
			if loop[0].ScheduledDeparture.UTC().Format(time.RFC3339) != "2026-11-14T06:10:00Z" {
				t.Fatal(loop[0])
			}
		}
	}
	parse := func(value string) time.Time {
		t.Helper()
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	q := DepartureQuery{StopID: 1, From: parse("2026-11-14T06:00:00Z"), Until: parse("2026-11-14T07:00:00Z"), Limit: 100}
	departures, err := repo.Departures(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(departures) != 3 || departures[0].TripID != 10 || departures[0].ServiceDate != "2026-11-13" || departures[1].TripID != 11 || departures[2].TripID != 12 {
		t.Fatalf("incorrect upcoming departures: %+v", departures)
	}
	for _, tc := range []struct {
		name  string
		query DepartureQuery
		ids   []int
	}{
		{"limit", DepartureQuery{StopID: 1, From: q.From, Until: q.Until, Limit: 1}, []int{10}},
		{"inactive stop", DepartureQuery{StopID: 2, From: q.From, Until: q.Until, Limit: 100}, []int{}},
		{"route filter", DepartureQuery{RouteID: 8, From: q.From, Until: q.Until, Limit: 100}, []int{}},
		{"trip filter", DepartureQuery{TripID: 11, From: q.From, Until: q.Until, Limit: 100}, []int{11}},
		{"intersection", DepartureQuery{StopID: 2, TripID: 11, From: q.From, Until: q.Until, Limit: 100}, []int{}},
		{"window boundaries", DepartureQuery{StopID: 1, From: parse("2026-11-14T06:10:00Z"), Until: parse("2026-11-14T06:30:00Z"), Limit: 100}, []int{10, 11}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results, err := repo.Departures(ctx, tc.query)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]int, 0, len(results))
			for _, result := range results {
				ids = append(ids, result.TripID)
			}
			if !reflect.DeepEqual(ids, tc.ids) {
				t.Fatalf("got=%v want=%v", ids, tc.ids)
			}
		})
	}
	for _, tc := range []struct{ from, until, expected string }{
		{"2026-11-01T06:00:00Z", "2026-11-01T07:00:00Z", "2026-11-01T06:30:00Z"},
		{"2026-03-08T06:00:00Z", "2026-03-08T07:00:00Z", "2026-03-08T06:30:00Z"},
	} {
		q.TripID, q.From, q.Until = 12, parse(tc.from), parse(tc.until)
		results, err := repo.Departures(ctx, q)
		if err != nil || len(results) != 1 {
			t.Fatalf("DST results=%+v err=%v", results, err)
		}
		if !results[0].ScheduledDeparture.Equal(parse(tc.expected)) {
			t.Fatal(results[0])
		}
	}

	stops, err := repo.SearchStops(ctx, StopSearchQuery{Query: "hub", Limit: 10})
	if err != nil || len(stops) != 1 || stops[0].ID != 3 {
		t.Fatalf("SearchStops got=%+v err=%v", stops, err)
	}
	directPlan, err := repo.Journeys(ctx, JourneyRepoQuery{
		OriginStopIDs:      []int{5},
		DestinationStopIDs: []int{3},
		From:               parse("2026-11-14T12:30:00Z"),
		Until:              parse("2026-11-14T15:00:00Z"),
		Limit:              5,
		MaxTransfers:       DefaultMaxTransfers,
		MaxWalkMeters:      DefaultMaxWalkMeters,
	})
	if err != nil || len(directPlan.Itineraries) != 1 || directPlan.Itineraries[0].Transfers != 0 || directPlan.Itineraries[0].DurationMinutes != 15 {
		t.Fatalf("direct Journeys got=%+v err=%v", directPlan, err)
	}
	transferPlan, err := repo.Journeys(ctx, JourneyRepoQuery{
		OriginStopIDs:      []int{5},
		DestinationStopIDs: []int{4},
		From:               parse("2026-11-14T12:30:00Z"),
		Until:              parse("2026-11-14T15:00:00Z"),
		Limit:              5,
		MaxTransfers:       DefaultMaxTransfers,
		MaxWalkMeters:      DefaultMaxWalkMeters,
	})
	if err != nil || len(transferPlan.Itineraries) != 1 || transferPlan.Itineraries[0].Transfers != 1 || len(transferPlan.Itineraries[0].Legs) != 2 || transferPlan.Itineraries[0].DurationMinutes != 40 {
		t.Fatalf("transfer Journeys got=%+v err=%v", transferPlan, err)
	}
	twoTransferPlan, err := repo.Journeys(ctx, JourneyRepoQuery{
		OriginStopIDs:      []int{5},
		DestinationStopIDs: []int{7},
		From:               parse("2026-11-14T12:30:00Z"),
		Until:              parse("2026-11-14T15:00:00Z"),
		Limit:              5,
		MaxTransfers:       DefaultMaxTransfers,
		MaxWalkMeters:      DefaultMaxWalkMeters,
	})
	if err != nil || len(twoTransferPlan.Itineraries) != 1 || twoTransferPlan.Itineraries[0].Transfers != 2 || len(twoTransferPlan.Itineraries[0].Legs) != 3 || twoTransferPlan.Itineraries[0].DurationMinutes != 60 {
		t.Fatalf("two-transfer Journeys got=%+v err=%v", twoTransferPlan, err)
	}
	egressWalkPlan, err := repo.Journeys(ctx, JourneyRepoQuery{
		OriginStopIDs:      []int{5},
		DestinationStopIDs: []int{6},
		From:               parse("2026-11-14T12:30:00Z"),
		Until:              parse("2026-11-14T15:00:00Z"),
		Limit:              5,
		MaxTransfers:       DefaultMaxTransfers,
		MaxWalkMeters:      DefaultMaxWalkMeters,
	})
	if err != nil || len(egressWalkPlan.Itineraries) != 1 || egressWalkPlan.Itineraries[0].Transfers != 1 || len(egressWalkPlan.Itineraries[0].Legs) != 3 || egressWalkPlan.Itineraries[0].Legs[2].Mode != "walk" {
		t.Fatalf("egress-walk Journeys got=%+v err=%v", egressWalkPlan, err)
	}

	if err := tx.Exec(`INSERT INTO disruptions VALUES (1, 'cancellation', 'Route 9 cancelled', NULL, '2026-11-14T12:00:00Z', '2026-11-14T15:00:00Z', 'active', 'ops'); INSERT INTO disruption_routes VALUES (1, 1, 9, NULL);`).Error; err != nil {
		t.Fatal(err)
	}
	disruptedPlan, err := repo.Journeys(ctx, JourneyRepoQuery{
		OriginStopIDs:      []int{5},
		DestinationStopIDs: []int{3},
		From:               parse("2026-11-14T12:30:00Z"),
		Until:              parse("2026-11-14T15:00:00Z"),
		Limit:              5,
		MaxTransfers:       DefaultMaxTransfers,
		MaxWalkMeters:      DefaultMaxWalkMeters,
	})
	if err != nil || len(disruptedPlan.Itineraries) != 0 || len(disruptedPlan.ExcludedDisruptions) != 1 {
		t.Fatalf("expected disrupted route 9 excluded, got=%+v err=%v", disruptedPlan, err)
	}
}
