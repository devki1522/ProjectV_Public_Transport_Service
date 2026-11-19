package transit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func mockRepository(t *testing.T) (*PostgresRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		db.Close()
	})
	return NewPostgresRepository(gormDB), mock
}

// TestTimetableGroupsTripsAndRetainsRepeatedStops verifies timetable groups trips and retains repeated stops.
func TestTimetableGroupsTripsAndRetainsRepeatedStops(t *testing.T) {
	repo, mock := mockRepository(t)
	arrival := time.Date(2026, 11, 11, 5, 10, 0, 0, time.UTC)
	departure := arrival.Add(time.Minute)
	mock.ExpectQuery("SELECT t.id AS trip_id").WillReturnRows(sqlmock.NewRows([]string{
		"trip_id", "schedule_type", "route_stop_id", "stop_id", "stop_name", "stop_sequence", "scheduled_arrival", "scheduled_departure",
	}).AddRow(1, "weekday", 10, 5, "Station", 1, arrival, departure).
		AddRow(1, "weekday", 11, 5, "Station", 2, arrival.Add(time.Hour), departure.Add(time.Hour)).
		AddRow(2, "daily", nil, nil, nil, nil, nil, nil))
	trips, err := repo.Timetable(context.Background(), 7, "2026-11-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(trips) != 2 || len(trips[0].StopTimes) != 2 || trips[1].StopTimes == nil || len(trips[1].StopTimes) != 0 {
		t.Fatalf("%+v", trips)
	}
	first, second := trips[0].StopTimes[0], trips[0].StopTimes[1]
	if first.StopID != second.StopID || first.RouteStopID == second.RouteStopID || second.StopSequence != 2 || !first.ScheduledDeparture.Equal(departure) || !first.ScheduledArrival.Equal(arrival) {
		t.Fatalf("wrong stop occurrences: %+v", trips[0])
	}
}

// TestDepartureRowMappingAndBoundFilters verifies departure row mapping and bound filters.
func TestDepartureRowMappingAndBoundFilters(t *testing.T) {
	repo, mock := mockRepository(t)
	q := testQuery()
	q.RouteID = 7
	q.TripID = 10
	mock.ExpectQuery(`AND s.id = \$\d+ AND r.id = \$\d+ AND t.id = \$\d+`).WillReturnRows(sqlmock.NewRows([]string{
		"trip_id", "route_id", "number", "name", "direction", "is_active", "route_stop_id", "stop_id", "stop_name", "stop_sequence", "service_date", "scheduled_departure",
	}).AddRow(10, 7, "7", "Main", "direction_1", true, 20, 1, "Station", 3, "2026-11-09", q.From))
	result, err := repo.Departures(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatal(result)
	}
	d := result[0]
	if d.Route.ID != 7 || d.Route.Number != "7" || d.Route.Direction != "direction_1" || d.TripID != 10 || d.StopSequence != 3 || d.ServiceDate != "2026-11-09" || !d.ScheduledDeparture.Equal(q.From) || d.EstimatedDeparture != nil {
		t.Fatalf("incorrect response: %+v", d)
	}
}

