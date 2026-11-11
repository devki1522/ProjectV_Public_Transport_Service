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

type ScheduleService interface {
	Timetable(context.Context, int, string) (transit.Timetable, error)
	Departures(context.Context, transit.DepartureQuery) ([]transit.Departure, error)
	Stop(context.Context, int, transit.DepartureQuery) (transit.StopDetails, error)
}

type ScheduleHandler struct {
	service ScheduleService
	now     func() time.Time
}

func NewScheduleHandler(service ScheduleService) *ScheduleHandler {
	return &ScheduleHandler{service: service, now: time.Now}
}

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
