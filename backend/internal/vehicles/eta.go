package vehicles

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
	_ "time/tzdata"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/transit"
	"gorm.io/gorm"
)

const (
	defaultTransitSpeedKmh         = 28.0
	minTransitSpeedKmh             = 15.0
	maxTransitSpeedKmh             = 65.0
	delayedTravelFactor            = 1.25
	layoverDwellDuration           = 2 * time.Minute
	stopArrivalSnapMeters          = 50.0
	terminusArrivalToleranceMeters = 500.0
	oppositeHeadingTiePenaltyM     = 75.0
	maxInt32Value                  = 2147483647

	liveEstimatePastExactHorizon = 60 * time.Minute
	liveEstimatePastRouteHorizon = 30 * time.Minute
	liveEstimateFutureHorizon    = 2 * time.Hour
	liveEstimateMaxRouteDelta    = 60 * time.Minute
)

// EstimatedArrival describes a live estimated arrival time for an active vehicle at an upcoming stop.
type EstimatedArrival struct {
	TripID               int        `json:"trip_id"`
	RouteID              int        `json:"route_id"`
	RouteNumber          string     `json:"route_number"`
	RouteName            string     `json:"route_name"`
	Direction            string     `json:"direction"`
	StopID               int        `json:"stop_id"`
	StopName             string     `json:"stop_name"`
	StopSequence         int        `json:"stop_sequence"`
	VehicleID            int        `json:"vehicle_id"`
	VehicleNumber        string     `json:"vehicle_number"`
	VehicleStatus        string     `json:"vehicle_status"`
	ScheduledArrivalTime *time.Time `json:"scheduled_arrival_time"`
	EstimatedArrivalTime time.Time  `json:"estimated_arrival_time"`
	DelayMinutes         int        `json:"delay_minutes"`
	DistanceMeters       int        `json:"distance_meters"`
	IsSimulated          bool       `json:"is_simulated"`
	RecordedAt           time.Time  `json:"recorded_at"`
}

// TripStopPoint describes an ordered stop along a scheduled trip with coordinates and schedule offsets.
type TripStopPoint struct {
	TripID       int
	RouteID      int
	RouteNumber  string
	RouteName    string
	Direction    string
	RouteStopID  int
	StopID       int
	StopName     string
	StopSequence int
	Latitude     float64
	Longitude    float64
	ArrivalSec   int64
	DepartureSec int64
}

// RouteStopPoint describes an ordered stop along a route with coordinates for route-only active vehicles.
type RouteStopPoint struct {
	RouteID      int
	RouteNumber  string
	RouteName    string
	Direction    string
	RouteStopID  int
	StopID       int
	StopName     string
	StopSequence int
	Latitude     float64
	Longitude    float64
}

type projectedStopArrival struct {
	RouteStopID          int
	StopID               int
	StopName             string
	StopSequence         int
	ScheduledArrivalTime *time.Time
	EstimatedArrivalTime time.Time
	DelayMinutes         int
	DistanceMeters       int
}

// StopExists reports whether a stop exists for the given stop identifier.
func (p *PostgresRepository) StopExists(ctx context.Context, stopID int) error {
	if err := p.ensureDB(); err != nil {
		return err
	}
	var row struct{ ID int }
	return mapRecordError(p.db.WithContext(ctx).Table("stops").Select("id").Where("id = ?", stopID).Take(&row).Error)
}

const listActiveVehiclesForStopSQL = `
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
  AND EXISTS (
    SELECT 1
    FROM route_stops rs
    JOIN stops s ON s.id = rs.stop_id
    WHERE rs.route_id = vl.route_id
      AND rs.stop_id = ?
      AND s.is_active = TRUE
  )
ORDER BY r.number, r.direction, v.number`

// ListActiveVehiclesForStop loads fresh active vehicles on routes serving stopID (optionally filtered by routeID).
func (p *PostgresRepository) ListActiveVehiclesForStop(
	ctx context.Context,
	cutoff time.Time,
	stopID, routeID int,
) ([]ActiveVehicle, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	rows, err := p.db.WithContext(ctx).Raw(
		listActiveVehiclesForStopSQL,
		cutoff.UTC(),
		routeID, routeID,
		stopID,
	).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanActiveVehicleRows(rows)
}

