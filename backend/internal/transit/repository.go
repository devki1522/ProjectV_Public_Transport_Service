package transit

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

// PostgresRepository reads transit schedule data from PostgreSQL.
type PostgresRepository struct{ db *gorm.DB }

// NewPostgresRepository creates a PostgreSQL-backed schedule repository.
func NewPostgresRepository(db *gorm.DB) *PostgresRepository { return &PostgresRepository{db: db} }

func resourceError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// Route loads one route by identifier.
func (p *PostgresRepository) Route(ctx context.Context, id int) (Route, error) {
	var route Route
	err := p.db.WithContext(ctx).Table("routes").Where("id = ?", id).Take(&route).Error
	return route, resourceError(err)
}

// Stop loads stop details and associated departures.
func (p *PostgresRepository) Stop(ctx context.Context, id int) (Stop, error) {
	var stop Stop
	err := p.db.WithContext(ctx).Table("stops").Where("id = ?", id).Take(&stop).Error
	return stop, resourceError(err)
}

// TripExists checks whether a trip identifier exists.
func (p *PostgresRepository) TripExists(ctx context.Context, id int) error {
	var trip struct{ ID int }
	return resourceError(p.db.WithContext(ctx).Table("trips").Select("id").Where("id = ?", id).Take(&trip).Error)
}

// Convert local service-day intervals to absolute timestamps in PostgreSQL, rather
// than parsing its configurable interval text format or using the server timezone.
const timetableSQL = `
SELECT t.id AS trip_id, t.schedule_type, rs.id AS route_stop_id,
       s.id AS stop_id, s.name AS stop_name, rs.stop_sequence,
       (CAST(@date AS date) + st.arrival_time) AT TIME ZONE @zone AS scheduled_arrival,
       (CAST(@date AS date) + st.departure_time) AT TIME ZONE @zone AS scheduled_departure
FROM trips t
JOIN routes r ON r.id = t.route_id
LEFT JOIN stop_times st ON st.trip_id = t.id AND st.route_id = t.route_id
LEFT JOIN route_stops rs ON rs.id = st.route_stop_id AND rs.route_id = t.route_id
LEFT JOIN stops s ON s.id = rs.stop_id
WHERE t.route_id = @route_id AND r.is_active AND t.is_active
  AND (t.schedule_type = 'daily' OR t.schedule_type =
       CASE WHEN EXTRACT(ISODOW FROM CAST(@date AS date)) IN (6, 7) THEN 'weekend' ELSE 'weekday' END)
ORDER BY MIN(st.departure_time) OVER (PARTITION BY t.id) NULLS LAST, t.id, rs.stop_sequence`

// Timetable returns scheduled trips for a route and service date.
func (p *PostgresRepository) Timetable(ctx context.Context, routeID int, date string) ([]ScheduledTrip, error) {
	var rows []struct {
		TripID                               int
		ScheduleType                         string
		RouteStopID                          *int
		StopID                               int
		StopName                             string
		StopSequence                         int
		ScheduledArrival, ScheduledDeparture time.Time
	}
	err := p.db.WithContext(ctx).Raw(timetableSQL, map[string]any{
		"date": date, "zone": AgencyTimezone, "route_id": routeID,
	}).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	trips := make([]ScheduledTrip, 0)
	for _, row := range rows {
		if len(trips) == 0 || trips[len(trips)-1].ID != row.TripID {
			trips = append(trips, ScheduledTrip{ID: row.TripID, ScheduleType: row.ScheduleType, Status: "scheduled", StopTimes: []StopTime{}})
		}
		if row.RouteStopID != nil {
			trip := &trips[len(trips)-1]
			trip.StopTimes = append(trip.StopTimes, StopTime{
				RouteStopID: *row.RouteStopID, StopID: row.StopID, StopName: row.StopName, StopSequence: row.StopSequence,
				ScheduledArrival: row.ScheduledArrival, ScheduledDeparture: row.ScheduledDeparture,
			})
		}
	}
	return trips, nil
}

