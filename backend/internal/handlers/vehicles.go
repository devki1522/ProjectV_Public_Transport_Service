package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/auth"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/vehicles"
	"github.com/go-chi/chi/v5"
)

// VehicleService describes the vehicle location, route geometry, and estimated arrival operations needed by HTTP handlers.
type VehicleService interface {
	SubmitLocation(ctx context.Context, input vehicles.LocationUpdateInput, defaultSource string) (vehicles.LocationUpdateResult, error)
	ListActiveVehicles(ctx context.Context, query vehicles.ActiveVehiclesQuery) ([]vehicles.ActiveVehicle, error)
	RouteGeometry(ctx context.Context, routeID int) (vehicles.RouteGeometry, error)
	UpcomingArrivals(ctx context.Context, tripID, stopID int) ([]vehicles.EstimatedArrival, error)
	StopArrivals(ctx context.Context, stopID, routeID int) ([]vehicles.EstimatedArrival, error)
}

// VehicleSimulator describes lifecycle controls for the built-in live bus simulator.
type VehicleSimulator interface {
	Status() vehicles.SimulatorStatus
	Start(speedMultiplier *float64) (vehicles.SimulatorStatus, error)
	Stop() vehicles.SimulatorStatus
	Step(context.Context) (vehicles.SimulatorStatus, error)
	Reset(context.Context) (vehicles.SimulatorStatus, error)
}

// SimulatorStartInput optionally overrides simulation speed when starting the simulator.
type SimulatorStartInput struct {
	SpeedMultiplier *float64 `json:"speed_multiplier,omitempty"`
}

// VehicleHandler serves live vehicle tracking, map geometry, and simulator endpoints.
type VehicleHandler struct {
	service   VehicleService
	simulator VehicleSimulator
	audit     *slog.Logger
}

// NewVehicleHandler creates a VehicleHandler with default JSON audit logging to stdout.
func NewVehicleHandler(service VehicleService, simulator VehicleSimulator) *VehicleHandler {
	return NewVehicleHandlerWithAudit(service, simulator, nil)
}

// NewVehicleHandlerWithAudit creates a VehicleHandler with an explicit structured audit logger.
func NewVehicleHandlerWithAudit(service VehicleService, simulator VehicleSimulator, audit *slog.Logger) *VehicleHandler {
	if audit == nil {
		audit = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return &VehicleHandler{
		service:   service,
		simulator: simulator,
		audit:     audit,
	}
}

// SubmitLocation creates or updates the latest reported location of a transit vehicle.
//
// @Summary Submit a vehicle location update
// @Description Requires an administrator session cookie, trusted Origin, and X-CSRF-Token. Updates the vehicle's latest GPS coordinates, speed, heading, route/trip assignment, and operating status.
// @Tags vehicles
// @Accept json
// @Produce json
// @Param X-CSRF-Token header string true "Session-bound CSRF token"
// @Param body body vehicles.LocationUpdateInput true "Vehicle location payload"
// @Success 200 {object} vehicles.ActiveVehicle
// @Success 201 {object} vehicles.ActiveVehicle
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/vehicles/locations [post]
func (h *VehicleHandler) SubmitLocation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "vehicle service is unavailable"})
		return
	}

	var input vehicles.LocationUpdateInput
	if !decodeStrictJSONObject(w, r, &input) {
		h.auditVehicleUpdate(r.Context(), 0, "invalid_request")
		return
	}

	defaultSource := "admin"
	if principal, ok := auth.PrincipalFromContext(r.Context()); ok && principal.Username != "" {
		defaultSource = principal.Username
	}

	result, err := h.service.SubmitLocation(r.Context(), input, defaultSource)
	if err != nil {
		h.auditVehicleUpdate(r.Context(), 0, vehicleAuditOutcome(err))
		writeVehicleError(w, err)
		return
	}

	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	h.auditVehicleUpdate(r.Context(), result.Vehicle.VehicleID, "success")
	writeJSON(w, status, result.Vehicle)
}