const loadTripStopsSQL = `
SELECT t.id AS trip_id,
       t.route_id,
       r.number AS route_number,
       r.name AS route_name,
       r.direction,
       rs.id AS route_stop_id,
       s.id AS stop_id,
       s.name AS stop_name,
       rs.stop_sequence,
       s.latitude,
       s.longitude,
       CAST(EXTRACT(EPOCH FROM st.arrival_time) AS bigint) AS arrival_sec,
       CAST(EXTRACT(EPOCH FROM st.departure_time) AS bigint) AS departure_sec
FROM stop_times st
JOIN trips t ON t.id = st.trip_id AND t.route_id = st.route_id
JOIN routes r ON r.id = t.route_id
JOIN route_stops rs ON rs.id = st.route_stop_id AND rs.route_id = t.route_id
JOIN stops s ON s.id = rs.stop_id
WHERE t.id IN ?
  AND t.is_active = TRUE
  AND r.is_active = TRUE
  AND s.is_active = TRUE
ORDER BY t.id ASC, rs.stop_sequence ASC, rs.id ASC`

// LoadTripStops loads ordered stop times and coordinates for the requested active trip IDs.
func (p *PostgresRepository) LoadTripStops(ctx context.Context, tripIDs []int) ([]TripStopPoint, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	unique := uniqueSortedPositiveIDs(tripIDs)
	if len(unique) == 0 {
		return []TripStopPoint{}, nil
	}

	rows, err := p.db.WithContext(ctx).Raw(loadTripStopsSQL, unique).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stops := make([]TripStopPoint, 0)
	for rows.Next() {
		var item TripStopPoint
		if err := rows.Scan(
			&item.TripID,
			&item.RouteID,
			&item.RouteNumber,
			&item.RouteName,
			&item.Direction,
			&item.RouteStopID,
			&item.StopID,
			&item.StopName,
			&item.StopSequence,
			&item.Latitude,
			&item.Longitude,
			&item.ArrivalSec,
			&item.DepartureSec,
		); err != nil {
			return nil, err
		}
		stops = append(stops, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stops, nil
}

const loadRouteStopsSQL = `
SELECT r.id AS route_id,
       r.number AS route_number,
       r.name AS route_name,
       r.direction,
       rs.id AS route_stop_id,
       s.id AS stop_id,
       s.name AS stop_name,
       rs.stop_sequence,
       s.latitude,
       s.longitude
FROM route_stops rs
JOIN routes r ON r.id = rs.route_id
JOIN stops s ON s.id = rs.stop_id
WHERE rs.route_id IN ?
  AND r.is_active = TRUE
  AND s.is_active = TRUE
ORDER BY rs.route_id ASC, rs.stop_sequence ASC, rs.id ASC`

// LoadRouteStops loads ordered active stops and coordinates for the requested route IDs.
func (p *PostgresRepository) LoadRouteStops(ctx context.Context, routeIDs []int) ([]RouteStopPoint, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	unique := uniqueSortedPositiveIDs(routeIDs)
	if len(unique) == 0 {
		return []RouteStopPoint{}, nil
	}

	rows, err := p.db.WithContext(ctx).Raw(loadRouteStopsSQL, unique).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stops := make([]RouteStopPoint, 0)
	for rows.Next() {
		var item RouteStopPoint
		if err := rows.Scan(
			&item.RouteID,
			&item.RouteNumber,
			&item.RouteName,
			&item.Direction,
			&item.RouteStopID,
			&item.StopID,
			&item.StopName,
			&item.StopSequence,
			&item.Latitude,
			&item.Longitude,
		); err != nil {
			return nil, err
		}
		stops = append(stops, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stops, nil
}

// UpcomingArrivals returns live estimated arrivals for a trip and/or stop backed by fresh vehicle locations in PostgreSQL.
func (p *PostgresRepository) UpcomingArrivals(ctx context.Context, tripID, stopID int) ([]EstimatedArrival, error) {
	if err := p.ensureDB(); err != nil {
		return nil, err
	}
	return NewService(p).UpcomingArrivals(ctx, tripID, stopID)
}

// UpcomingArrivals computes live estimated arrival times for upcoming stops of an active trip and/or stop.
func (s *Service) UpcomingArrivals(ctx context.Context, tripID, stopID int) ([]EstimatedArrival, error) {
	return s.upcomingArrivals(ctx, tripID, stopID, 0)
}

// StopArrivals computes live estimated arrival times for active vehicles approaching stopID, optionally filtered by routeID.
func (s *Service) StopArrivals(ctx context.Context, stopID, routeID int) ([]EstimatedArrival, error) {
	if !isPositiveInt32(stopID) {
		return nil, &InvalidInput{Message: "stop_id must be a positive 32-bit integer"}
	}
	if routeID < 0 || int64(routeID) > maxInt32Value {
		return nil, &InvalidInput{Message: "route_id must be a positive 32-bit integer"}
	}
	return s.upcomingArrivals(ctx, 0, stopID, routeID)
}

func (s *Service) upcomingArrivals(ctx context.Context, tripID, stopID, routeID int) ([]EstimatedArrival, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("vehicle repository is not configured")
	}
	if tripID < 0 || int64(tripID) > maxInt32Value {
		return nil, &InvalidInput{Message: "trip_id must be a positive 32-bit integer"}
	}
	if stopID < 0 || int64(stopID) > maxInt32Value {
		return nil, &InvalidInput{Message: "stop_id must be a positive 32-bit integer"}
	}
	if routeID < 0 || int64(routeID) > maxInt32Value {
		return nil, &InvalidInput{Message: "route_id must be a positive 32-bit integer"}
	}
	if tripID == 0 && stopID == 0 {
		return nil, &InvalidInput{Message: "trip_id or stop_id must be a positive 32-bit integer"}
	}

	if tripID > 0 {
		if err := s.repo.TripExists(ctx, tripID); err != nil {
			return nil, err
		}
	}
	if stopID > 0 {
		if err := s.repo.StopExists(ctx, stopID); err != nil {
			return nil, err
		}
	}
	if routeID > 0 {
		if err := s.repo.RouteExists(ctx, routeID); err != nil {
			return nil, err
		}
	}

	nowUTC := s.now().UTC()
	cutoff := nowUTC.Add(-DefaultActiveMaxAge)

	var active []ActiveVehicle
	var err error
	if tripID > 0 {
		active, err = s.repo.ListActiveVehicles(ctx, cutoff, routeID, "", tripID)
	} else {
		active, err = s.repo.ListActiveVehiclesForStop(ctx, cutoff, stopID, routeID)
	}
	if err != nil {
		return nil, err
	}

	fresh := filterFreshActiveVehicles(active, cutoff)
	if len(fresh) == 0 {
		return []EstimatedArrival{}, nil
	}

	tripIDs := make([]int, 0, len(fresh))
	for _, v := range fresh {
		if v.TripID != nil && *v.TripID > 0 {
			tripIDs = append(tripIDs, *v.TripID)
		}
	}

	tripStopsByID := make(map[int][]TripStopPoint)
	if len(tripIDs) > 0 {
		loadedTripStops, err := s.repo.LoadTripStops(ctx, tripIDs)
		if err != nil {
			return nil, err
		}
		for _, pt := range loadedTripStops {
			tripStopsByID[pt.TripID] = append(tripStopsByID[pt.TripID], pt)
		}
	}

	routeIDsForFallback := make([]int, 0)
	for _, v := range fresh {
		if v.TripID == nil || len(tripStopsByID[*v.TripID]) == 0 {
			if tripID == 0 && v.RouteID > 0 {
				routeIDsForFallback = append(routeIDsForFallback, v.RouteID)
			}
		}
	}

	routeStopsByID := make(map[int][]RouteStopPoint)
	if len(routeIDsForFallback) > 0 {
		loadedRouteStops, err := s.repo.LoadRouteStops(ctx, routeIDsForFallback)
		if err != nil {
			return nil, err
		}
		for _, pt := range loadedRouteStops {
			routeStopsByID[pt.RouteID] = append(routeStopsByID[pt.RouteID], pt)
		}
	}

	loc := loadAgencyLocation()
	arrivals := make([]EstimatedArrival, 0)

	for _, v := range fresh {
		var projected []projectedStopArrival
		resolvedTripID := 0
		if v.TripID != nil && len(tripStopsByID[*v.TripID]) > 0 {
			resolvedTripID = *v.TripID
			projected = projectVehicleOntoTripStops(v, tripStopsByID[resolvedTripID], nowUTC, loc)
		} else if tripID == 0 && len(routeStopsByID[v.RouteID]) > 0 {
			projected = projectVehicleOntoRouteStops(v, routeStopsByID[v.RouteID], nowUTC)
		}

		for _, p := range projected {
			if stopID > 0 && p.StopID != stopID {
				continue
			}
			arrivals = append(arrivals, EstimatedArrival{
				TripID:               resolvedTripID,
				RouteID:              v.RouteID,
				RouteNumber:          v.RouteNumber,
				RouteName:            v.RouteName,
				Direction:            v.Direction,
				StopID:               p.StopID,
				StopName:             p.StopName,
				StopSequence:         p.StopSequence,
				VehicleID:            v.VehicleID,
				VehicleNumber:        v.VehicleNumber,
				VehicleStatus:        v.Status,
				ScheduledArrivalTime: p.ScheduledArrivalTime,
				EstimatedArrivalTime: p.EstimatedArrivalTime,
				DelayMinutes:         p.DelayMinutes,
				DistanceMeters:       p.DistanceMeters,
				IsSimulated:          v.IsSimulated,
				RecordedAt:           v.RecordedAt.UTC(),
			})
		}
	}

	if tripID > 0 && stopID == 0 {
		sort.Slice(arrivals, func(i, j int) bool {
			if arrivals[i].StopSequence != arrivals[j].StopSequence {
				return arrivals[i].StopSequence < arrivals[j].StopSequence
			}
			if !arrivals[i].EstimatedArrivalTime.Equal(arrivals[j].EstimatedArrivalTime) {
				return arrivals[i].EstimatedArrivalTime.Before(arrivals[j].EstimatedArrivalTime)
			}
			return arrivals[i].VehicleID < arrivals[j].VehicleID
		})
	} else {
		sort.Slice(arrivals, func(i, j int) bool {
			if !arrivals[i].EstimatedArrivalTime.Equal(arrivals[j].EstimatedArrivalTime) {
				return arrivals[i].EstimatedArrivalTime.Before(arrivals[j].EstimatedArrivalTime)
			}
			if arrivals[i].RouteNumber != arrivals[j].RouteNumber {
				return arrivals[i].RouteNumber < arrivals[j].RouteNumber
			}
			if arrivals[i].VehicleID != arrivals[j].VehicleID {
				return arrivals[i].VehicleID < arrivals[j].VehicleID
			}
			return arrivals[i].StopSequence < arrivals[j].StopSequence
		})
	}

	return arrivals, nil
}

// LiveEstimateProvider computes live departure/arrival estimates from active vehicle locations for transit schedules and journeys.
type LiveEstimateProvider struct {
	repo Repository
	now  func() time.Time
}

var _ transit.EstimateProvider = (*LiveEstimateProvider)(nil)

// NewLiveEstimateProvider creates an EstimateProvider backed by PostgreSQL vehicle_locations.
// When db is nil, Estimates returns an empty map so scheduled times remain intact with null estimates.
func NewLiveEstimateProvider(db *gorm.DB) *LiveEstimateProvider {
	if db == nil {
		return &LiveEstimateProvider{now: time.Now}
	}
	return NewLiveEstimateProviderWithRepository(NewPostgresRepository(db), time.Now)
}

// NewLiveEstimateProviderWithRepository creates an EstimateProvider using a custom repository and clock.
func NewLiveEstimateProviderWithRepository(repo Repository, now func() time.Time) *LiveEstimateProvider {
	if now == nil {
		now = time.Now
	}
	return &LiveEstimateProvider{repo: repo, now: now}
}

// Estimates returns live estimated departure/arrival timestamps for departures with approaching active vehicles.
// Missing keys preserve null estimates; estimates are never fabricated without a recent vehicle_location row.
func (p *LiveEstimateProvider) Estimates(ctx context.Context, departures []transit.Departure) (map[transit.DepartureKey]time.Time, error) {
	result := make(map[transit.DepartureKey]time.Time)
	if p == nil || p.repo == nil || len(departures) == 0 {
		return result, nil
	}

	nowUTC := p.now().UTC()
	cutoff := nowUTC.Add(-DefaultActiveMaxAge)

	active, err := p.repo.ListActiveVehicles(ctx, cutoff, 0, "", 0)
	if err != nil {
		return nil, err
	}
	fresh := filterFreshActiveVehicles(active, cutoff)
	if len(fresh) == 0 {
		return result, nil
	}

	depTripIDs := make(map[int]struct{}, len(departures))
	depRouteIDs := make(map[int]struct{}, len(departures))
	for _, d := range departures {
		if d.TripID > 0 {
			depTripIDs[d.TripID] = struct{}{}
		}
		if d.Route.ID > 0 {
			depRouteIDs[d.Route.ID] = struct{}{}
		}
	}

	relevant := make([]ActiveVehicle, 0, len(fresh))
	tripIDsToLoad := make([]int, 0)
	for _, v := range fresh {
		matchesTrip := v.TripID != nil && hasIntKey(depTripIDs, *v.TripID)
		matchesRoute := hasIntKey(depRouteIDs, v.RouteID)
		if !matchesTrip && !matchesRoute {
			continue
		}
		relevant = append(relevant, v)
		if v.TripID != nil && *v.TripID > 0 {
			tripIDsToLoad = append(tripIDsToLoad, *v.TripID)
		}
	}
	if len(relevant) == 0 {
		return result, nil
	}

	tripStopsByID := make(map[int][]TripStopPoint)
	if len(tripIDsToLoad) > 0 {
		loadedTripStops, err := p.repo.LoadTripStops(ctx, tripIDsToLoad)
		if err != nil {
			return nil, err
		}
		for _, pt := range loadedTripStops {
			tripStopsByID[pt.TripID] = append(tripStopsByID[pt.TripID], pt)
		}
	}

	routeIDsToLoad := make([]int, 0)
	for _, v := range relevant {
		if v.TripID == nil || len(tripStopsByID[*v.TripID]) == 0 {
			if v.RouteID > 0 {
				routeIDsToLoad = append(routeIDsToLoad, v.RouteID)
			}
		}
	}

	routeStopsByID := make(map[int][]RouteStopPoint)
	if len(routeIDsToLoad) > 0 {
		loadedRouteStops, err := p.repo.LoadRouteStops(ctx, routeIDsToLoad)
		if err != nil {
			return nil, err
		}
		for _, pt := range loadedRouteStops {
			routeStopsByID[pt.RouteID] = append(routeStopsByID[pt.RouteID], pt)
		}
	}

	type vehicleProjection struct {
		vehicle   ActiveVehicle
		projected []projectedStopArrival
	}

	loc := loadAgencyLocation()
	byTrip := make(map[int][]vehicleProjection)
	byRoute := make(map[int][]vehicleProjection)

	for _, v := range relevant {
		var proj []projectedStopArrival
		if v.TripID != nil && len(tripStopsByID[*v.TripID]) > 0 {
			proj = projectVehicleOntoTripStops(v, tripStopsByID[*v.TripID], nowUTC, loc)
		} else if len(routeStopsByID[v.RouteID]) > 0 {
			proj = projectVehicleOntoRouteStops(v, routeStopsByID[v.RouteID], nowUTC)
		}
		if len(proj) == 0 {
			continue
		}
		vp := vehicleProjection{vehicle: v, projected: proj}
		if v.TripID != nil && *v.TripID > 0 {
			byTrip[*v.TripID] = append(byTrip[*v.TripID], vp)
		}
		if v.RouteID > 0 {
			byRoute[v.RouteID] = append(byRoute[v.RouteID], vp)
		}
	}

	matchedVehicles := make(map[int]bool)
	claimedKeys := make(map[transit.DepartureKey]bool)

	// Pass 1: exact trip_id match within the live horizon.
	for _, d := range departures {
		if !withinLiveHorizon(d.ScheduledDeparture, nowUTC, liveEstimatePastExactHorizon, liveEstimateFutureHorizon, true) {
			continue
		}
		candidates := byTrip[d.TripID]
		if len(candidates) == 0 {
			continue
		}
		var bestETA time.Time
		bestVehicleID := 0
		found := false
		for _, vp := range candidates {
			eta, ok := lookupProjectedETA(vp.projected, d.RouteStopID, d.StopID, d.StopSequence)
			if !ok {
				continue
			}
			if !found || eta.Before(bestETA) {
				bestETA = eta
				bestVehicleID = vp.vehicle.VehicleID
				found = true
			}
		}
		if found {
			key := transit.DepartureKey{TripID: d.TripID, RouteStopID: d.RouteStopID, ServiceDate: d.ServiceDate}
			result[key] = bestETA
			claimedKeys[key] = true
			matchedVehicles[bestVehicleID] = true
		}
	}

	// Pass 2: pair remaining active vehicles on a route to the nearest imminent unclaimed trip occurrence on that route.
	type tripInstanceKey struct {
		RouteID     int
		TripID      int
		ServiceDate string
	}
	routeInstances := make(map[int]map[tripInstanceKey][]transit.Departure)
	for _, d := range departures {
		if d.Route.ID <= 0 {
			continue
		}
		key := transit.DepartureKey{TripID: d.TripID, RouteStopID: d.RouteStopID, ServiceDate: d.ServiceDate}
		if claimedKeys[key] {
			continue
		}
		if !withinLiveHorizon(d.ScheduledDeparture, nowUTC, liveEstimatePastRouteHorizon, liveEstimateFutureHorizon, false) {
			continue
		}
		if routeInstances[d.Route.ID] == nil {
			routeInstances[d.Route.ID] = make(map[tripInstanceKey][]transit.Departure)
		}
		instKey := tripInstanceKey{RouteID: d.Route.ID, TripID: d.TripID, ServiceDate: d.ServiceDate}
		routeInstances[d.Route.ID][instKey] = append(routeInstances[d.Route.ID][instKey], d)
	}

	type routePairCandidate struct {
		vehicleIdx int
		instKey    tripInstanceKey
		avgDelta   time.Duration
		earliest   time.Time
		matches    map[transit.DepartureKey]time.Time
	}

	routeIDs := make([]int, 0, len(routeInstances))
	for routeID := range routeInstances {
		routeIDs = append(routeIDs, routeID)
	}
	sort.Ints(routeIDs)

	for _, routeID := range routeIDs {
		vehiclesOnRoute := byRoute[routeID]
		if len(vehiclesOnRoute) == 0 {
			continue
		}
		instances := routeInstances[routeID]
		candidates := make([]routePairCandidate, 0)

		for vIdx, vp := range vehiclesOnRoute {
			if matchedVehicles[vp.vehicle.VehicleID] {
				continue
			}
			for instKey, deps := range instances {
				matches := make(map[transit.DepartureKey]time.Time, len(deps))
				var totalDelta time.Duration
				var earliest time.Time
				valid := true
				for _, d := range deps {
					eta, ok := lookupProjectedETA(vp.projected, d.RouteStopID, d.StopID, d.StopSequence)
					if !ok {
						continue
					}
					delta := absDuration(d.ScheduledDeparture.Sub(eta))
					if delta > liveEstimateMaxRouteDelta {
						valid = false
						break
					}
					dKey := transit.DepartureKey{TripID: d.TripID, RouteStopID: d.RouteStopID, ServiceDate: d.ServiceDate}
					matches[dKey] = eta
					totalDelta += delta
					if earliest.IsZero() || d.ScheduledDeparture.Before(earliest) {
						earliest = d.ScheduledDeparture
					}
				}
				if !valid || len(matches) == 0 {
					continue
				}
				candidates = append(candidates, routePairCandidate{
					vehicleIdx: vIdx,
					instKey:    instKey,
					avgDelta:   totalDelta / time.Duration(len(matches)),
					earliest:   earliest,
					matches:    matches,
				})
			}
		}

		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].avgDelta != candidates[j].avgDelta {
				return candidates[i].avgDelta < candidates[j].avgDelta
			}
			if !candidates[i].earliest.Equal(candidates[j].earliest) {
				return candidates[i].earliest.Before(candidates[j].earliest)
			}
			if candidates[i].instKey.TripID != candidates[j].instKey.TripID {
				return candidates[i].instKey.TripID < candidates[j].instKey.TripID
			}
			return vehiclesOnRoute[candidates[i].vehicleIdx].vehicle.VehicleID < vehiclesOnRoute[candidates[j].vehicleIdx].vehicle.VehicleID
		})

		usedVehicle := make(map[int]bool)
		usedInstance := make(map[tripInstanceKey]bool)
		for _, c := range candidates {
			if usedVehicle[c.vehicleIdx] || usedInstance[c.instKey] {
				continue
			}
			usedVehicle[c.vehicleIdx] = true
			usedInstance[c.instKey] = true
			for k, eta := range c.matches {
				result[k] = eta
			}
		}
	}

	return result, nil
}

