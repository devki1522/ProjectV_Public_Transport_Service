package disruptions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// PostgresRepository persists and queries disruptions, route/stop links, and trip/vehicle/segment impacts in PostgreSQL.
type PostgresRepository struct {
	db *gorm.DB
}

// NewPostgresRepository creates a PostgreSQL-backed disruption repository.
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
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// RouteExists verifies that a route with routeID exists.
func (p *PostgresRepository) RouteExists(ctx context.Context, routeID int) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	var row struct{ ID int }
	return mapRecordError(p.db.WithContext(ctx).Table("routes").Select("id").Where("id = ?", routeID).Take(&row).Error)
}

// StopExists verifies that a stop with stopID exists.
func (p *PostgresRepository) StopExists(ctx context.Context, stopID int) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	var row struct{ ID int }
	return mapRecordError(p.db.WithContext(ctx).Table("stops").Select("id").Where("id = ?", stopID).Take(&row).Error)
}

// TripRouteID verifies that a trip with tripID exists and returns its route_id.
func (p *PostgresRepository) TripRouteID(ctx context.Context, tripID int) (int, error) {
	if err := p.ensureDB(); err != nil {
		return 0, err
	}
	var row struct {
		ID      int
		RouteID int
	}
	if err := p.db.WithContext(ctx).Table("trips").Select("id, route_id").Where("id = ?", tripID).Take(&row).Error; err != nil {
		return 0, mapRecordError(err)
	}
	return row.RouteID, nil
}

const listDisruptionsSQL = `
SELECT d.id, d.type, d.title, d.description, d.affected_area,
       d.start_time, d.end_time, d.status, d.source_service,
       d.external_reference, d.received_at
FROM disruptions d
WHERE (? = '' OR d.status = ?)
  AND (? = '' OR d.type = ?)
  AND (? = '' OR d.source_service = ?)
  AND (? = 0 OR EXISTS (
      SELECT 1 FROM disruption_routes dr
      WHERE dr.disruption_id = d.id AND dr.route_id = ?
  ))
  AND (? = 0 OR EXISTS (
      SELECT 1 FROM disruption_routes dr
      WHERE dr.disruption_id = d.id AND dr.stop_id = ?
  ))
  AND (? = 0 OR EXISTS (
      SELECT 1 FROM disruption_impacts di
      WHERE di.disruption_id = d.id AND di.trip_id = ?
  ))
  AND (? = 0 OR EXISTS (
      SELECT 1 FROM disruption_impacts di
      WHERE di.disruption_id = d.id AND di.vehicle_id = ?
  ))
ORDER BY
  CASE d.status WHEN 'active' THEN 0 WHEN 'scheduled' THEN 1 ELSE 2 END,
  d.start_time DESC,
  d.id DESC`

// List loads disruptions matching the provided filter and hydrates their affected routes, stops, trips, vehicles, and segments.
func (p *PostgresRepository) List(ctx context.Context, filter ListFilter) ([]DisruptionResponse, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	rows, err := p.db.WithContext(ctx).Raw(
		listDisruptionsSQL,
		filter.Status, filter.Status,
		filter.Type, filter.Type,
		filter.SourceService, filter.SourceService,
		filter.RouteID, filter.RouteID,
		filter.StopID, filter.StopID,
		filter.TripID, filter.TripID,
		filter.VehicleID, filter.VehicleID,
	).Rows()
	if err != nil {
		return nil, err
	}
	items, err := scanDisruptionRows(rows)
	if err != nil {
		return nil, err
	}
	return hydrateDisruptions(p.db.WithContext(ctx), items)
}

const getDisruptionByIDSQL = `
SELECT d.id, d.type, d.title, d.description, d.affected_area,
       d.start_time, d.end_time, d.status, d.source_service,
       d.external_reference, d.received_at
FROM disruptions d
WHERE d.id = ?`

// Get loads a single disruption by identifier and hydrates all associated transit entities.
func (p *PostgresRepository) Get(ctx context.Context, id int) (DisruptionResponse, error) {
	if err := p.ensureDB(); err != nil {
		return DisruptionResponse{}, err
	}
	return getDisruptionTx(p.db.WithContext(ctx), id)
}