// TestRepositoryMissingAndFailedReads verifies repository missing and failed reads.
func TestRepositoryMissingAndFailedReads(t *testing.T) {
	repo, mock := mockRepository(t)
	mock.ExpectQuery(`SELECT \* FROM "routes"`).WithArgs(7, 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if _, err := repo.Route(context.Background(), 7); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT \* FROM "stops"`).WithArgs(1, 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if _, err := repo.Stop(context.Background(), 1); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT "id" FROM "trips"`).WithArgs(10, 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if err := repo.TripExists(context.Background(), 10); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	failure := errors.New("database offline")
	mock.ExpectQuery("SELECT t.id AS trip_id").WillReturnError(failure)
	if _, err := repo.Timetable(context.Background(), 7, "2026-11-10"); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	mock.ExpectQuery("WITH departures AS").WillReturnError(failure)
	if _, err := repo.Departures(context.Background(), testQuery()); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}

// TestSearchStopsQueryAndError verifies active stop search query building and errors.
func TestSearchStopsQueryAndError(t *testing.T) {
	repo, mock := mockRepository(t)
	mock.ExpectQuery(`SELECT \* FROM "stops" WHERE is_active = \$1 AND LOWER\(name\) LIKE \$2 ORDER BY name ASC, id ASC LIMIT \$3`).
		WithArgs(true, "%central%", 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "latitude", "longitude", "wheelchair_boarding", "is_active"}).
			AddRow(21, "Central Station", 43.45, -80.49, 1, true))
	stops, err := repo.SearchStops(context.Background(), StopSearchQuery{Query: "Central", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(stops) != 1 || stops[0].ID != 21 || stops[0].Name != "Central Station" {
		t.Fatalf("unexpected stops: %+v", stops)
	}

	failure := errors.New("database offline")
	mock.ExpectQuery(`SELECT \* FROM "stops"`).WillReturnError(failure)
	if _, err := repo.SearchStops(context.Background(), StopSearchQuery{Limit: 10}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}

// TestJourneysDirectAndTransferMapping verifies network loading, multi-transfer planning, walking legs, and ordering.
func TestJourneysDirectAndTransferMapping(t *testing.T) {
	repo, mock := mockRepository(t)
	from := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC) // 07:00 EST
	q := JourneyRepoQuery{
		OriginStopIDs:      []int{21},
		DestinationStopIDs: []int{22},
		From:               from,
		Until:              from.Add(6 * time.Hour),
		Limit:              5,
		MaxTransfers:       DefaultMaxTransfers,
		MaxWalkMeters:      DefaultMaxWalkMeters,
	}

	mock.ExpectQuery("FROM stops").WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "latitude", "longitude", "wheelchair_boarding", "is_active",
	}).
		AddRow(21, "Central Station", 43.4500, -80.4900, 1, true).
		AddRow(30, "Transfer Hub", 43.4400, -80.4700, 1, true).
		AddRow(31, "Conestoga Hub", 43.4300, -80.4500, 1, true).
		AddRow(23, "Opposite Fairway", 43.4208, -80.4300, 1, true).
		AddRow(22, "Fairway Station", 43.4200, -80.4300, 1, true))

	mock.ExpectQuery("FROM routes").WillReturnRows(sqlmock.NewRows([]string{
		"id", "number", "name", "direction", "is_active",
	}).
		AddRow(7, "7", "King", "direction_0", true).
		AddRow(1, "1", "Queen-River", "direction_0", true).
		AddRow(201, "201", "iXpress", "direction_0", true).
		AddRow(61, "61", "Preston", "direction_0", true))

	mock.ExpectQuery("FROM stop_times st").WillReturnRows(sqlmock.NewRows([]string{
		"trip_id", "route_id", "schedule_type", "route_stop_id", "stop_id", "stop_sequence", "arrival_sec", "departure_sec",
	}).
		AddRow(100, 7, "daily", 701, 21, 1, sec(7, 15), sec(7, 15)).
		AddRow(100, 7, "daily", 702, 22, 2, sec(7, 45), sec(7, 45)).
		AddRow(101, 1, "daily", 101, 21, 1, sec(7, 5), sec(7, 5)).
		AddRow(101, 1, "daily", 102, 30, 2, sec(7, 15), sec(7, 15)).
		AddRow(202, 201, "daily", 201, 30, 1, sec(7, 18), sec(7, 18)).
		AddRow(202, 201, "daily", 202, 31, 2, sec(7, 28), sec(7, 28)).
		AddRow(303, 61, "daily", 301, 31, 1, sec(7, 31), sec(7, 31)).
		AddRow(303, 61, "daily", 302, 23, 2, sec(7, 38), sec(7, 38)))

	mock.ExpectQuery("FROM disruptions d").WillReturnRows(sqlmock.NewRows([]string{
		"id", "type", "title", "start_time", "end_time", "status", "route_id", "stop_id",
	}))

	plan, err := repo.Journeys(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	itineraries := plan.Itineraries
	if len(itineraries) != 2 {
		t.Fatalf("expected 2 non-dominated itineraries, got %d: %+v", len(itineraries), itineraries)
	}
	// Earlier arrival (2-transfer + egress walk arriving 12:40 vs direct arriving 12:45) comes first.
	if itineraries[0].Transfers != 2 || len(itineraries[0].Legs) != 4 || itineraries[0].Legs[3].Mode != "walk" || itineraries[0].WalkingMinutes != 2 {
		t.Fatalf("unexpected first itinerary: %+v", itineraries[0])
	}
	if itineraries[1].Transfers != 0 || len(itineraries[1].Legs) != 1 || itineraries[1].Legs[0].Mode != "transit" || itineraries[1].DurationMinutes != 30 {
		t.Fatalf("unexpected second itinerary: %+v", itineraries[1])
	}
	if len(itineraries[1].Legs[0].Stops) != 2 || itineraries[1].Legs[0].Stops[0].RouteStopID != 701 || itineraries[1].Legs[0].Stops[1].StopName != "Fairway Station" {
		t.Fatalf("expected ordered stops on leg, got %+v", itineraries[1].Legs[0].Stops)
	}

	// Verify active disruption returned by PostgreSQL excludes the disrupted route.
	mock.ExpectQuery("FROM stops").WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "latitude", "longitude", "wheelchair_boarding", "is_active",
	}).
		AddRow(21, "Central Station", 43.4500, -80.4900, 1, true).
		AddRow(22, "Fairway Station", 43.4200, -80.4300, 1, true))
	mock.ExpectQuery("FROM routes").WillReturnRows(sqlmock.NewRows([]string{
		"id", "number", "name", "direction", "is_active",
	}).AddRow(7, "7", "King", "direction_0", true))
	mock.ExpectQuery("FROM stop_times st").WillReturnRows(sqlmock.NewRows([]string{
		"trip_id", "route_id", "schedule_type", "route_stop_id", "stop_id", "stop_sequence", "arrival_sec", "departure_sec",
	}).
		AddRow(100, 7, "daily", 701, 21, 1, sec(7, 15), sec(7, 15)).
		AddRow(100, 7, "daily", 702, 22, 2, sec(7, 45), sec(7, 45)))
	mock.ExpectQuery("FROM disruptions d").WillReturnRows(sqlmock.NewRows([]string{
		"id", "type", "title", "start_time", "end_time", "status", "route_id", "stop_id",
	}).AddRow(9, "cancellation", "Route 7 cancelled", from, from.Add(2*time.Hour), "active", 7, nil))

	disruptedPlan, err := repo.Journeys(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(disruptedPlan.Itineraries) != 0 || len(disruptedPlan.ExcludedDisruptions) != 1 || disruptedPlan.ExcludedDisruptions[0].ID != 9 {
		t.Fatalf("expected route 7 excluded by disruption 9, got %+v", disruptedPlan)
	}

	failure := errors.New("database offline")
	mock.ExpectQuery("FROM stops").WillReturnError(failure)
	if _, err := repo.Journeys(context.Background(), q); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