func projectVehicleOntoTripStops(
	v ActiveVehicle,
	stops []TripStopPoint,
	now time.Time,
	loc *time.Location,
) []projectedStopArrival {
	if len(stops) == 0 {
		return nil
	}
	coords := make([]Coordinate, len(stops))
	for i, s := range stops {
		coords[i] = Coordinate{Lat: s.Latitude, Lng: s.Longitude}
	}
	firstIdx, distToFirst, ok := projectPointOntoStopCoords(v, coords)
	if !ok || firstIdx >= len(stops) {
		return nil
	}

	refTime := v.RecordedAt.UTC()
	if refTime.IsZero() {
		refTime = now.UTC()
	}
	year, month, day := resolveTripServiceDay(refTime, stops[firstIdx].ArrivalSec, loc)

	out := make([]projectedStopArrival, 0, len(stops)-firstIdx)
	cumDist := distToFirst
	for i := firstIdx; i < len(stops); i++ {
		if i > firstIdx {
			cumDist += haversineMeters(coords[i-1], coords[i])
		}
		eta := estimateArrivalTime(v, cumDist, now)
		sched := serviceDayTimestamp(year, month, day, stops[i].ArrivalSec, loc)
		schedCopy := sched
		delayMinutes := int(math.Round(eta.Sub(sched).Minutes()))
		out = append(out, projectedStopArrival{
			RouteStopID:          stops[i].RouteStopID,
			StopID:               stops[i].StopID,
			StopName:             stops[i].StopName,
			StopSequence:         stops[i].StopSequence,
			ScheduledArrivalTime: &schedCopy,
			EstimatedArrivalTime: eta,
			DelayMinutes:         delayMinutes,
			DistanceMeters:       int(math.Round(cumDist)),
		})
	}
	return out
}