func getDisruptionTx(tx *gorm.DB, id int) (DisruptionResponse, error) {
	rows, err := tx.Raw(getDisruptionByIDSQL, id).Rows()
	if err != nil {
		return DisruptionResponse{}, err
	}
	items, err := scanDisruptionRows(rows)
	if err != nil {
		return DisruptionResponse{}, err
	}
	if len(items) == 0 {
		return DisruptionResponse{}, ErrNotFound
	}
	hydrated, err := hydrateDisruptions(tx, items)
	if err != nil {
		return DisruptionResponse{}, err
	}
	return hydrated[0], nil
}

const listActiveForTripSQL = `
SELECT d.id, d.type, d.title, d.description, d.affected_area,
       d.start_time, d.end_time, d.status, d.source_service,
       d.external_reference, d.received_at
FROM disruptions d
WHERE d.status = 'active'
  AND (
      EXISTS (
          SELECT 1 FROM disruption_impacts di
          WHERE di.disruption_id = d.id AND di.trip_id = ?
      )
      OR EXISTS (
          SELECT 1 FROM disruption_routes dr
          WHERE dr.disruption_id = d.id AND dr.route_id = ?
      )
  )
ORDER BY d.start_time DESC, d.id DESC`

// ListActiveForTrip loads active disruptions affecting tripID directly or affecting its parent routeID.
func (p *PostgresRepository) ListActiveForTrip(ctx context.Context, tripID int, routeID int) ([]DisruptionResponse, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	rows, err := p.db.WithContext(ctx).Raw(listActiveForTripSQL, tripID, routeID).Rows()
	if err != nil {
		return nil, err
	}
	items, err := scanDisruptionRows(rows)
	if err != nil {
		return nil, err
	}
	return hydrateDisruptions(p.db.WithContext(ctx), items)
}

const insertDisruptionSQL = `
INSERT INTO disruptions (
	type, title, description, affected_area,
	start_time, end_time, status, source_service,
	external_reference, received_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id`

// Create inserts a new disruption along with its route, stop, trip, vehicle, and segment associations atomically.
func (p *PostgresRepository) Create(ctx context.Context, params SaveDisruptionParams) (DisruptionResponse, error) {
	if err := p.ensureDB(); err != nil {
		return DisruptionResponse{}, err
	}
	var out DisruptionResponse
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := validateEntityReferencesTx(tx, params.RouteIDs, params.StopIDs, params.TripIDs, params.VehicleIDs); err != nil {
			return err
		}

		disruptionID, err := insertDisruptionRowTx(tx, params)
		if err != nil {
			return err
		}

		if err := insertDisruptionRoutesTx(tx, disruptionID, params.RouteIDs, params.StopIDs); err != nil {
			return err
		}
		if err := insertDisruptionImpactsTx(tx, disruptionID, params.TripIDs, params.VehicleIDs, params.AffectedSegments); err != nil {
			return err
		}

		hydrated, err := getDisruptionTx(tx, disruptionID)
		if err != nil {
			return err
		}
		out = hydrated
		return nil
	})
	return out, err
}

const updateDisruptionSQL = `
UPDATE disruptions
SET type = ?,
    title = ?,
    description = ?,
    affected_area = ?,
    start_time = ?,
    end_time = ?,
    status = ?
WHERE id = ?`