// Candidate service dates are derived for each interval, so 24:xx (and longer)
// stop times remain associated with the preceding service day. The final window
// comparison uses absolute instants, including at daylight-saving transitions.
const departuresSQL = `
WITH departures AS (
 SELECT t.id AS trip_id, r.id AS route_id, r.number, r.name, r.direction, r.is_active,
        rs.id AS route_stop_id, s.id AS stop_id, s.name AS stop_name, rs.stop_sequence,
        TO_CHAR(days.service_day, 'YYYY-MM-DD') AS service_date,
        (days.service_day + st.departure_time) AT TIME ZONE @zone AS scheduled_departure
 FROM stop_times st
 JOIN trips t ON t.id = st.trip_id AND t.route_id = st.route_id
 JOIN routes r ON r.id = t.route_id
 JOIN route_stops rs ON rs.id = st.route_stop_id AND rs.route_id = t.route_id
 JOIN stops s ON s.id = rs.stop_id
 CROSS JOIN LATERAL generate_series(
   CAST((CAST(@from AS timestamptz) AT TIME ZONE @zone - st.departure_time) AS date)::timestamp,
   CAST((CAST(@until AS timestamptz) AT TIME ZONE @zone - st.departure_time) AS date)::timestamp,
   INTERVAL '1 day') AS days(service_day)
 WHERE r.is_active AND t.is_active AND s.is_active
   AND (t.schedule_type = 'daily' OR t.schedule_type =
        CASE WHEN EXTRACT(ISODOW FROM days.service_day) IN (6, 7) THEN 'weekend' ELSE 'weekday' END)
   %s
)
SELECT * FROM departures
WHERE scheduled_departure >= @from AND scheduled_departure < @until
ORDER BY scheduled_departure, route_id, trip_id, stop_sequence, route_stop_id
LIMIT @limit`

// Departures returns departures matching the requested filters and time window.
func (p *PostgresRepository) Departures(ctx context.Context, q DepartureQuery) ([]Departure, error) {
	filters := make([]string, 0, 3)
	args := map[string]any{"from": q.From, "until": q.Until, "zone": AgencyTimezone, "limit": q.Limit}
	for _, filter := range []struct {
		column, name string
		id           int
	}{
		{"s.id", "stop_id", q.StopID}, {"r.id", "route_id", q.RouteID}, {"t.id", "trip_id", q.TripID},
	} {
		if filter.id != 0 {
			filters = append(filters, "AND "+filter.column+" = @"+filter.name)
			args[filter.name] = filter.id
		}
	}
	// Only fixed predicates are inserted; every caller-supplied value is bound.
	query := strings.Replace(departuresSQL, "%s", strings.Join(filters, " "), 1)
	var rows []struct {
		TripID, RouteID                   int
		Number, Name, Direction           string
		IsActive                          bool
		RouteStopID, StopID, StopSequence int
		StopName, ServiceDate             string
		ScheduledDeparture                time.Time
	}
	if err := p.db.WithContext(ctx).Raw(query, args).Scan(&rows).Error; err != nil {
		return nil, err
	}
	departures := make([]Departure, 0, len(rows))
	for _, row := range rows {
		departures = append(departures, Departure{
			TripID: row.TripID, Route: Route{ID: row.RouteID, Number: row.Number, Name: row.Name, Direction: row.Direction, IsActive: row.IsActive},
			RouteStopID: row.RouteStopID, StopID: row.StopID, StopName: row.StopName, StopSequence: row.StopSequence,
			ServiceDate: row.ServiceDate, ScheduledDeparture: row.ScheduledDeparture,
		})
	}
	return departures, nil
}

// SearchStops returns active stops matching an optional case-insensitive name filter.
func (p *PostgresRepository) SearchStops(ctx context.Context, q StopSearchQuery) ([]Stop, error) {
	var stops []Stop
	tx := p.db.WithContext(ctx).Table("stops").Where("is_active = ?", true)
	if q.Query != "" {
		tx = tx.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(q.Query)+"%")
	}
	if err := tx.Order("name ASC, id ASC").Limit(q.Limit).Find(&stops).Error; err != nil {
		return nil, err
	}
	return stops, nil
}

const activeStopsSQL = `
SELECT id, name, latitude, longitude, wheelchair_boarding, is_active
FROM stops
WHERE is_active = true`

const activeRoutesSQL = `
SELECT id, number, name, direction, is_active
FROM routes
WHERE is_active = true`

const activeScheduleStopTimesSQL = `
SELECT t.id AS trip_id, t.route_id, t.schedule_type,
       rs.id AS route_stop_id, rs.stop_id, rs.stop_sequence,
       CAST(EXTRACT(EPOCH FROM st.arrival_time) AS bigint) AS arrival_sec,
       CAST(EXTRACT(EPOCH FROM st.departure_time) AS bigint) AS departure_sec
FROM stop_times st
JOIN trips t ON t.id = st.trip_id AND t.route_id = st.route_id
JOIN routes r ON r.id = t.route_id
JOIN route_stops rs ON rs.id = st.route_stop_id AND rs.route_id = st.route_id
JOIN stops s ON s.id = rs.stop_id
WHERE r.is_active AND t.is_active AND s.is_active
  AND t.schedule_type IN @schedule_types
ORDER BY t.route_id, t.id, rs.stop_sequence`