func projectVehicleOntoRouteStops(
	v ActiveVehicle,
	stops []RouteStopPoint,
	now time.Time,
) []projectedStopArrival {
	if len(stops) == 0 {
		return nil
	}
	coords := make([]Coordinate, len(stops))
	for i, s := range stops {
		coords[i] = Coordinate{Lat: s.Latitude, Lng: s.Longitude}
	}
	firstIdx, distToFirst, ok := projectPointOntoStopCoords(v, coords)
	if !ok || firstIdx >= len(stops) {
		return nil
	}

	out := make([]projectedStopArrival, 0, len(stops)-firstIdx)
	cumDist := distToFirst
	for i := firstIdx; i < len(stops); i++ {
		if i > firstIdx {
			cumDist += haversineMeters(coords[i-1], coords[i])
		}
		eta := estimateArrivalTime(v, cumDist, now)
		out = append(out, projectedStopArrival{
			RouteStopID:          stops[i].RouteStopID,
			StopID:               stops[i].StopID,
			StopName:             stops[i].StopName,
			StopSequence:         stops[i].StopSequence,
			ScheduledArrivalTime: nil,
			EstimatedArrivalTime: eta,
			DelayMinutes:         0,
			DistanceMeters:       int(math.Round(cumDist)),
		})
	}
	return out
}