// Update applies partial updates to an existing disruption and replaces any specified association lists atomically.
func (p *PostgresRepository) Update(ctx context.Context, id int, params PatchDisruptionParams) (DisruptionResponse, error) {
	if err := p.ensureDB(); err != nil {
		return DisruptionResponse{}, err
	}
	var out DisruptionResponse
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existingRows, err := tx.Raw(getDisruptionByIDSQL, id).Rows()
		if err != nil {
			return err
		}
		existingList, err := scanDisruptionRows(existingRows)
		if err != nil {
			return err
		}
		if len(existingList) == 0 {
			return ErrNotFound
		}
		cur := existingList[0]

		if params.Type != nil {
			cur.Type = *params.Type
		}
		if params.Title != nil {
			cur.Title = *params.Title
		}
		if params.DescriptionSet {
			cur.Description = params.Description
		}
		if params.AffectedAreaSet {
			cur.AffectedArea = params.AffectedArea
		}
		if params.Status != nil {
			cur.Status = *params.Status
		}
		if params.StartTime != nil {
			cur.StartTime = *params.StartTime
		}
		if params.EndTimeSet {
			cur.EndTime = params.EndTime
		}
		if cur.EndTime != nil && cur.EndTime.Before(cur.StartTime) {
			return &InvalidInput{Message: "end_time must be greater than or equal to start_time"}
		}

		var routeIDsToCheck, stopIDsToCheck, tripIDsToCheck, vehicleIDsToCheck []int
		if params.RouteIDs != nil {
			routeIDsToCheck = *params.RouteIDs
		}
		if params.StopIDs != nil {
			stopIDsToCheck = *params.StopIDs
		}
		if params.TripIDs != nil {
			tripIDsToCheck = *params.TripIDs
		}
		if params.VehicleIDs != nil {
			vehicleIDsToCheck = *params.VehicleIDs
		}
		if err := validateEntityReferencesTx(tx, routeIDsToCheck, stopIDsToCheck, tripIDsToCheck, vehicleIDsToCheck); err != nil {
			return err
		}

		if err := tx.Exec(
			updateDisruptionSQL,
			cur.Type,
			cur.Title,
			cur.Description,
			cur.AffectedArea,
			cur.StartTime,
			cur.EndTime,
			cur.Status,
			id,
		).Error; err != nil {
			return err
		}

		if params.RouteIDs != nil {
			if err := tx.Exec("DELETE FROM disruption_routes WHERE disruption_id = ? AND stop_id IS NULL", id).Error; err != nil {
				return err
			}
			if err := insertDisruptionRoutesTx(tx, id, *params.RouteIDs, nil); err != nil {
				return err
			}
		}
		if params.StopIDs != nil {
			if err := tx.Exec("DELETE FROM disruption_routes WHERE disruption_id = ? AND stop_id IS NOT NULL", id).Error; err != nil {
				return err
			}
			var fallbackRoutes []int
			if params.RouteIDs != nil {
				fallbackRoutes = *params.RouteIDs
			}
			if err := insertStopRoutesOnlyTx(tx, id, *params.StopIDs, fallbackRoutes); err != nil {
				return err
			}
		}
		if params.TripIDs != nil {
			if err := tx.Exec("DELETE FROM disruption_impacts WHERE disruption_id = ? AND trip_id IS NOT NULL", id).Error; err != nil {
				return err
			}
			if err := insertDisruptionImpactsTx(tx, id, *params.TripIDs, nil, nil); err != nil {
				return err
			}
		}
		if params.VehicleIDs != nil {
			if err := tx.Exec("DELETE FROM disruption_impacts WHERE disruption_id = ? AND vehicle_id IS NOT NULL", id).Error; err != nil {
				return err
			}
			if err := insertDisruptionImpactsTx(tx, id, nil, *params.VehicleIDs, nil); err != nil {
				return err
			}
		}
		if params.AffectedSegments != nil {
			if err := tx.Exec("DELETE FROM disruption_impacts WHERE disruption_id = ? AND segment_description IS NOT NULL", id).Error; err != nil {
				return err
			}
			if err := insertDisruptionImpactsTx(tx, id, nil, nil, *params.AffectedSegments); err != nil {
				return err
			}
		}

		hydrated, err := getDisruptionTx(tx, id)
		if err != nil {
			return err
		}
		out = hydrated
		return nil
	})
	return out, err
}

const findExternalDisruptionSQL = `
SELECT id
FROM disruptions
WHERE source_service = ? AND external_reference = ?
ORDER BY id DESC
LIMIT 1`

const updateExternalDisruptionSQL = `
UPDATE disruptions
SET type = ?,
    title = ?,
    description = ?,
    affected_area = ?,
    start_time = ?,
    end_time = ?,
    status = ?,
    received_at = ?
WHERE id = ?`

