package vehicles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

// PostgresRepository stores and queries live vehicle locations and route geometries in PostgreSQL.
type PostgresRepository struct {
	db *gorm.DB
}

// NewPostgresRepository constructs a PostgreSQL repository for live vehicle operations.
func NewPostgresRepository(db *gorm.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (p *PostgresRepository) ensureDB() error {
	if p == nil || p.db == nil {
		return fmt.Errorf("database connection unavailable")
	}
	return nil
}

func mapRecordError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// RouteExists reports whether a route exists for the given route identifier.
func (p *PostgresRepository) RouteExists(ctx context.Context, routeID int) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	var row struct{ ID int }
	return mapRecordError(p.db.WithContext(ctx).Table("routes").Select("id").Where("id = ?", routeID).Take(&row).Error)
}

// TripExists reports whether a trip exists for the given trip identifier.
func (p *PostgresRepository) TripExists(ctx context.Context, tripID int) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	var row struct{ ID int }
	return mapRecordError(p.db.WithContext(ctx).Table("trips").Select("id").Where("id = ?", tripID).Take(&row).Error)
}

const upsertVehicleByNumberSQL = `
INSERT INTO vehicles (number, is_active, is_accessible)
VALUES (?, TRUE, ?)
ON CONFLICT (number) DO UPDATE SET number = EXCLUDED.number
RETURNING id, number, is_active, is_accessible`

const upsertVehicleLocationSQL = `
INSERT INTO vehicle_locations (
	vehicle_id, route_id, trip_id, latitude, longitude,
	heading, speed_kmh, status, source_service, is_simulated,
	recorded_at, received_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (vehicle_id) DO UPDATE SET
	route_id = EXCLUDED.route_id,
	trip_id = EXCLUDED.trip_id,
	latitude = EXCLUDED.latitude,
	longitude = EXCLUDED.longitude,
	heading = EXCLUDED.heading,
	speed_kmh = EXCLUDED.speed_kmh,
	status = EXCLUDED.status,
	source_service = EXCLUDED.source_service,
	is_simulated = EXCLUDED.is_simulated,
	recorded_at = EXCLUDED.recorded_at,
	received_at = EXCLUDED.received_at
WHERE EXCLUDED.recorded_at >= vehicle_locations.recorded_at
RETURNING (xmax = 0) AS created`

