package vehicles

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

func mockVehicleRepository(t *testing.T) (*PostgresRepository, sqlmock.Sqlmock) {
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

func TestUpsertLocationCreatesUpdatesAndRejectsStale(t *testing.T) {
	now := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)

	t.Run("creates vehicle by number and inserts location", func(t *testing.T) {
		repo, mock := mockVehicleRepository(t)
		heading := 45.0
		speed := 32.0
		tripID := 200

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT id, number, name, direction, is_active FROM "routes"`).
			WithArgs(7, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "number", "name", "direction", "is_active"}).
				AddRow(7, "7", "King", "direction_0", true))
		mock.ExpectQuery(`SELECT id, route_id, is_active FROM "trips"`).
			WithArgs(200, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "route_id", "is_active"}).
				AddRow(200, 7, true))
		mock.ExpectQuery(`INSERT INTO vehicles`).
			WithArgs("GRT-301", true).
			WillReturnRows(sqlmock.NewRows([]string{"id", "number", "is_active", "is_accessible"}).
				AddRow(55, "GRT-301", true, true))
		mock.ExpectQuery(`INSERT INTO vehicle_locations`).
			WillReturnRows(sqlmock.NewRows([]string{"created"}).AddRow(true))
		mock.ExpectCommit()

		res, err := repo.UpsertLocation(context.Background(), LocationUpdateParams{
			VehicleNumber: "GRT-301",
			IsAccessible:  true,
			RouteID:       7,
			TripID:        &tripID,
			Latitude:      43.4516,
			Longitude:     -80.4925,
			Heading:       &heading,
			SpeedKmh:      &speed,
			Status:        StatusInService,
			SourceService: "dispatch",
			RecordedAt:    now,
			ReceivedAt:    now,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Created || res.Vehicle.VehicleID != 55 || res.Vehicle.RouteNumber != "7" || res.Vehicle.TripID == nil || *res.Vehicle.TripID != 200 {
			t.Fatalf("unexpected upsert result: %+v", res)
		}
	})

	t.Run("rejects stale timestamp conflict", func(t *testing.T) {
		repo, mock := mockVehicleRepository(t)
		vehID := 55

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT id, number, name, direction, is_active FROM "routes"`).
			WithArgs(7, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "number", "name", "direction", "is_active"}).
				AddRow(7, "7", "King", "direction_0", true))
		mock.ExpectQuery(`SELECT id, number, is_active, is_accessible FROM "vehicles"`).
			WithArgs(55, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "number", "is_active", "is_accessible"}).
				AddRow(55, "GRT-301", true, true))
		mock.ExpectQuery(`INSERT INTO vehicle_locations`).
			WillReturnRows(sqlmock.NewRows([]string{"created"}))
		mock.ExpectRollback()

		_, err := repo.UpsertLocation(context.Background(), LocationUpdateParams{
			VehicleID:     &vehID,
			RouteID:       7,
			Latitude:      43.4516,
			Longitude:     -80.4925,
			Status:        StatusInService,
			SourceService: "dispatch",
			RecordedAt:    now.Add(-time.Minute),
			ReceivedAt:    now,
		})
		if !errors.Is(err, ErrStaleLocationUpdate) {
			t.Fatalf("got %v, want ErrStaleLocationUpdate", err)
		}
	})

	t.Run("rejects inactive route", func(t *testing.T) {
		repo, mock := mockVehicleRepository(t)
		vehID := 55

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT id, number, name, direction, is_active FROM "routes"`).
			WithArgs(9, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "number", "name", "direction", "is_active"}).
				AddRow(9, "9", "Lakeshore", "direction_0", false))
		mock.ExpectRollback()

		_, err := repo.UpsertLocation(context.Background(), LocationUpdateParams{
			VehicleID:     &vehID,
			RouteID:       9,
			Latitude:      43.45,
			Longitude:     -80.49,
			Status:        StatusInService,
			SourceService: "dispatch",
			RecordedAt:    now,
			ReceivedAt:    now,
		})
		var invalid *InvalidInput
		if !errors.As(err, &invalid) {
			t.Fatalf("got %v, want *InvalidInput", err)
		}
	})
}

func TestListActiveVehiclesAndRouteGeometry(t *testing.T) {
	now := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)

	t.Run("maps active vehicle rows", func(t *testing.T) {
		repo, mock := mockVehicleRepository(t)
		cutoff := now.Add(-15 * time.Minute)
		mock.ExpectQuery(`SELECT vl.vehicle_id`).
			WithArgs(cutoff, 7, 7, "", "", 0, 0).
			WillReturnRows(sqlmock.NewRows([]string{
				"vehicle_id", "vehicle_number", "is_accessible",
				"route_id", "route_number", "route_name", "direction",
				"trip_id", "latitude", "longitude", "heading", "speed_kmh",
				"status", "source_service", "is_simulated", "recorded_at", "received_at",
			}).AddRow(55, "GRT-301", true, 7, "7", "King", "direction_0", 200, 43.45, -80.49, 180.0, 35.0, "delayed", "simulator", true, now, now))

		list, err := repo.ListActiveVehicles(context.Background(), cutoff, 7, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].VehicleNumber != "GRT-301" || list[0].Status != "delayed" || !list[0].IsSimulated {
			t.Fatalf("unexpected vehicles: %+v", list)
		}
	})

	t.Run("loads GeoJSON route shape", func(t *testing.T) {
		repo, mock := mockVehicleRepository(t)
		mock.ExpectQuery(`SELECT id, number, name, direction FROM "routes"`).
			WithArgs(7, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "number", "name", "direction"}).
				AddRow(7, "7", "King", "direction_0"))
		mock.ExpectQuery(`SELECT rp.headsign, rs.geometry`).
			WithArgs(7).
			WillReturnRows(sqlmock.NewRows([]string{"headsign", "geometry"}).
				AddRow("Conestoga Mall", []byte(`{"type":"LineString","coordinates":[[-80.4925,43.4516],[-80.5201,43.4723]]}`)))

		geom, err := repo.RouteGeometry(context.Background(), 7)
		if err != nil {
			t.Fatal(err)
		}
		if geom.Headsign != "Conestoga Mall" || len(geom.Coordinates) != 2 || geom.Coordinates[0].Lat != 43.4516 || geom.Coordinates[0].Lng != -80.4925 {
			t.Fatalf("unexpected geometry: %+v", geom)
		}
	})

	t.Run("falls back to route_stops when route_shapes missing", func(t *testing.T) {
		repo, mock := mockVehicleRepository(t)
		mock.ExpectQuery(`SELECT id, number, name, direction FROM "routes"`).
			WithArgs(8, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "number", "name", "direction"}).
				AddRow(8, "8", "Weber", "direction_0"))
		mock.ExpectQuery(`SELECT rp.headsign, rs.geometry`).
			WithArgs(8).
			WillReturnRows(sqlmock.NewRows([]string{"headsign", "geometry"}))
		mock.ExpectQuery(`SELECT s.latitude, s.longitude`).
			WithArgs(8).
			WillReturnRows(sqlmock.NewRows([]string{"latitude", "longitude"}).
				AddRow(43.44, -80.48).
				AddRow(43.46, -80.51))

		geom, err := repo.RouteGeometry(context.Background(), 8)
		if err != nil {
			t.Fatal(err)
		}
		if len(geom.Coordinates) != 2 || geom.Coordinates[1].Lat != 43.46 {
			t.Fatalf("unexpected fallback geometry: %+v", geom)
		}
	})
}