// ListActive returns recently active transit vehicles for live fleet or map views.
//
// @Summary List active vehicles
// @Description Filters active non-retired vehicles reported within max_age_seconds (default 900s) by optional route_id, route_number, or trip_id.
// @Tags vehicles
// @Produce json
// @Param route_id query int false "Positive route_id" minimum(1)
// @Param route_number query string false "Public route number (case-insensitive)"
// @Param trip_id query int false "Positive trip_id" minimum(1)
// @Param max_age_seconds query int false "Freshness window in seconds" minimum(10) maximum(86400) default(900)
// @Success 200 {array} vehicles.ActiveVehicle
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/vehicles [get]
func (h *VehicleHandler) ListActive(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "vehicle service is unavailable"})
		return
	}
	q, err := parseActiveVehiclesQuery(r)
	if err != nil {
		writeVehicleError(w, err)
		return
	}
	list, err := h.service.ListActiveVehicles(r.Context(), q)
	if err != nil {
		writeVehicleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// ListByRoute returns active vehicles currently operating on the specified route ID.
//
// @Summary List active vehicles for a route
// @Tags vehicles
// @Produce json
// @Param id path int true "Positive route ID" minimum(1)
// @Param route_id query int false "Conflicting route_id is rejected" minimum(1)
// @Param trip_id query int false "Optional trip_id filter" minimum(1)
// @Param max_age_seconds query int false "Freshness window in seconds" minimum(10) maximum(86400) default(900)
// @Success 200 {array} vehicles.ActiveVehicle
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/routes/{id}/vehicles [get]
func (h *VehicleHandler) ListByRoute(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "vehicle service is unavailable"})
		return
	}
	id, ok := parsePositivePathID(w, r, "id")
	if !ok {
		return
	}
	q, err := parseActiveVehiclesQuery(r)
	if err != nil {
		writeVehicleError(w, err)
		return
	}
	if q.RouteID != 0 && q.RouteID != id {
		writeVehicleError(w, &vehicles.InvalidInput{Message: "query route_id conflicts with path id"})
		return
	}
	q.RouteID = id
	list, err := h.service.ListActiveVehicles(r.Context(), q)
	if err != nil {
		writeVehicleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// ListByTrip returns active vehicles currently operating on the specified trip ID.
//
// @Summary List active vehicles for a trip
// @Tags vehicles
// @Produce json
// @Param id path int true "Positive trip ID" minimum(1)
// @Param route_id query int false "Optional route_id filter" minimum(1)
// @Param trip_id query int false "Conflicting trip_id is rejected" minimum(1)
// @Param max_age_seconds query int false "Freshness window in seconds" minimum(10) maximum(86400) default(900)
// @Success 200 {array} vehicles.ActiveVehicle
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/trips/{id}/vehicles [get]
func (h *VehicleHandler) ListByTrip(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "vehicle service is unavailable"})
		return
	}
	id, ok := parsePositivePathID(w, r, "id")
	if !ok {
		return
	}
	q, err := parseActiveVehiclesQuery(r)
	if err != nil {
		writeVehicleError(w, err)
		return
	}
	if q.TripID != 0 && q.TripID != id {
		writeVehicleError(w, &vehicles.InvalidInput{Message: "query trip_id conflicts with path id"})
		return
	}
	q.TripID = id
	list, err := h.service.ListActiveVehicles(r.Context(), q)
	if err != nil {
		writeVehicleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// RouteShape returns map polyline coordinates for a route from primary route shapes or ordered stop coordinates.
//
// @Summary Get route polyline geometry for map rendering
// @Tags vehicles
// @Produce json
// @Param id path int true "Positive route ID" minimum(1)
// @Success 200 {object} vehicles.RouteGeometry
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/routes/{id}/shape [get]
func (h *VehicleHandler) RouteShape(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "vehicle service is unavailable"})
		return
	}
	id, ok := parsePositivePathID(w, r, "id")
	if !ok {
		return
	}
	geom, err := h.service.RouteGeometry(r.Context(), id)
	if err != nil {
		writeVehicleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, geom)
}

// TripArrivals returns live estimated arrival times for upcoming stops along an active trip.
//
// @Summary Get estimated bus arrivals for a trip
// @Description Returns estimated arrival times for upcoming stops of an active trip that has recent vehicle location data, preserving scheduled arrival times. Returns an empty array when no active vehicle with recent location data is reporting on the trip.
// @Tags vehicles
// @Produce json
// @Param id path int true "Positive trip ID" minimum(1)
// @Success 200 {array} vehicles.EstimatedArrival
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/trips/{id}/arrivals [get]
func (h *VehicleHandler) TripArrivals(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "vehicle service is unavailable"})
		return
	}
	id, ok := parsePositivePathID(w, r, "id")
	if !ok {
		return
	}
	if err := rejectUnexpectedQueryParams(r); err != nil {
		writeVehicleError(w, err)
		return
	}
	arrivals, err := h.service.UpcomingArrivals(r.Context(), id, 0)
	if err != nil {
		writeVehicleError(w, err)
		return
	}
	if arrivals == nil {
		arrivals = []vehicles.EstimatedArrival{}
	}
	writeJSON(w, http.StatusOK, arrivals)
}