// UpsertLocation resolves route/trip/vehicle references and stores the latest vehicle location atomically.
func (p *PostgresRepository) UpsertLocation(ctx context.Context, params LocationUpdateParams) (LocationUpdateResult, error) {
	if err := p.ensureDB(); err != nil {
		return LocationUpdateResult{}, err
	}

	var result LocationUpdateResult
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var routeRow struct {
			ID        int
			Number    string
			Name      string
			Direction string
			IsActive  bool
		}
		if err := tx.Table("routes").
			Select("id, number, name, direction, is_active").
			Where("id = ?", params.RouteID).
			Take(&routeRow).Error; err != nil {
			return mapRecordError(err)
		}
		if !routeRow.IsActive {
			return &InvalidInput{Message: "route is inactive"}
		}

		if params.TripID != nil {
			var tripRow struct {
				ID       int
				RouteID  int
				IsActive bool
			}
			if err := tx.Table("trips").
				Select("id, route_id, is_active").
				Where("id = ?", *params.TripID).
				Take(&tripRow).Error; err != nil {
				return mapRecordError(err)
			}
			if tripRow.RouteID != params.RouteID {
				return &InvalidInput{Message: "trip_id does not belong to route_id"}
			}
			if !tripRow.IsActive {
				return &InvalidInput{Message: "trip is inactive"}
			}
		}

		var vehRow struct {
			ID           int
			Number       string
			IsActive     bool
			IsAccessible bool
		}
		if params.VehicleID != nil {
			if err := tx.Table("vehicles").
				Select("id, number, is_active, is_accessible").
				Where("id = ?", *params.VehicleID).
				Take(&vehRow).Error; err != nil {
				return mapRecordError(err)
			}
			if !vehRow.IsActive {
				return &InvalidInput{Message: "vehicle is inactive"}
			}
			if params.VehicleNumber != "" && params.VehicleNumber != vehRow.Number {
				return &InvalidInput{Message: "vehicle_number does not match vehicle_id"}
			}
		} else {
			rows, err := tx.Raw(upsertVehicleByNumberSQL, params.VehicleNumber, params.IsAccessible).Rows()
			if err != nil {
				return err
			}
			hasRow := rows.Next()
			rowErr := rows.Err()
			var scanErr error
			if hasRow {
				scanErr = rows.Scan(&vehRow.ID, &vehRow.Number, &vehRow.IsActive, &vehRow.IsAccessible)
			}
			if closeErr := rows.Close(); closeErr != nil && scanErr == nil && rowErr == nil {
				return closeErr
			}
			if !hasRow {
				if rowErr != nil {
					return rowErr
				}
				return fmt.Errorf("vehicle upsert returned no row")
			}
			if scanErr != nil {
				return scanErr
			}
			if !vehRow.IsActive {
				return &InvalidInput{Message: "vehicle is inactive"}
			}
		}

		locRows, err := tx.Raw(
			upsertVehicleLocationSQL,
			vehRow.ID,
			params.RouteID,
			params.TripID,
			params.Latitude,
			params.Longitude,
			params.Heading,
			params.SpeedKmh,
			params.Status,
			params.SourceService,
			params.IsSimulated,
			params.RecordedAt,
			params.ReceivedAt,
		).Rows()
		if err != nil {
			return err
		}
		defer locRows.Close()

		if !locRows.Next() {
			if rowErr := locRows.Err(); rowErr != nil {
				return rowErr
			}
			return ErrStaleLocationUpdate
		}

		var created bool
		if err := locRows.Scan(&created); err != nil {
			return err
		}

		result = LocationUpdateResult{
			Created: created,
			Vehicle: ActiveVehicle{
				VehicleID:     vehRow.ID,
				VehicleNumber: vehRow.Number,
				IsAccessible:  vehRow.IsAccessible,
				RouteID:       routeRow.ID,
				RouteNumber:   routeRow.Number,
				RouteName:     routeRow.Name,
				Direction:     routeRow.Direction,
				TripID:        params.TripID,
				Latitude:      params.Latitude,
				Longitude:     params.Longitude,
				Heading:       params.Heading,
				SpeedKmh:      params.SpeedKmh,
				Status:        params.Status,
				SourceService: params.SourceService,
				IsSimulated:   params.IsSimulated,
				RecordedAt:    params.RecordedAt,
				ReceivedAt:    params.ReceivedAt,
			},
		}
		return nil
	})

	return result, err
}

const listActiveVehiclesSQL = `
SELECT vl.vehicle_id, v.number AS vehicle_number, v.is_accessible,
       vl.route_id, r.number AS route_number, r.name AS route_name, r.direction,
       vl.trip_id, vl.latitude, vl.longitude, vl.heading, vl.speed_kmh,
       vl.status, vl.source_service, vl.is_simulated, vl.recorded_at, vl.received_at
FROM vehicle_locations vl
JOIN vehicles v ON v.id = vl.vehicle_id
JOIN routes r ON r.id = vl.route_id
WHERE v.is_active = TRUE
  AND r.is_active = TRUE
  AND vl.status <> 'out_of_service'
  AND vl.recorded_at >= ?
  AND (? = 0 OR vl.route_id = ?)
  AND (? = '' OR LOWER(r.number) = LOWER(?))
  AND (? = 0 OR vl.trip_id = ?)
ORDER BY r.number, r.direction, v.number`

// ListActiveVehicles loads fresh non-retired vehicle positions matching the provided filters.
func (p *PostgresRepository) ListActiveVehicles(
	ctx context.Context,
	cutoff time.Time,
	routeID int,
	routeNumber string,
	tripID int,
) ([]ActiveVehicle, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}

	rows, err := p.db.WithContext(ctx).Raw(
		listActiveVehiclesSQL,
		cutoff.UTC(),
		routeID, routeID,
		strings.TrimSpace(routeNumber), strings.TrimSpace(routeNumber),
		tripID, tripID,
	).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanActiveVehicleRows(rows)
}