const activeDisruptionsSQL = `
SELECT d.id, d.type, d.title, d.start_time, d.end_time, d.status,
       dr.route_id, dr.stop_id
FROM disruptions d
JOIN disruption_routes dr ON dr.disruption_id = d.id
WHERE d.status IN ('active', 'scheduled')
  AND d.type IN ('cancellation', 'stop_closure', 'road_closure', 'emergency', 'detour')
  AND d.start_time <= @window_end
  AND (d.end_time IS NULL OR d.end_time >= @window_start)
ORDER BY d.start_time, d.id`

func minutesBetween(start, end time.Time) int {
	if !end.After(start) {
		return 0
	}
	return int(math.Round(end.Sub(start).Minutes()))
}

func (p *PostgresRepository) loadActiveStops(ctx context.Context) ([]Stop, error) {
	rows, err := p.db.WithContext(ctx).Raw(activeStopsSQL).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stops []Stop
	for rows.Next() {
		var stop Stop
		if err := rows.Scan(&stop.ID, &stop.Name, &stop.Latitude, &stop.Longitude, &stop.WheelchairBoarding, &stop.IsActive); err != nil {
			return nil, err
		}
		stops = append(stops, stop)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stops, nil
}

func (p *PostgresRepository) loadActiveRoutes(ctx context.Context) ([]Route, error) {
	rows, err := p.db.WithContext(ctx).Raw(activeRoutesSQL).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routes []Route
	for rows.Next() {
		var route Route
		if err := rows.Scan(&route.ID, &route.Number, &route.Name, &route.Direction, &route.IsActive); err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return routes, nil
}

func (p *PostgresRepository) loadActiveStopTimes(ctx context.Context, scheduleTypes []string) ([]JourneyStopTimeRow, error) {
	rows, err := p.db.WithContext(ctx).Raw(activeScheduleStopTimesSQL, map[string]any{"schedule_types": scheduleTypes}).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stopTimes []JourneyStopTimeRow
	for rows.Next() {
		var row JourneyStopTimeRow
		if err := rows.Scan(&row.TripID, &row.RouteID, &row.ScheduleType, &row.RouteStopID, &row.StopID, &row.StopSequence, &row.ArrivalSec, &row.DepartureSec); err != nil {
			return nil, err
		}
		stopTimes = append(stopTimes, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stopTimes, nil
}

func (p *PostgresRepository) loadActiveDisruptions(ctx context.Context, from, until time.Time) ([]JourneyDisruption, error) {
	rows, err := p.db.WithContext(ctx).Raw(activeDisruptionsSQL, map[string]any{
		"window_start": from,
		"window_end":   until.Add(maxJourneyDuration),
	}).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var disruptions []JourneyDisruption
	for rows.Next() {
		var d JourneyDisruption
		if err := rows.Scan(&d.ID, &d.Type, &d.Title, &d.StartTime, &d.EndTime, &d.Status, &d.RouteID, &d.StopID); err != nil {
			return nil, err
		}
		disruptions = append(disruptions, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return disruptions, nil
}

func (p *PostgresRepository) loadJourneyNetwork(ctx context.Context, q JourneyRepoQuery) (JourneyNetwork, error) {
	stops, err := p.loadActiveStops(ctx)
	if err != nil {
		return JourneyNetwork{}, err
	}
	routes, err := p.loadActiveRoutes(ctx)
	if err != nil {
		return JourneyNetwork{}, err
	}
	stopTimes, err := p.loadActiveStopTimes(ctx, neededScheduleTypes(q.From, q.Until))
	if err != nil {
		return JourneyNetwork{}, err
	}
	disruptions, err := p.loadActiveDisruptions(ctx, q.From, q.Until)
	if err != nil {
		return JourneyNetwork{}, err
	}
	return JourneyNetwork{Stops: stops, Routes: routes, StopTimes: stopTimes, Disruptions: disruptions}, nil
}

// Journeys plans direct, multi-transfer, and walking-connected itineraries while excluding disrupted trips and stops.
func (p *PostgresRepository) Journeys(ctx context.Context, q JourneyRepoQuery) (JourneyPlan, error) {
	if len(q.OriginStopIDs) == 0 || len(q.DestinationStopIDs) == 0 || q.Limit <= 0 {
		return JourneyPlan{Itineraries: []Itinerary{}}, nil
	}
	network, err := p.loadJourneyNetwork(ctx, q)
	if err != nil {
		return JourneyPlan{}, err
	}
	return PlanJourneysDetailed(network, q), nil
}