func projectPointOntoStopCoords(v ActiveVehicle, coords []Coordinate) (int, float64, bool) {
	n := len(coords)
	if n == 0 {
		return 0, 0, false
	}
	pos := Coordinate{Lat: v.Latitude, Lng: v.Longitude}
	if n == 1 {
		return 0, haversineMeters(pos, coords[0]), true
	}

	bestSeg := 0
	bestClampedT := 0.0
	bestRawT := 0.0
	bestScore := math.Inf(1)

	useHeading := v.Heading != nil && isFiniteInRange(*v.Heading, 0, 360) &&
		v.SpeedKmh != nil && !math.IsNaN(*v.SpeedKmh) && !math.IsInf(*v.SpeedKmh, 0) && *v.SpeedKmh > 2.0

	for i := 0; i < n-1; i++ {
		a, b := coords[i], coords[i+1]
		cosLat := math.Cos(a.Lat * math.Pi / 180.0)
		bx := (b.Lng - a.Lng) * cosLat * 111320.0
		by := (b.Lat - a.Lat) * 111320.0
		vx := (pos.Lng - a.Lng) * cosLat * 111320.0
		vy := (pos.Lat - a.Lat) * 111320.0

		segLenSq := bx*bx + by*by
		rawT := 0.0
		if segLenSq > 1e-6 {
			rawT = (vx*bx + vy*by) / segLenSq
		}
		clampedT := math.Max(0, math.Min(1, rawT))
		proj := Coordinate{
			Lat: a.Lat + (b.Lat-a.Lat)*clampedT,
			Lng: a.Lng + (b.Lng-a.Lng)*clampedT,
		}
		score := haversineMeters(pos, proj)
		if useHeading && segLenSq > 1.0 {
			if angularDiffDegrees(*v.Heading, bearingDegrees(a, b)) > 110.0 {
				score += oppositeHeadingTiePenaltyM
			}
		}

		if score < bestScore {
			bestScore = score
			bestSeg = i
			bestClampedT = clampedT
			bestRawT = rawT
		}
	}

	if bestSeg == 0 && bestRawT <= 0.05 {
		return 0, haversineMeters(pos, coords[0]), true
	}
	if bestClampedT <= 0.05 && haversineMeters(pos, coords[bestSeg]) <= stopArrivalSnapMeters {
		return bestSeg, haversineMeters(pos, coords[bestSeg]), true
	}
	if bestSeg == n-2 && bestRawT > 1.0 {
		distToLast := haversineMeters(pos, coords[n-1])
		if distToLast <= terminusArrivalToleranceMeters {
			return n - 1, distToLast, true
		}
		return 0, 0, false
	}

	firstIdx := bestSeg + 1
	return firstIdx, haversineMeters(pos, coords[firstIdx]), true
}