func scanActiveVehicleRows(rows *sql.Rows) ([]ActiveVehicle, error) {
	list := make([]ActiveVehicle, 0)
	for rows.Next() {
		var item ActiveVehicle
		if err := rows.Scan(
			&item.VehicleID,
			&item.VehicleNumber,
			&item.IsAccessible,
			&item.RouteID,
			&item.RouteNumber,
			&item.RouteName,
			&item.Direction,
			&item.TripID,
			&item.Latitude,
			&item.Longitude,
			&item.Heading,
			&item.SpeedKmh,
			&item.Status,
			&item.SourceService,
			&item.IsSimulated,
			&item.RecordedAt,
			&item.ReceivedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

const routePrimaryShapeSQL = `
SELECT rp.headsign, rs.geometry
FROM route_patterns rp
JOIN route_shapes rs ON rs.id = rp.shape_id
WHERE rp.route_id = ?
ORDER BY rp.is_primary DESC, rp.id ASC
LIMIT 1`

const routeFallbackStopsSQL = `
SELECT s.latitude, s.longitude
FROM route_stops rs
JOIN stops s ON s.id = rs.stop_id
WHERE rs.route_id = ? AND s.is_active = TRUE
ORDER BY rs.stop_sequence ASC`

// RouteGeometry returns the primary LineString geometry for routeID, falling back to ordered route stop coordinates.
func (p *PostgresRepository) RouteGeometry(ctx context.Context, routeID int) (RouteGeometry, error) {
	if err := p.ensureDB(); err != nil {
		return RouteGeometry{}, err
	}

	var routeRow struct {
		ID        int
		Number    string
		Name      string
		Direction string
	}
	if err := p.db.WithContext(ctx).
		Table("routes").
		Select("id, number, name, direction").
		Where("id = ?", routeID).
		Take(&routeRow).Error; err != nil {
		return RouteGeometry{}, mapRecordError(err)
	}

	geom := RouteGeometry{
		RouteID:     routeRow.ID,
		RouteNumber: routeRow.Number,
		RouteName:   routeRow.Name,
		Direction:   routeRow.Direction,
		Coordinates: []Coordinate{},
	}

	shapeRows, err := p.db.WithContext(ctx).Raw(routePrimaryShapeSQL, routeID).Rows()
	if err != nil {
		return RouteGeometry{}, err
	}
	hasShape := shapeRows.Next()
	shapeRowErr := shapeRows.Err()
	var headsign string
	var rawJSON []byte
	var shapeScanErr error
	if hasShape {
		shapeScanErr = shapeRows.Scan(&headsign, &rawJSON)
	}
	if closeErr := shapeRows.Close(); closeErr != nil && shapeScanErr == nil && shapeRowErr == nil {
		return RouteGeometry{}, closeErr
	}
	if shapeScanErr != nil {
		return RouteGeometry{}, shapeScanErr
	}
	if shapeRowErr != nil {
		return RouteGeometry{}, shapeRowErr
	}
	if hasShape {
		geom.Headsign = headsign
		coords, parseErr := parseGeoJSONLineString(rawJSON)
		if parseErr == nil && len(coords) >= 2 {
			geom.Coordinates = coords
			return geom, nil
		}
	}

	stopRows, err := p.db.WithContext(ctx).Raw(routeFallbackStopsSQL, routeID).Rows()
	if err != nil {
		return RouteGeometry{}, err
	}
	defer stopRows.Close()

	for stopRows.Next() {
		var coord Coordinate
		if err := stopRows.Scan(&coord.Lat, &coord.Lng); err != nil {
			return RouteGeometry{}, err
		}
		geom.Coordinates = append(geom.Coordinates, coord)
	}
	if err := stopRows.Err(); err != nil {
		return RouteGeometry{}, err
	}
	return geom, nil
}

const activeRoutesWithGeometriesSQL = `
SELECT DISTINCT ON (r.id)
       r.id, r.number, r.name, r.direction,
       COALESCE(rp.headsign, '') AS headsign,
       sh.geometry,
       (SELECT t.id FROM trips t WHERE t.route_id = r.id AND t.is_active = TRUE ORDER BY t.id ASC LIMIT 1) AS sample_trip_id
FROM routes r
LEFT JOIN route_patterns rp ON rp.route_id = r.id
LEFT JOIN route_shapes sh ON sh.id = rp.shape_id
WHERE r.is_active = TRUE
ORDER BY r.id, rp.is_primary DESC NULLS LAST, rp.id ASC`

// ActiveRoutesWithGeometries returns active routes paired with polyline coordinates, diversified across route numbers.
func (p *PostgresRepository) ActiveRoutesWithGeometries(ctx context.Context, limit int) ([]SimulatedRoute, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}

	rows, err := p.db.WithContext(ctx).Raw(activeRoutesWithGeometriesSQL).Rows()
	if err != nil {
		return nil, err
	}

	candidates := make([]SimulatedRoute, 0)
	for rows.Next() {
		var item SimulatedRoute
		var rawJSON []byte
		if err := rows.Scan(
			&item.RouteID,
			&item.RouteNumber,
			&item.RouteName,
			&item.Direction,
			&item.Headsign,
			&rawJSON,
			&item.SampleTripID,
		); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if len(rawJSON) > 0 {
			if coords, parseErr := parseGeoJSONLineString(rawJSON); parseErr == nil && len(coords) >= 2 {
				item.Coordinates = coords
			}
		}
		candidates = append(candidates, item)
	}
	rowErr := rows.Err()
	if closeErr := rows.Close(); closeErr != nil && rowErr == nil {
		return nil, closeErr
	}
	if rowErr != nil {
		return nil, rowErr
	}

	for i := range candidates {
		if len(candidates[i].Coordinates) >= 2 {
			continue
		}
		fallback, err := p.RouteGeometry(ctx, candidates[i].RouteID)
		if err != nil {
			return nil, err
		}
		candidates[i].Coordinates = fallback.Coordinates
	}

	usable := make([]SimulatedRoute, 0, len(candidates))
	for _, item := range candidates {
		if len(item.Coordinates) >= 2 {
			usable = append(usable, item)
		}
	}
	if limit <= 0 || len(usable) <= limit {
		return usable, nil
	}

	// Spread simulated vehicles across distinct public route numbers before adding opposite directions.
	selected := make([]SimulatedRoute, 0, limit)
	usedRoute := make([]bool, len(usable))
	seenNumbers := make(map[string]struct{}, limit)
	for i, item := range usable {
		if _, seen := seenNumbers[item.RouteNumber]; seen {
			continue
		}
		seenNumbers[item.RouteNumber] = struct{}{}
		usedRoute[i] = true
		selected = append(selected, item)
		if len(selected) == limit {
			return selected, nil
		}
	}
	for i, item := range usable {
		if usedRoute[i] {
			continue
		}
		selected = append(selected, item)
		if len(selected) == limit {
			break
		}
	}
	return selected, nil
}

// ClearSimulatedLocations removes vehicle locations marked as simulated while preserving real feeds.
func (p *PostgresRepository) ClearSimulatedLocations(ctx context.Context) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	return p.db.WithContext(ctx).Exec("DELETE FROM vehicle_locations WHERE is_simulated = TRUE").Error
}

type geoJSONLineString struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

func parseGeoJSONLineString(raw []byte) ([]Coordinate, error) {
	var ls geoJSONLineString
	if err := json.Unmarshal(raw, &ls); err != nil {
		return nil, err
	}
	if ls.Type != "LineString" {
		return nil, fmt.Errorf("unexpected geometry type %q", ls.Type)
	}
	coords := make([]Coordinate, 0, len(ls.Coordinates))
	for _, pair := range ls.Coordinates {
		if len(pair) < 2 {
			continue
		}
		lng, lat := pair[0], pair[1]
		if !isFiniteInRange(lat, -90, 90) || !isFiniteInRange(lng, -180, 180) {
			continue
		}
		// Skip consecutive duplicates so heading calculations stay well-defined.
		if len(coords) > 0 {
			prev := coords[len(coords)-1]
			if math.Abs(prev.Lat-lat) < 1e-9 && math.Abs(prev.Lng-lng) < 1e-9 {
				continue
			}
		}
		coords = append(coords, Coordinate{Lat: lat, Lng: lng})
	}
	return coords, nil
}
