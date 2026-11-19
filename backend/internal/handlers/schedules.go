// Package handlers translates transit service results into HTTP responses.
package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/transit"
	"github.com/go-chi/chi/v5"
)

// ScheduleService defines the schedule operations required by HTTP handlers.
type ScheduleService interface {
	Timetable(context.Context, int, string) (transit.Timetable, error)
	Departures(context.Context, transit.DepartureQuery) ([]transit.Departure, error)
	Stop(context.Context, int, transit.DepartureQuery) (transit.StopDetails, error)
	SearchStops(context.Context, transit.StopSearchQuery) ([]transit.Stop, error)
	Journeys(context.Context, transit.JourneyQuery) (transit.JourneySearchResult, error)
}

// ScheduleHandler serves timetable, departure, and stop HTTP requests.
type ScheduleHandler struct {
	service ScheduleService
	now     func() time.Time
}

// NewScheduleHandler creates a schedule HTTP handler for the given service.
func NewScheduleHandler(service ScheduleService) *ScheduleHandler {
	return &ScheduleHandler{service: service, now: time.Now}
}

// Timetable returns scheduled trips for a route and service date.
// @Tags schedules
// @Produce json
// @Summary Get a route timetable
// @Param id path int true "Positive route ID" minimum(1)
// @Param date query string true "Agency-local service date (YYYY-MM-DD)" example(2026-11-10)
// @Success 200 {object} transit.Timetable
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/routes/{id}/schedule [get]
func (h *ScheduleHandler) Timetable(w http.ResponseWriter, r *http.Request) {
	id, ok := transitRouteID(w, r)
	if !ok {
		return
	}
	values, err := scheduleQuery(r, "date")
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	result, err := h.service.Timetable(r.Context(), id, values.Get("date"))
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// Departures returns departures matching the requested filters and time window.
// @Tags schedules
// @Produce json
// @Summary Get upcoming departures
// @Description Filters combine with AND. Path IDs fix their filter; conflicting query IDs are rejected. Departure windows are [from, until), at most 24 hours. /api/departures requires at least one ID filter.
// @Param stop_id query int false "Positive stop_id" minimum(1)
// @Param route_id query int false "Positive route_id" minimum(1)
// @Param trip_id query int false "Positive trip_id" minimum(1)
// @Param from query string false "RFC3339 timestamp; defaults to now"
// @Param until query string false "RFC3339 timestamp; defaults to from plus 2 hours"
// @Param limit query int false "Maximum results" minimum(1) maximum(100) default(20)
// @Success 200 {array} transit.Departure
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string

// @Router /api/departures [get]
func (h *ScheduleHandler) Departures(w http.ResponseWriter, r *http.Request) {
	q, err := h.departureQuery(r)
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	result, err := h.service.Departures(r.Context(), q)
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// Stop loads stop details and associated departures.
// @Tags schedules
// @Produce json
// @Summary Get stop details and upcoming departures
// @Description Filters combine with AND. Path IDs fix their filter; conflicting query IDs are rejected. Departure windows are [from, until), at most 24 hours. /api/departures requires at least one ID filter.
// @Param id path int true "Positive stop ID" minimum(1)
// @Param stop_id query int false "Positive stop_id" minimum(1)
// @Param route_id query int false "Positive route_id" minimum(1)
// @Param trip_id query int false "Positive trip_id" minimum(1)
// @Param from query string false "RFC3339 timestamp; defaults to now"
// @Param until query string false "RFC3339 timestamp; defaults to from plus 2 hours"
// @Param limit query int false "Maximum results" minimum(1) maximum(100) default(20)
// @Success 200 {object} transit.StopDetails
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/stops/{id} [get]
func (h *ScheduleHandler) Stop(w http.ResponseWriter, r *http.Request) {
	id, ok := transitRouteID(w, r)
	if !ok {
		return
	}
	q, err := h.departureQuery(r)
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	if q.StopID != 0 && q.StopID != id {
		writeScheduleError(w, &transit.InvalidInput{Message: "query id conflicts with path id"})
		return
	}
	result, err := h.service.Stop(r.Context(), id, q)
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func scheduleQuery(r *http.Request, allowed ...string) (url.Values, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, &transit.InvalidInput{Message: "invalid query string"}
	}
	for key, entries := range values {
		known := false
		for _, name := range allowed {
			if key == name {
				known = true
				break
			}
		}
		if !known || len(entries) != 1 || entries[0] == "" {
			return nil, &transit.InvalidInput{Message: "unknown, repeated, or empty query parameter: " + key}
		}
	}
	return values, nil
}

type queryIDTarget struct {
	name   string
	target *int
}

func parsePositiveQueryIDs(values url.Values, fields ...queryIDTarget) error {
	for _, field := range fields {
		if value := values.Get(field.name); value != "" {
			id, err := strconv.ParseInt(value, 10, 32)
			if err != nil || id <= 0 {
				return &transit.InvalidInput{Message: field.name + " must be a positive 32-bit integer"}
			}
			*field.target = int(id)
		}
	}
	return nil
}

func parseTimeWindow(values url.Values, from, until *time.Time, defaultWindow time.Duration) error {
	if value := values.Get("from"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return &transit.InvalidInput{Message: "from must be an RFC3339 timestamp with a timezone"}
		}
		*from = parsed
	}
	*until = from.Add(defaultWindow)
	if value := values.Get("until"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return &transit.InvalidInput{Message: "until must be an RFC3339 timestamp with a timezone"}
		}
		*until = parsed
	}
	return nil
}