func effectiveSpeedKmh(speed *float64) float64 {
	if speed == nil || math.IsNaN(*speed) || math.IsInf(*speed, 0) || *speed <= 0 {
		return defaultTransitSpeedKmh
	}
	if *speed < minTransitSpeedKmh {
		return minTransitSpeedKmh
	}
	if *speed > maxTransitSpeedKmh {
		return maxTransitSpeedKmh
	}
	return *speed
}

func estimateArrivalTime(v ActiveVehicle, distanceMeters float64, now time.Time) time.Time {
	if distanceMeters < 0 {
		distanceMeters = 0
	}
	speedMetersPerSec := effectiveSpeedKmh(v.SpeedKmh) * 1000.0 / 3600.0
	travelSeconds := distanceMeters / speedMetersPerSec
	if v.Status == StatusDelayed {
		travelSeconds *= delayedTravelFactor
	}
	travelDur := time.Duration(math.Round(travelSeconds)) * time.Second
	if v.Status == StatusLayover {
		travelDur += layoverDwellDuration
	}

	base := v.RecordedAt.UTC()
	nowUTC := now.UTC()
	if base.IsZero() {
		base = nowUTC
	}
	eta := base.Add(travelDur)
	if eta.Before(nowUTC) {
		eta = nowUTC
	}
	return eta.UTC().Truncate(time.Second)
}

