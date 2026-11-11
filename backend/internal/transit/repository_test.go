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
