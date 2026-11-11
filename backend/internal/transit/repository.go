package transit

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

type PostgresRepository struct{ db *gorm.DB }

func NewPostgresRepository(db *gorm.DB) *PostgresRepository { return &PostgresRepository{db: db} }

func resourceError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func (p *PostgresRepository) Route(ctx context.Context, id int) (Route, error) {
	var route Route
	err := p.db.WithContext(ctx).Table("routes").Where("id = ?", id).Take(&route).Error
	return route, resourceError(err)
}

func (p *PostgresRepository) Stop(ctx context.Context, id int) (Stop, error) {
	var stop Stop
	err := p.db.WithContext(ctx).Table("stops").Where("id = ?", id).Take(&stop).Error
	return stop, resourceError(err)
}

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