func loadAgencyLocation() *time.Location {
	loc, err := time.LoadLocation(transit.AgencyTimezone)
	if err != nil {
		return time.FixedZone("EST", -5*3600)
	}
	return loc
}

func resolveTripServiceDay(refTime time.Time, anchorArrivalSec int64, loc *time.Location) (int, time.Month, int) {
	refLocal := refTime.In(loc)
	bestYear, bestMonth, bestDay := refLocal.Year(), refLocal.Month(), refLocal.Day()
	bestDiff := time.Duration(math.MaxInt64)

	for offset := -1; offset <= 1; offset++ {
		dayDate := time.Date(refLocal.Year(), refLocal.Month(), refLocal.Day()+offset, 0, 0, 0, 0, loc)
		candidate := serviceDayTimestamp(dayDate.Year(), dayDate.Month(), dayDate.Day(), anchorArrivalSec, loc)
		diff := absDuration(candidate.Sub(refTime))
		if diff < bestDiff {
			bestDiff = diff
			bestYear, bestMonth, bestDay = dayDate.Year(), dayDate.Month(), dayDate.Day()
		}
	}
	return bestYear, bestMonth, bestDay
}

func serviceDayTimestamp(year int, month time.Month, day int, offsetSec int64, loc *time.Location) time.Time {
	extraDays := int(offsetSec / 86400)
	rem := int(offsetSec % 86400)
	if rem < 0 {
		extraDays--
		rem += 86400
	}
	hour := rem / 3600
	minute := (rem % 3600) / 60
	second := rem % 60
	return time.Date(year, month, day+extraDays, hour, minute, second, 0, loc).UTC()
}