// UpsertByExternalRef creates a new disruption or idempotently updates an existing disruption matching (source_service, external_reference).
func (p *PostgresRepository) UpsertByExternalRef(ctx context.Context, params SaveDisruptionParams) (DisruptionResponse, bool, error) {
	if err := p.ensureDB(); err != nil {
		return DisruptionResponse{}, false, err
	}
	if params.ExternalReference == nil || *params.ExternalReference == "" {
		res, err := p.Create(ctx, params)
		return res, true, err
	}

	var (
		out     DisruptionResponse
		created bool
	)
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := validateEntityReferencesTx(tx, params.RouteIDs, params.StopIDs, params.TripIDs, params.VehicleIDs); err != nil {
			return err
		}

		rows, err := tx.Raw(findExternalDisruptionSQL, params.SourceService, *params.ExternalReference).Rows()
		if err != nil {
			return err
		}
		var existingID int
		hasExisting := rows.Next()
		var scanErr error
		if hasExisting {
			scanErr = rows.Scan(&existingID)
		}
		rowErr := rows.Err()
		if closeErr := rows.Close(); closeErr != nil && scanErr == nil && rowErr == nil {
			return closeErr
		}
		if scanErr != nil {
			return scanErr
		}
		if rowErr != nil {
			return rowErr
		}

		var disruptionID int
		if hasExisting {
			created = false
			disruptionID = existingID
			if err := tx.Exec(
				updateExternalDisruptionSQL,
				params.Type,
				params.Title,
				params.Description,
				params.AffectedArea,
				params.StartTime,
				params.EndTime,
				params.Status,
				params.ReceivedAt,
				disruptionID,
			).Error; err != nil {
				return err
			}
			if err := tx.Exec("DELETE FROM disruption_routes WHERE disruption_id = ?", disruptionID).Error; err != nil {
				return err
			}
			if err := tx.Exec("DELETE FROM disruption_impacts WHERE disruption_id = ?", disruptionID).Error; err != nil {
				return err
			}
		} else {
			created = true
			id, err := insertDisruptionRowTx(tx, params)
			if err != nil {
				return err
			}
			disruptionID = id
		}

		if err := insertDisruptionRoutesTx(tx, disruptionID, params.RouteIDs, params.StopIDs); err != nil {
			return err
		}
		if err := insertDisruptionImpactsTx(tx, disruptionID, params.TripIDs, params.VehicleIDs, params.AffectedSegments); err != nil {
			return err
		}

		hydrated, err := getDisruptionTx(tx, disruptionID)
		if err != nil {
			return err
		}
		out = hydrated
		return nil
	})
	return out, created, err
}

const findStopsInBoundsSQL = `
SELECT id, name, latitude, longitude
FROM stops
WHERE is_active = TRUE
  AND latitude BETWEEN ? AND ?
  AND longitude BETWEEN ? AND ?
ORDER BY id ASC`