// StopArrivals returns live estimated arrival times for active vehicles approaching a stop.
//
// @Summary Get estimated bus arrivals at a stop
// @Description Returns estimated arrival times for active vehicles on routes serving the requested stop (optionally filtered by route_id) that have recent location data. Returns an empty array when no active vehicle with recent location data is approaching the stop.
// @Tags vehicles
// @Produce json
// @Param id path int true "Positive stop ID" minimum(1)
// @Param route_id query int false "Optional positive route ID filter" minimum(1)
// @Success 200 {array} vehicles.EstimatedArrival
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/stops/{id}/arrivals [get]
func (h *VehicleHandler) StopArrivals(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "vehicle service is unavailable"})
		return
	}
	id, ok := parsePositivePathID(w, r, "id")
	if !ok {
		return
	}
	routeID, err := parseOptionalRouteIDQuery(r)
	if err != nil {
		writeVehicleError(w, err)
		return
	}
	arrivals, err := h.service.StopArrivals(r.Context(), id, routeID)
	if err != nil {
		writeVehicleError(w, err)
		return
	}
	if arrivals == nil {
		arrivals = []vehicles.EstimatedArrival{}
	}
	writeJSON(w, http.StatusOK, arrivals)
}

// SimulatorStatus returns the status of the built-in live bus simulator.
//
// @Summary Get live bus simulator status
// @Tags vehicles
// @Produce json
// @Success 200 {object} vehicles.SimulatorStatus
// @Router /api/vehicles/simulator [get]
func (h *VehicleHandler) SimulatorStatus(w http.ResponseWriter, _ *http.Request) {
	if h.simulator == nil {
		writeJSON(w, http.StatusOK, vehicles.SimulatorStatus{Running: false, SpeedMultiplier: 1})
		return
	}
	writeJSON(w, http.StatusOK, h.simulator.Status())
}

// StartSimulator starts or resumes periodic live bus simulation ticks.
//
// @Summary Start live bus simulator
// @Tags vehicles
// @Accept json
// @Produce json
// @Param X-CSRF-Token header string true "Session-bound CSRF token"
// @Param body body SimulatorStartInput false "Optional simulator speed multiplier"
// @Success 200 {object} vehicles.SimulatorStatus
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/admin/vehicles/simulator/start [post]
func (h *VehicleHandler) StartSimulator(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.simulator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "vehicle simulator is not configured"})
		return
	}
	var input SimulatorStartInput
	if !decodeOptionalJSONObject(w, r, &input) {
		return
	}
	status, err := h.simulator.Start(input.SpeedMultiplier)
	if err != nil {
		writeVehicleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// StopSimulator pauses periodic live bus simulation ticks.
//
// @Summary Pause live bus simulator
// @Tags vehicles
// @Produce json
// @Param X-CSRF-Token header string true "Session-bound CSRF token"
// @Success 200 {object} vehicles.SimulatorStatus
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/admin/vehicles/simulator/stop [post]
func (h *VehicleHandler) StopSimulator(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.simulator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "vehicle simulator is not configured"})
		return
	}
	writeJSON(w, http.StatusOK, h.simulator.Stop())
}

// StepSimulator advances simulated buses by a single tick immediately.
//
// @Summary Advance simulated buses by one step
// @Tags vehicles
// @Produce json
// @Param X-CSRF-Token header string true "Session-bound CSRF token"
// @Success 200 {object} vehicles.SimulatorStatus
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/admin/vehicles/simulator/step [post]
func (h *VehicleHandler) StepSimulator(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.simulator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "vehicle simulator is not configured"})
		return
	}
	status, err := h.simulator.Step(r.Context())
	if err != nil {
		writeVehicleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// ResetSimulator clears simulated vehicle locations and resets simulation progress.
//
// @Summary Reset simulated vehicle locations
// @Tags vehicles
// @Produce json
// @Param X-CSRF-Token header string true "Session-bound CSRF token"
// @Success 200 {object} vehicles.SimulatorStatus
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /api/admin/vehicles/simulator/reset [post]
func (h *VehicleHandler) ResetSimulator(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.simulator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "vehicle simulator is not configured"})
		return
	}
	status, err := h.simulator.Reset(r.Context())
	if err != nil {
		writeVehicleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *VehicleHandler) auditVehicleUpdate(ctx context.Context, vehicleID int, outcome string) {
	if h.audit == nil {
		return
	}
	subject := "unknown"
	if principal, ok := auth.PrincipalFromContext(ctx); ok && principal.Subject != "" {
		subject = principal.Subject
	}
	attrs := []slog.Attr{
		slog.String("timestamp", time.Now().UTC().Format(time.RFC3339Nano)),
		slog.String("subject", subject),
		slog.String("operation", "upsert"),
		slog.String("resource_type", "vehicle_location"),
		slog.String("outcome", outcome),
	}
	if vehicleID > 0 {
		attrs = append(attrs, slog.Int("vehicle_id", vehicleID))
	}
	h.audit.LogAttrs(ctx, slog.LevelInfo, "admin_vehicle_location_update", attrs...)
}