func lookupProjectedETA(projected []projectedStopArrival, routeStopID, stopID, stopSequence int) (time.Time, bool) {
	if routeStopID > 0 {
		for _, p := range projected {
			if p.RouteStopID == routeStopID {
				return p.EstimatedArrivalTime, true
			}
		}
	}
	if stopID > 0 && stopSequence > 0 {
		for _, p := range projected {
			if p.StopID == stopID && p.StopSequence == stopSequence {
				return p.EstimatedArrivalTime, true
			}
		}
	}
	if stopID > 0 {
		for _, p := range projected {
			if p.StopID == stopID {
				return p.EstimatedArrivalTime, true
			}
		}
	}
	return time.Time{}, false
}

func filterFreshActiveVehicles(active []ActiveVehicle, cutoff time.Time) []ActiveVehicle {
	fresh := make([]ActiveVehicle, 0, len(active))
	for _, v := range active {
		if v.Status == StatusOutOfService {
			continue
		}
		if !v.RecordedAt.IsZero() && v.RecordedAt.Before(cutoff) {
			continue
		}
		fresh = append(fresh, v)
	}
	return fresh
}

func withinLiveHorizon(scheduled, now time.Time, maxPast, maxFuture time.Duration, allowZero bool) bool {
	if scheduled.IsZero() {
		return allowZero
	}
	return !scheduled.Before(now.Add(-maxPast)) && !scheduled.After(now.Add(maxFuture))
}

func angularDiffDegrees(a, b float64) float64 {
	diff := math.Abs(math.Mod(a-b+540.0, 360.0) - 180.0)
	return diff
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func isPositiveInt32(id int) bool {
	return id > 0 && int64(id) <= maxInt32Value
}

func hasIntKey(set map[int]struct{}, key int) bool {
	_, ok := set[key]
	return ok
}

func uniqueSortedPositiveIDs(ids []int) []int {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}