// FindStopsInBounds loads active stops within a geographic bounding box for Haversine radius matching.
func (p *PostgresRepository) FindStopsInBounds(ctx context.Context, minLat, maxLat, minLng, maxLng float64) ([]NearbyStop, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	rows, err := p.db.WithContext(ctx).Raw(findStopsInBoundsSQL, minLat, maxLat, minLng, maxLng).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stops := make([]NearbyStop, 0)
	for rows.Next() {
		var s NearbyStop
		if err := rows.Scan(&s.ID, &s.Name, &s.Latitude, &s.Longitude); err != nil {
			return nil, err
		}
		stops = append(stops, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stops, nil
}

const listActiveImpactedTripsSQL = `
SELECT di.disruption_id, t.id AS trip_id, t.route_id
FROM disruption_impacts di
JOIN trips t ON t.id = di.trip_id
WHERE di.disruption_id IN ?
  AND di.trip_id IS NOT NULL
ORDER BY t.id ASC, di.disruption_id ASC`

// ListActiveImpactedTrips returns trip and route metadata for trips referenced by the given disruption IDs.
func (p *PostgresRepository) ListActiveImpactedTrips(ctx context.Context, disruptionIDs []int) ([]AffectedTripDetail, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	unique := uniqueSortedPositive(disruptionIDs)
	if len(unique) == 0 {
		return []AffectedTripDetail{}, nil
	}
	rows, err := p.db.WithContext(ctx).Raw(listActiveImpactedTripsSQL, unique).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]AffectedTripDetail, 0)
	for rows.Next() {
		var item AffectedTripDetail
		if err := rows.Scan(&item.DisruptionID, &item.TripID, &item.RouteID); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

const listActiveImpactedVehiclesSQL = `
SELECT di.disruption_id, v.id AS vehicle_id, v.number AS vehicle_number
FROM disruption_impacts di
JOIN vehicles v ON v.id = di.vehicle_id
WHERE di.disruption_id IN ?
  AND di.vehicle_id IS NOT NULL
ORDER BY v.number ASC, v.id ASC, di.disruption_id ASC`

// ListActiveImpactedVehicles returns vehicle metadata for vehicles referenced by the given disruption IDs.
func (p *PostgresRepository) ListActiveImpactedVehicles(ctx context.Context, disruptionIDs []int) ([]AffectedVehicleDetail, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	unique := uniqueSortedPositive(disruptionIDs)
	if len(unique) == 0 {
		return []AffectedVehicleDetail{}, nil
	}
	rows, err := p.db.WithContext(ctx).Raw(listActiveImpactedVehiclesSQL, unique).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]AffectedVehicleDetail, 0)
	for rows.Next() {
		var item AffectedVehicleDetail
		if err := rows.Scan(&item.DisruptionID, &item.VehicleID, &item.VehicleNumber); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func validateEntityReferencesTx(tx *gorm.DB, routeIDs, stopIDs, tripIDs, vehicleIDs []int) error {
	for _, check := range []struct {
		table string
		ids   []int
	}{
		{"routes", routeIDs},
		{"stops", stopIDs},
		{"trips", tripIDs},
		{"vehicles", vehicleIDs},
	} {
		unique := uniqueSortedPositive(check.ids)
		if len(unique) == 0 {
			continue
		}
		var count int64
		if err := tx.Table(check.table).Where("id IN ?", unique).Count(&count).Error; err != nil {
			return err
		}
		if int(count) != len(unique) {
			return ErrNotFound
		}
	}
	return nil
}

func insertDisruptionRowTx(tx *gorm.DB, params SaveDisruptionParams) (int, error) {
	rows, err := tx.Raw(
		insertDisruptionSQL,
		params.Type,
		params.Title,
		params.Description,
		params.AffectedArea,
		params.StartTime,
		params.EndTime,
		params.Status,
		params.SourceService,
		params.ExternalReference,
		params.ReceivedAt,
	).Rows()
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if !rows.Next() {
		if rowErr := rows.Err(); rowErr != nil {
			return 0, rowErr
		}
		return 0, fmt.Errorf("disruption insert returned no id")
	}
	var id int
	if err := rows.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

const insertDisruptionRouteWideSQL = `
INSERT INTO disruption_routes (disruption_id, route_id, stop_id)
VALUES (?, ?, NULL)
ON CONFLICT DO NOTHING`

const insertDisruptionRouteStopSQL = `
INSERT INTO disruption_routes (disruption_id, route_id, stop_id)
VALUES (?, ?, ?)
ON CONFLICT DO NOTHING`

const resolveActiveRoutesForStopsSQL = `
SELECT DISTINCT rs.route_id, rs.stop_id
FROM route_stops rs
JOIN routes r ON r.id = rs.route_id
WHERE rs.stop_id IN ? AND r.is_active = TRUE
ORDER BY rs.route_id ASC, rs.stop_id ASC`

const resolveAnyRoutesForStopsSQL = `
SELECT DISTINCT rs.route_id, rs.stop_id
FROM route_stops rs
WHERE rs.stop_id IN ?
ORDER BY rs.route_id ASC, rs.stop_id ASC`

func insertDisruptionRoutesTx(tx *gorm.DB, disruptionID int, routeIDs, stopIDs []int) error {
	for _, routeID := range uniqueSortedPositive(routeIDs) {
		if err := tx.Exec(insertDisruptionRouteWideSQL, disruptionID, routeID).Error; err != nil {
			return err
		}
	}
	return insertStopRoutesOnlyTx(tx, disruptionID, stopIDs, routeIDs)
}

func insertStopRoutesOnlyTx(tx *gorm.DB, disruptionID int, stopIDs, fallbackRouteIDs []int) error {
	uniqueStops := uniqueSortedPositive(stopIDs)
	if len(uniqueStops) == 0 {
		return nil
	}

	type routeStopPair struct {
		RouteID int
		StopID  int
	}

	rows, err := tx.Raw(resolveActiveRoutesForStopsSQL, uniqueStops).Rows()
	if err != nil {
		return err
	}
	pairs := make([]routeStopPair, 0)
	coveredStops := make(map[int]bool, len(uniqueStops))
	for rows.Next() {
		var p routeStopPair
		if err := rows.Scan(&p.RouteID, &p.StopID); err != nil {
			_ = rows.Close()
			return err
		}
		pairs = append(pairs, p)
		coveredStops[p.StopID] = true
	}
	rowErr := rows.Err()
	if closeErr := rows.Close(); closeErr != nil && rowErr == nil {
		return closeErr
	}
	if rowErr != nil {
		return rowErr
	}

	// If any stop was not served by an active route in route_stops, check all route_stops or pair with fallbackRouteIDs.
	uncovered := make([]int, 0)
	for _, sid := range uniqueStops {
		if !coveredStops[sid] {
			uncovered = append(uncovered, sid)
		}
	}
	if len(uncovered) > 0 {
		anyRows, err := tx.Raw(resolveAnyRoutesForStopsSQL, uncovered).Rows()
		if err != nil {
			return err
		}
		for anyRows.Next() {
			var p routeStopPair
			if err := anyRows.Scan(&p.RouteID, &p.StopID); err != nil {
				_ = anyRows.Close()
				return err
			}
			pairs = append(pairs, p)
			coveredStops[p.StopID] = true
		}
		anyRowErr := anyRows.Err()
		if closeErr := anyRows.Close(); closeErr != nil && anyRowErr == nil {
			return closeErr
		}
		if anyRowErr != nil {
			return anyRowErr
		}

		cleanFallback := uniqueSortedPositive(fallbackRouteIDs)
		if len(cleanFallback) > 0 {
			for _, sid := range uncovered {
				if !coveredStops[sid] {
					for _, rid := range cleanFallback {
						pairs = append(pairs, routeStopPair{RouteID: rid, StopID: sid})
					}
				}
			}
		}
	}

	seen := make(map[routeStopPair]bool, len(pairs))
	for _, p := range pairs {
		if seen[p] {
			continue
		}
		seen[p] = true
		if err := tx.Exec(insertDisruptionRouteStopSQL, disruptionID, p.RouteID, p.StopID).Error; err != nil {
			return err
		}
	}
	return nil
}

const insertDisruptionImpactSQL = `
INSERT INTO disruption_impacts (disruption_id, trip_id, vehicle_id, segment_description)
VALUES (?, ?, ?, ?)`

func insertDisruptionImpactsTx(tx *gorm.DB, disruptionID int, tripIDs, vehicleIDs []int, segments []string) error {
	for _, tripID := range uniqueSortedPositive(tripIDs) {
		tid := tripID
		if err := tx.Exec(insertDisruptionImpactSQL, disruptionID, &tid, nil, nil).Error; err != nil {
			return err
		}
	}
	for _, vehicleID := range uniqueSortedPositive(vehicleIDs) {
		vid := vehicleID
		if err := tx.Exec(insertDisruptionImpactSQL, disruptionID, nil, &vid, nil).Error; err != nil {
			return err
		}
	}
	for _, segment := range segments {
		seg := segment
		if err := tx.Exec(insertDisruptionImpactSQL, disruptionID, nil, nil, &seg).Error; err != nil {
			return err
		}
	}
	return nil
}

func scanDisruptionRows(rows *sql.Rows) ([]DisruptionResponse, error) {
	defer rows.Close()
	items := make([]DisruptionResponse, 0)
	for rows.Next() {
		var d DisruptionResponse
		var startTime, receivedAt time.Time
		if err := rows.Scan(
			&d.ID,
			&d.Type,
			&d.Title,
			&d.Description,
			&d.AffectedArea,
			&startTime,
			&d.EndTime,
			&d.Status,
			&d.SourceService,
			&d.ExternalReference,
			&receivedAt,
		); err != nil {
			return nil, err
		}
		d.StartTime = startTime.UTC()
		if d.EndTime != nil {
			utcEnd := d.EndTime.UTC()
			d.EndTime = &utcEnd
		}
		d.ReceivedAt = receivedAt.UTC()
		d = normalizeDisruptionResponse(d)
		items = append(items, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const loadDisruptionRoutesSQL = `
SELECT dr.disruption_id, dr.route_id, r.number AS route_number, r.name AS route_name, r.direction,
       dr.stop_id, s.name AS stop_name
FROM disruption_routes dr
JOIN routes r ON r.id = dr.route_id
LEFT JOIN stops s ON s.id = dr.stop_id
WHERE dr.disruption_id IN ?
ORDER BY dr.disruption_id ASC, r.number ASC, r.direction ASC, r.id ASC, dr.stop_id ASC NULLS FIRST`

const loadDisruptionImpactsSQL = `
SELECT di.disruption_id, di.trip_id, di.vehicle_id, di.segment_description
FROM disruption_impacts di
WHERE di.disruption_id IN ?
ORDER BY di.disruption_id ASC, di.id ASC`

func hydrateDisruptions(tx *gorm.DB, items []DisruptionResponse) ([]DisruptionResponse, error) {
	if len(items) == 0 {
		return []DisruptionResponse{}, nil
	}

	indexByID := make(map[int]int, len(items))
	ids := make([]int, 0, len(items))
	for i := range items {
		items[i] = normalizeDisruptionResponse(items[i])
		indexByID[items[i].ID] = i
		ids = append(ids, items[i].ID)
	}

	type routeSeenKey struct {
		DisruptionID int
		RouteID      int
	}
	type stopSeenKey struct {
		DisruptionID int
		StopID       int
		RouteID      int
	}
	seenRoutes := make(map[routeSeenKey]bool)
	seenStops := make(map[stopSeenKey]bool)

	routeRows, err := tx.Raw(loadDisruptionRoutesSQL, ids).Rows()
	if err != nil {
		return nil, err
	}
	for routeRows.Next() {
		var (
			disruptionID int
			routeID      int
			routeNumber  string
			routeName    string
			direction    string
			stopID       *int
			stopName     *string
		)
		if err := routeRows.Scan(&disruptionID, &routeID, &routeNumber, &routeName, &direction, &stopID, &stopName); err != nil {
			_ = routeRows.Close()
			return nil, err
		}
		idx, ok := indexByID[disruptionID]
		if !ok {
			continue
		}
		rKey := routeSeenKey{DisruptionID: disruptionID, RouteID: routeID}
		if !seenRoutes[rKey] {
			seenRoutes[rKey] = true
			items[idx].AffectedRoutes = append(items[idx].AffectedRoutes, AffectedRoute{
				RouteID:     routeID,
				RouteNumber: routeNumber,
				RouteName:   routeName,
				Direction:   direction,
			})
		}
		if stopID != nil && stopName != nil {
			sKey := stopSeenKey{DisruptionID: disruptionID, StopID: *stopID, RouteID: routeID}
			if !seenStops[sKey] {
				seenStops[sKey] = true
				items[idx].AffectedStops = append(items[idx].AffectedStops, AffectedStop{
					StopID:   *stopID,
					StopName: *stopName,
					RouteID:  routeID,
				})
			}
		}
	}
	routeRowErr := routeRows.Err()
	if closeErr := routeRows.Close(); closeErr != nil && routeRowErr == nil {
		return nil, closeErr
	}
	if routeRowErr != nil {
		return nil, routeRowErr
	}

	type intSeenKey struct {
		DisruptionID int
		EntityID     int
	}
	type segSeenKey struct {
		DisruptionID int
		Segment      string
	}
	seenTrips := make(map[intSeenKey]bool)
	seenVehicles := make(map[intSeenKey]bool)
	seenSegments := make(map[segSeenKey]bool)

	impactRows, err := tx.Raw(loadDisruptionImpactsSQL, ids).Rows()
	if err != nil {
		return nil, err
	}
	defer impactRows.Close()

	for impactRows.Next() {
		var (
			disruptionID int
			tripID       *int
			vehicleID    *int
			segment      *string
		)
		if err := impactRows.Scan(&disruptionID, &tripID, &vehicleID, &segment); err != nil {
			return nil, err
		}
		idx, ok := indexByID[disruptionID]
		if !ok {
			continue
		}
		if tripID != nil {
			k := intSeenKey{DisruptionID: disruptionID, EntityID: *tripID}
			if !seenTrips[k] {
				seenTrips[k] = true
				items[idx].AffectedTrips = append(items[idx].AffectedTrips, *tripID)
			}
		}
		if vehicleID != nil {
			k := intSeenKey{DisruptionID: disruptionID, EntityID: *vehicleID}
			if !seenVehicles[k] {
				seenVehicles[k] = true
				items[idx].AffectedVehicles = append(items[idx].AffectedVehicles, *vehicleID)
			}
		}
		if segment != nil && *segment != "" {
			k := segSeenKey{DisruptionID: disruptionID, Segment: *segment}
			if !seenSegments[k] {
				seenSegments[k] = true
				items[idx].AffectedSegments = append(items[idx].AffectedSegments, *segment)
			}
		}
	}
	if err := impactRows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
