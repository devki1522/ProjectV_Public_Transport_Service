package disruptions

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

func mockDisruptionRepository(t *testing.T) (*PostgresRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	gormDB, err := gorm.Open(
		postgres.New(postgres.Config{Conn: db}),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
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

func TestRepositoryCreateAndHydrate(t *testing.T) {
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)

	t.Run("creates disruption, resolves routes for stops via route_stops, and hydrates response", func(t *testing.T) {
		repo, mock := mockDisruptionRepository(t)
		desc := "Road work on King"
		area := "Uptown Waterloo"

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT count\(\*\) FROM "routes" WHERE id IN \(\$1\)`).
			WithArgs(7).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT count\(\*\) FROM "stops" WHERE id IN \(\$1\)`).
			WithArgs(3).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT count\(\*\) FROM "trips" WHERE id IN \(\$1\)`).
			WithArgs(100).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT count\(\*\) FROM "vehicles" WHERE id IN \(\$1\)`).
			WithArgs(55).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(`INSERT INTO disruptions`).
			WithArgs(TypeConstruction, "King Construction", &desc, &area, now, nil, StatusActive, SourceTransitAdmin, nil, now).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(10))

		mock.ExpectExec(`INSERT INTO disruption_routes \(disruption_id, route_id, stop_id\) VALUES \(\$1, \$2, NULL\)`).
			WithArgs(10, 7).
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectQuery(`SELECT DISTINCT rs\.route_id, rs\.stop_id FROM route_stops rs JOIN routes r`).
			WithArgs(3).
			WillReturnRows(sqlmock.NewRows([]string{"route_id", "stop_id"}).AddRow(7, 3).AddRow(9, 3))

		mock.ExpectExec(`INSERT INTO disruption_routes \(disruption_id, route_id, stop_id\) VALUES \(\$1, \$2, \$3\)`).
			WithArgs(10, 7, 3).
			WillReturnResult(sqlmock.NewResult(2, 1))
		mock.ExpectExec(`INSERT INTO disruption_routes \(disruption_id, route_id, stop_id\) VALUES \(\$1, \$2, \$3\)`).
			WithArgs(10, 9, 3).
			WillReturnResult(sqlmock.NewResult(3, 1))

		mock.ExpectExec(`INSERT INTO disruption_impacts`).
			WithArgs(10, 100, nil, nil).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(`INSERT INTO disruption_impacts`).
			WithArgs(10, nil, 55, nil).
			WillReturnResult(sqlmock.NewResult(2, 1))
		mock.ExpectExec(`INSERT INTO disruption_impacts`).
			WithArgs(10, nil, nil, "King St N").
			WillReturnResult(sqlmock.NewResult(3, 1))

		mock.ExpectQuery(`SELECT d\.id, d\.type, d\.title`).
			WithArgs(10).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "type", "title", "description", "affected_area",
				"start_time", "end_time", "status", "source_service",
				"external_reference", "received_at",
			}).AddRow(10, TypeConstruction, "King Construction", desc, area, now, nil, StatusActive, SourceTransitAdmin, nil, now))

		mock.ExpectQuery(`SELECT dr\.disruption_id, dr\.route_id`).
			WithArgs(10).
			WillReturnRows(sqlmock.NewRows([]string{
				"disruption_id", "route_id", "route_number", "route_name", "direction", "stop_id", "stop_name",
			}).
				AddRow(10, 7, "7", "King", "direction_0", nil, nil).
				AddRow(10, 7, "7", "King", "direction_0", 3, "Midtown Hub").
				AddRow(10, 9, "9", "Lakeshore", "direction_0", 3, "Midtown Hub"))

		mock.ExpectQuery(`SELECT di\.disruption_id, di\.trip_id, di\.vehicle_id, di\.segment_description`).
			WithArgs(10).
			WillReturnRows(sqlmock.NewRows([]string{
				"disruption_id", "trip_id", "vehicle_id", "segment_description",
			}).
				AddRow(10, 100, nil, nil).
				AddRow(10, nil, 55, nil).
				AddRow(10, nil, nil, "King St N"))

		mock.ExpectCommit()

		created, err := repo.Create(context.Background(), SaveDisruptionParams{
			Type:             TypeConstruction,
			Title:            "King Construction",
			Description:      &desc,
			AffectedArea:     &area,
			StartTime:        now,
			Status:           StatusActive,
			SourceService:    SourceTransitAdmin,
			ReceivedAt:       now,
			RouteIDs:         []int{7},
			StopIDs:          []int{3},
			TripIDs:          []int{100},
			VehicleIDs:       []int{55},
			AffectedSegments: []string{"King St N"},
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if created.ID != 10 || len(created.AffectedRoutes) != 2 || len(created.AffectedStops) != 2 {
			t.Fatalf("unexpected hydrated disruption: %+v", created)
		}
		if len(created.AffectedTrips) != 1 || created.AffectedTrips[0] != 100 ||
			len(created.AffectedVehicles) != 1 || created.AffectedVehicles[0] != 55 ||
			len(created.AffectedSegments) != 1 || created.AffectedSegments[0] != "King St N" {
			t.Fatalf("unexpected hydrated impacts: %+v", created)
		}
	})

	t.Run("returns ErrNotFound when referenced route does not exist", func(t *testing.T) {
		repo, mock := mockDisruptionRepository(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT count\(\*\) FROM "routes" WHERE id IN \(\$1\)`).
			WithArgs(999).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectRollback()

		_, err := repo.Create(context.Background(), SaveDisruptionParams{
			Type:          TypeDelay,
			Title:         "Missing Route",
			StartTime:     now,
			Status:        StatusActive,
			SourceService: SourceTransitAdmin,
			ReceivedAt:    now,
			RouteIDs:      []int{999},
		})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestRepositoryUpdate(t *testing.T) {
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)

	t.Run("rejects update when patched end_time is before existing start_time", func(t *testing.T) {
		repo, mock := mockDisruptionRepository(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT d\.id, d\.type, d\.title`).
			WithArgs(10).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "type", "title", "description", "affected_area",
				"start_time", "end_time", "status", "source_service",
				"external_reference", "received_at",
			}).AddRow(10, TypeDelay, "Delay", nil, nil, now, nil, StatusActive, SourceTransitAdmin, nil, now))
		mock.ExpectRollback()

		badEnd := now.Add(-10 * time.Minute)
		_, err := repo.Update(context.Background(), 10, PatchDisruptionParams{
			EndTime:    &badEnd,
			EndTimeSet: true,
		})
		var invalid *InvalidInput
		if !errors.As(err, &invalid) {
			t.Fatalf("expected *InvalidInput for end_time < start_time, got %v", err)
		}
	})
}