func vehicleAuditOutcome(err error) string {
	var invalid *vehicles.InvalidInput
	switch {
	case errors.As(err, &invalid):
		return "validation_error"
	case errors.Is(err, vehicles.ErrNotFound):
		return "not_found"
	case errors.Is(err, vehicles.ErrStaleLocationUpdate):
		return "conflict"
	default:
		return "internal_error"
	}
}

func parsePositivePathID(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 32)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": name + " must be a positive 32-bit integer"})
		return 0, false
	}
	return int(id), true
}

func parseActiveVehiclesQuery(r *http.Request) (vehicles.ActiveVehiclesQuery, error) {
	var q vehicles.ActiveVehiclesQuery
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return q, &vehicles.InvalidInput{Message: "invalid query string"}
	}

	allowed := map[string]bool{
		"route_id":        true,
		"route_number":    true,
		"trip_id":         true,
		"max_age_seconds": true,
	}
	for key, entries := range values {
		if !allowed[key] || len(entries) != 1 || entries[0] == "" {
			return q, &vehicles.InvalidInput{Message: "unknown, repeated, or empty query parameter: " + key}
		}
	}

	if value := values.Get("route_id"); value != "" {
		id, err := strconv.ParseInt(value, 10, 32)
		if err != nil || id <= 0 {
			return q, &vehicles.InvalidInput{Message: "route_id must be a positive 32-bit integer"}
		}
		q.RouteID = int(id)
	}
	if value := values.Get("trip_id"); value != "" {
		id, err := strconv.ParseInt(value, 10, 32)
		if err != nil || id <= 0 {
			return q, &vehicles.InvalidInput{Message: "trip_id must be a positive 32-bit integer"}
		}
		q.TripID = int(id)
	}
	q.RouteNumber = values.Get("route_number")
	if value := values.Get("max_age_seconds"); value != "" {
		seconds, err := strconv.Atoi(value)
		if err != nil {
			return q, &vehicles.InvalidInput{Message: "max_age_seconds must be between 10 and 86400"}
		}
		q.MaxAge = time.Duration(seconds) * time.Second
	}
	return q, nil
}

func rejectUnexpectedQueryParams(r *http.Request) error {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return &vehicles.InvalidInput{Message: "invalid query string"}
	}
	for key := range values {
		return &vehicles.InvalidInput{Message: "unknown query parameter: " + key}
	}
	return nil
}

func parseOptionalRouteIDQuery(r *http.Request) (int, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return 0, &vehicles.InvalidInput{Message: "invalid query string"}
	}
	for key, entries := range values {
		if key != "route_id" || len(entries) != 1 || entries[0] == "" {
			return 0, &vehicles.InvalidInput{Message: "unknown, repeated, or empty query parameter: " + key}
		}
	}
	if value := values.Get("route_id"); value != "" {
		id, err := strconv.ParseInt(value, 10, 32)
		if err != nil || id <= 0 {
			return 0, &vehicles.InvalidInput{Message: "route_id must be a positive 32-bit integer"}
		}
		return int(id), nil
	}
	return 0, nil
}

func decodeStrictJSONObject(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); err != nil || object == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be a JSON object"})
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must contain exactly one JSON object"})
		return false
	}
	data, _ := json.Marshal(object)
	fields := json.NewDecoder(bytes.NewReader(data))
	fields.DisallowUnknownFields()
	if err := fields.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body contains an unknown field or invalid field type"})
		return false
	}
	return true
}

func decodeOptionalJSONObject(w http.ResponseWriter, r *http.Request, target any) bool {
	if r.Body == nil {
		return true
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unable to read request body"})
		return false
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return true
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); err != nil || object == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be a JSON object"})
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must contain exactly one JSON object"})
		return false
	}
	fields := json.NewDecoder(bytes.NewReader(raw))
	fields.DisallowUnknownFields()
	if err := fields.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body contains an unknown field or invalid field type"})
		return false
	}
	return true
}

func writeVehicleError(w http.ResponseWriter, err error) {
	var invalid *vehicles.InvalidInput
	switch {
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": invalid.Message})
	case errors.Is(err, vehicles.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "transit resource not found"})
	case errors.Is(err, vehicles.ErrStaleLocationUpdate):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		log.Printf("vehicle operation failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to complete vehicle operation"})
	}
}