func (h *ScheduleHandler) departureQuery(r *http.Request) (transit.DepartureQuery, error) {
	q := transit.DepartureQuery{From: h.now().UTC(), Limit: 20}
	values, err := scheduleQuery(r, "stop_id", "route_id", "trip_id", "from", "until", "limit")
	if err != nil {
		return q, err
	}
	if err := parsePositiveQueryIDs(values,
		queryIDTarget{"stop_id", &q.StopID},
		queryIDTarget{"route_id", &q.RouteID},
		queryIDTarget{"trip_id", &q.TripID},
	); err != nil {
		return q, err
	}
	// Nested resource paths fix their own filter; conflicting query IDs are errors.
	for _, field := range []queryIDTarget{
		{"stopID", &q.StopID}, {"routeID", &q.RouteID}, {"tripID", &q.TripID},
	} {
		if value := chi.URLParam(r, field.name); value != "" {
			id, err := strconv.ParseInt(value, 10, 32)
			if err != nil || id <= 0 {
				return q, &transit.InvalidInput{Message: "path id must be a positive 32-bit integer"}
			}
			if *field.target != 0 && *field.target != int(id) {
				return q, &transit.InvalidInput{Message: "query id conflicts with path id"}
			}
			*field.target = int(id)
		}
	}
	if err := parseTimeWindow(values, &q.From, &q.Until, 2*time.Hour); err != nil {
		return q, err
	}
	if value := values.Get("limit"); value != "" {
		q.Limit, err = strconv.Atoi(value)
		if err != nil {
			return q, &transit.InvalidInput{Message: "limit must be between 1 and 100"}
		}
	}
	return q, nil
}

func (h *ScheduleHandler) journeyQuery(r *http.Request) (transit.JourneyQuery, error) {
	q := transit.JourneyQuery{
		From:          h.now().UTC(),
		Limit:         5,
		MaxTransfers:  transit.DefaultMaxTransfers,
		MaxWalkMeters: transit.DefaultMaxWalkMeters,
	}
	values, err := scheduleQuery(
		r,
		"origin_stop_id",
		"destination_stop_id",
		"origin",
		"destination",
		"from",
		"until",
		"limit",
		"max_transfers",
		"max_walk_meters",
	)
	if err != nil {
		return q, err
	}
	q.Origin = values.Get("origin")
	q.Destination = values.Get("destination")
	if err := parsePositiveQueryIDs(values,
		queryIDTarget{"origin_stop_id", &q.OriginStopID},
		queryIDTarget{"destination_stop_id", &q.DestinationStopID},
	); err != nil {
		return q, err
	}
	if err := parseTimeWindow(values, &q.From, &q.Until, 6*time.Hour); err != nil {
		return q, err
	}
	if value := values.Get("limit"); value != "" {
		q.Limit, err = strconv.Atoi(value)
		if err != nil {
			return q, &transit.InvalidInput{Message: "limit must be between 1 and 20"}
		}
	}
	if value := values.Get("max_transfers"); value != "" {
		q.MaxTransfers, err = strconv.Atoi(value)
		if err != nil {
			return q, &transit.InvalidInput{Message: "max_transfers must be between 0 and 3"}
		}
	}
	if value := values.Get("max_walk_meters"); value != "" {
		q.MaxWalkMeters, err = strconv.Atoi(value)
		if err != nil {
			return q, &transit.InvalidInput{Message: "max_walk_meters must be between 0 and 1500"}
		}
	}
	return q, nil
}

func writeScheduleError(w http.ResponseWriter, err error) {
	var invalid *transit.InvalidInput
	switch {
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": invalid.Message})
	case errors.Is(err, transit.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "transit resource not found"})
	default:
		log.Printf("schedule operation failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to retrieve schedule information"})
	}
}

// StopDepartures returns departures scoped to the resource in the path.
//
// @Tags schedules
// @Produce json
// @Summary Get upcoming departures
// @Description Filters combine with AND. Path IDs fix their filter; conflicting query IDs are rejected. Departure windows are [from, until), at most 24 hours. /api/departures requires at least one ID filter.
// @Param stop_id query int false "Positive stop_id" minimum(1)
// @Param route_id query int false "Positive route_id" minimum(1)
// @Param trip_id query int false "Positive trip_id" minimum(1)
// @Param from query string false "RFC3339 timestamp; defaults to now"
// @Param until query string false "RFC3339 timestamp; defaults to from plus 2 hours"
// @Param limit query int false "Maximum results" minimum(1) maximum(100) default(20)
// @Success 200 {array} transit.Departure
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string