func TestRepositoryListAndBoundsQueries(t *testing.T) {
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	repo, mock := mockDisruptionRepository(t)

	mock.ExpectQuery(`SELECT id, name, latitude, longitude FROM stops`).
		WithArgs(43.40, 43.50, -80.55, -80.45).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "latitude", "longitude"}).
			AddRow(3, "Midtown Hub", 43.45, -80.49))

	stops, err := repo.FindStopsInBounds(context.Background(), 43.40, 43.50, -80.55, -80.45)
	if err != nil || len(stops) != 1 || stops[0].ID != 3 {
		t.Fatalf("FindStopsInBounds failed: err=%v stops=%+v", err, stops)
	}

	mock.ExpectQuery(`SELECT d\.id, d\.type, d\.title`).
		WithArgs(StatusActive, StatusActive, "", "", "", "", 7, 7, 0, 0, 0, 0, 0, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "type", "title", "description", "affected_area",
			"start_time", "end_time", "status", "source_service",
			"external_reference", "received_at",
		}).AddRow(10, TypeDetour, "Detour", nil, nil, now, nil, StatusActive, SourceTransitAdmin, nil, now))

	mock.ExpectQuery(`SELECT dr\.disruption_id, dr\.route_id`).
		WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{
			"disruption_id", "route_id", "route_number", "route_name", "direction", "stop_id", "stop_name",
		}).AddRow(10, 7, "7", "King", "direction_0", nil, nil))

	mock.ExpectQuery(`SELECT di\.disruption_id, di\.trip_id`).
		WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{
			"disruption_id", "trip_id", "vehicle_id", "segment_description",
		}))

	list, err := repo.List(context.Background(), ListFilter{Status: StatusActive, RouteID: 7})
	if err != nil || len(list) != 1 || list[0].ID != 10 {
		t.Fatalf("List failed: err=%v list=%+v", err, list)
	}
}
