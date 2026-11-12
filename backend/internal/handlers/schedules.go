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

func (h *ScheduleHandler) departureQuery(r *http.Request) (transit.DepartureQuery, error) {
	q := transit.DepartureQuery{From: h.now().UTC(), Limit: 20}
	values, err := scheduleQuery(r, "stop_id", "route_id", "trip_id", "from", "until", "limit")
	if err != nil {
		return q, err
	}
	for _, field := range []struct {
		name   string
		target *int
	}{
		{"stop_id", &q.StopID}, {"route_id", &q.RouteID}, {"trip_id", &q.TripID},
	} {
		if value := values.Get(field.name); value != "" {
			id, err := strconv.ParseInt(value, 10, 32)
			if err != nil || id <= 0 {
				return q, &transit.InvalidInput{Message: field.name + " must be a positive 32-bit integer"}
			}
			*field.target = int(id)
		}
	}
	// Nested resource paths fix their own filter; conflicting query IDs are errors.
	for _, field := range []struct {
		name   string
		target *int
	}{
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
	if value := values.Get("from"); value != "" {
		q.From, err = time.Parse(time.RFC3339, value)
		if err != nil {
			return q, &transit.InvalidInput{Message: "from must be an RFC3339 timestamp with a timezone"}
		}
	}
	q.Until = q.From.Add(2 * time.Hour)
	if value := values.Get("until"); value != "" {
		q.Until, err = time.Parse(time.RFC3339, value)
		if err != nil {
			return q, &transit.InvalidInput{Message: "until must be an RFC3339 timestamp with a timezone"}
		}
	}
	if value := values.Get("limit"); value != "" {
		q.Limit, err = strconv.Atoi(value)
		if err != nil {
			return q, &transit.InvalidInput{Message: "limit must be between 1 and 100"}
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