// @Param stopID path int true "Positive resource ID" minimum(1)
// @Router /api/stops/{stopID}/departures [get]
func (h *ScheduleHandler) StopDepartures(w http.ResponseWriter, r *http.Request) {
	h.Departures(w, r)
}

// RouteDepartures returns departures scoped to the resource in the path.
//
// @Tags schedules
// @Produce json
// @Summary Get upcoming departures
// @Description Filters combine with AND. Path IDs fix their filter; conflicting query IDs are rejected. Departure windows are [from, until), at most 24 hours. /api/departures requires at least one ID filter.
// @Param stop_id query int false "Positive stop_id" minimum(1)
// @Param route_id query int false "Positive route_id" minimum(1)
// @Param trip_id query int false "Positive trip_id" minimum(1)
// @Param from query string false "RFC3339 timestamp; defaults to now"
// @Param until query string false "RFC3339 timestamp; defaults to from plus 2 hours"
// @Param limit query int false "Maximum results" minimum(1) maximum(100) default(20)
// @Success 200 {array} transit.Departure
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string

// @Param routeID path int true "Positive resource ID" minimum(1)
// @Router /api/routes/{routeID}/departures [get]
func (h *ScheduleHandler) RouteDepartures(w http.ResponseWriter, r *http.Request) {
	h.Departures(w, r)
}

// TripDepartures returns departures scoped to the resource in the path.
//
// @Tags schedules
// @Produce json
// @Summary Get upcoming departures
// @Description Filters combine with AND. Path IDs fix their filter; conflicting query IDs are rejected. Departure windows are [from, until), at most 24 hours. /api/departures requires at least one ID filter.
// @Param stop_id query int false "Positive stop_id" minimum(1)
// @Param route_id query int false "Positive route_id" minimum(1)
// @Param trip_id query int false "Positive trip_id" minimum(1)
// @Param from query string false "RFC3339 timestamp; defaults to now"
// @Param until query string false "RFC3339 timestamp; defaults to from plus 2 hours"
// @Param limit query int false "Maximum results" minimum(1) maximum(100) default(20)
// @Success 200 {array} transit.Departure
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string

// @Param tripID path int true "Positive resource ID" minimum(1)
// @Router /api/trips/{tripID}/departures [get]
func (h *ScheduleHandler) TripDepartures(w http.ResponseWriter, r *http.Request) {
	h.Departures(w, r)
}

// Stops searches or lists active transit stops.
//
// @Tags schedules
// @Produce json
// @Summary Search or list active stops
// @Param q query string false "Case-insensitive stop name filter"
// @Param limit query int false "Maximum results" minimum(1) maximum(100) default(20)
// @Success 200 {array} transit.Stop
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/stops [get]
func (h *ScheduleHandler) Stops(w http.ResponseWriter, r *http.Request) {
	values, err := scheduleQuery(r, "q", "limit")
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	q := transit.StopSearchQuery{Query: values.Get("q"), Limit: 20}
	if value := values.Get("limit"); value != "" {
		q.Limit, err = strconv.Atoi(value)
		if err != nil {
			writeScheduleError(w, &transit.InvalidInput{Message: "limit must be between 1 and 100"})
			return
		}
	}
	stops, err := h.service.SearchStops(r.Context(), q)
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stops)
}

// Journeys searches direct, multi-transfer, and walking-connected transit itineraries between two locations.
//
// @Tags schedules
// @Produce json
// @Summary Search transit journeys between two locations
// @Description Accepts origin (origin_stop_id and/or origin) and destination (destination_stop_id and/or destination). Returns direct, multi-transfer (up to 3 transfers), and walking-connected itinerary options with ordered leg stops, optional live estimates, and exclusion of cancelled trips, closed stops, and disrupted route segments, or a meaningful message when no itinerary is found.
// @Param origin_stop_id query int false "Positive origin stop ID" minimum(1)
// @Param destination_stop_id query int false "Positive destination stop ID" minimum(1)
// @Param origin query string false "Origin stop or location name"
// @Param destination query string false "Destination stop or location name"
// @Param from query string false "RFC3339 timestamp; defaults to now"
// @Param until query string false "RFC3339 timestamp; defaults to from plus 6 hours"
// @Param limit query int false "Maximum itineraries" minimum(1) maximum(20) default(5)
// @Param max_transfers query int false "Maximum transit transfers" minimum(0) maximum(3) default(3)
// @Param max_walk_meters query int false "Maximum straight-line walking distance per walk leg in meters (0 disables walking)" minimum(0) maximum(1500) default(600)
// @Success 200 {object} transit.JourneySearchResult
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/journeys [get]
func (h *ScheduleHandler) Journeys(w http.ResponseWriter, r *http.Request) {
	q, err := h.journeyQuery(r)
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	result, err := h.service.Journeys(r.Context(), q)
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
