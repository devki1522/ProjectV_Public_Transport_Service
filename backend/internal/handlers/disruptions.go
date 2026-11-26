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
	"strings"
	"time"
	"unicode/utf8"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/auth"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/disruptions"
)

// DisruptionService describes the disruption, operational status, and road-closure ingestion operations needed by HTTP handlers.
type DisruptionService interface {
	List(ctx context.Context, filter disruptions.ListFilter) ([]disruptions.DisruptionResponse, error)
	ListActive(ctx context.Context, filter disruptions.ListFilter) ([]disruptions.DisruptionResponse, error)
	Get(ctx context.Context, id int) (disruptions.DisruptionResponse, error)
	Create(ctx context.Context, input disruptions.CreateDisruptionInput, defaultSource string) (disruptions.DisruptionResponse, error)
	CreateRoadClosure(ctx context.Context, input disruptions.RoadClosureInput, defaultSource string) (disruptions.DisruptionResponse, error)
	GetRoadClosure(ctx context.Context, id int) (disruptions.DisruptionResponse, error)
	UpdateRoadClosure(ctx context.Context, id int, input disruptions.UpdateRoadClosureInput) (disruptions.DisruptionResponse, error)
	Update(ctx context.Context, id int, input disruptions.UpdateDisruptionInput) (disruptions.DisruptionResponse, error)
	AffectedServices(ctx context.Context) (disruptions.AffectedServicesSummary, error)
	ServiceStatus(ctx context.Context) (disruptions.ServiceStatusResponse, error)
	RouteStatus(ctx context.Context, routeID int) (disruptions.EntityOperationalStatus, error)
	StopStatus(ctx context.Context, stopID int) (disruptions.EntityOperationalStatus, error)
	TripStatus(ctx context.Context, tripID int) (disruptions.EntityOperationalStatus, error)
}

// DisruptionHandler serves public disruption and operational status queries as well as protected admin and enterprise integration endpoints.
type DisruptionHandler struct {
	service DisruptionService
	audit   *slog.Logger
}

// NewDisruptionHandler creates a DisruptionHandler with default JSON audit logging to stdout.
func NewDisruptionHandler(service DisruptionService) *DisruptionHandler {
	return NewDisruptionHandlerWithAudit(service, nil)
}

// NewDisruptionHandlerWithAudit creates a DisruptionHandler with an explicit structured audit logger.
func NewDisruptionHandlerWithAudit(service DisruptionService, audit *slog.Logger) *DisruptionHandler {
	if audit == nil {
		audit = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return &DisruptionHandler{
		service: service,
		audit:   audit,
	}
}

// List returns disruptions matching optional filter criteria.
//
// @Summary List transit disruptions
// @Description Lists transit disruptions filtered by optional status, type, route_id, stop_id, trip_id, vehicle_id, or source_service.
// @Tags disruptions
// @Produce json
// @Param status query string false "Disruption lifecycle status (scheduled, active, resolved)"
// @Param type query string false "Disruption type (delay, detour, cancellation, stop_closure, road_closure, construction, emergency, traffic_restriction, accessibility)"
// @Param route_id query int false "Positive route ID" minimum(1)
// @Param stop_id query int false "Positive stop ID" minimum(1)
// @Param trip_id query int false "Positive trip ID" minimum(1)
// @Param vehicle_id query int false "Positive vehicle ID" minimum(1)
// @Param source_service query string false "Originating service identifier"
// @Success 200 {array} disruptions.DisruptionResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/disruptions [get]
func (h *DisruptionHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}
	filter, err := parseDisruptionListQuery(r, true)
	if err != nil {
		writeDisruptionError(w, err)
		return
	}
	list, err := h.service.List(r.Context(), filter)
	if err != nil {
		writeDisruptionError(w, err)
		return
	}
	if list == nil {
		list = []disruptions.DisruptionResponse{}
	}
	writeJSON(w, http.StatusOK, list)
}

// ListActive returns currently active transit disruptions (`status='active'`).
//
// @Summary List active transit disruptions
// @Description Returns only active transit disruptions (`status='active'`), optionally filtered by type, route_id, stop_id, or trip_id.
// @Tags disruptions
// @Produce json
// @Param type query string false "Disruption type filter"
// @Param route_id query int false "Positive route ID filter" minimum(1)
// @Param stop_id query int false "Positive stop ID filter" minimum(1)
// @Param trip_id query int false "Positive trip ID filter" minimum(1)
// @Success 200 {array} disruptions.DisruptionResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/disruptions/active [get]
func (h *DisruptionHandler) ListActive(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}
	filter, err := parseDisruptionListQuery(r, false)
	if err != nil {
		writeDisruptionError(w, err)
		return
	}
	list, err := h.service.ListActive(r.Context(), filter)
	if err != nil {
		writeDisruptionError(w, err)
		return
	}
	if list == nil {
		list = []disruptions.DisruptionResponse{}
	}
	writeJSON(w, http.StatusOK, list)
}

// AffectedServices returns a summary of all routes, stops, trips, vehicles, and segments impacted by active disruptions.
//
// @Summary Get affected transit services summary
// @Description Aggregates all routes, stops, trips, vehicles, and route segments currently impacted by active disruptions along with their derived operational statuses.
// @Tags disruptions
// @Produce json
// @Success 200 {object} disruptions.AffectedServicesSummary
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/disruptions/affected [get]
func (h *DisruptionHandler) AffectedServices(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}
	if err := rejectUnexpectedDisruptionQuery(r); err != nil {
		writeDisruptionError(w, err)
		return
	}
	summary, err := h.service.AffectedServices(r.Context())
	if err != nil {
		writeDisruptionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// Get returns a single disruption by identifier.
//
// @Summary Get a transit disruption by ID
// @Tags disruptions
// @Produce json
// @Param id path int true "Positive disruption ID" minimum(1)
// @Success 200 {object} disruptions.DisruptionResponse
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/disruptions/{id} [get]
func (h *DisruptionHandler) Get(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}
	id, ok := parsePositivePathID(w, r, "id")
	if !ok {
		return
	}
	if err := rejectUnexpectedDisruptionQuery(r); err != nil {
		writeDisruptionError(w, err)
		return
	}
	item, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeDisruptionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// ServiceStatus returns the public service status snapshot including active disruptions, detours, and affected services.
//
// @Summary Get overall transit service status
// @Description Returns overall network operational status (normal or disrupted), active disruptions, active detours, and affected routes, stops, trips, vehicles, and segments.
// @Tags disruptions
// @Produce json
// @Success 200 {object} disruptions.ServiceStatusResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/service-status [get]
func (h *DisruptionHandler) ServiceStatus(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}
	if err := rejectUnexpectedDisruptionQuery(r); err != nil {
		writeDisruptionError(w, err)
		return
	}
	status, err := h.service.ServiceStatus(r.Context())
	if err != nil {
		writeDisruptionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// EnterpriseServiceStatus returns the authenticated enterprise service status snapshot (US 196 / PT-FR-021).
//
// @Summary Retrieve enterprise transit disruption and service status snapshot
// @Description Accepts a Registration Service service-account Bearer JWT (SH-S07; no cookie, Origin, or CSRF required) or an authenticated administrator session cookie. Returns overall network operational status, active disruptions, detours, and affected services.
// @Tags integrations
// @Security BearerAuth
// @Produce json
// @Success 200 {object} disruptions.ServiceStatusResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Failure 502 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Failure 504 {object} map[string]string
// @Router /api/transport/v1/integrations/service-status [get]
func (h *DisruptionHandler) EnterpriseServiceStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}
	if err := rejectUnexpectedDisruptionQuery(r); err != nil {
		h.auditDisruptionEvent(r.Context(), "read_enterprise_service_status", "service_status", 0, "", "validation_error")
		writeDisruptionError(w, err)
		return
	}
	status, err := h.service.ServiceStatus(r.Context())
	if err != nil {
		h.auditDisruptionEvent(r.Context(), "read_enterprise_service_status", "service_status", 0, "", disruptionAuditOutcome(err))
		writeDisruptionError(w, err)
		return
	}
	h.auditDisruptionEvent(r.Context(), "read_enterprise_service_status", "service_status", 0, "", "success")
	writeJSON(w, http.StatusOK, status)
}

// RouteStatus returns the current operational status and active disruptions for a single route.
//
// @Summary Get route operational status
// @Tags disruptions
// @Produce json
// @Param id path int true "Positive route ID" minimum(1)
// @Success 200 {object} disruptions.EntityOperationalStatus
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/routes/{id}/status [get]
func (h *DisruptionHandler) RouteStatus(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}
	id, ok := parsePositivePathID(w, r, "id")
	if !ok {
		return
	}
	if err := rejectUnexpectedDisruptionQuery(r); err != nil {
		writeDisruptionError(w, err)
		return
	}
	status, err := h.service.RouteStatus(r.Context(), id)
	if err != nil {
		writeDisruptionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// StopStatus returns the current operational status and active disruptions for a single stop.
//
// @Summary Get stop operational status
// @Tags disruptions
// @Produce json
// @Param id path int true "Positive stop ID" minimum(1)
// @Success 200 {object} disruptions.EntityOperationalStatus
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/stops/{id}/status [get]
func (h *DisruptionHandler) StopStatus(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}
	id, ok := parsePositivePathID(w, r, "id")
	if !ok {
		return
	}
	if err := rejectUnexpectedDisruptionQuery(r); err != nil {
		writeDisruptionError(w, err)
		return
	}
	status, err := h.service.StopStatus(r.Context(), id)
	if err != nil {
		writeDisruptionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// TripStatus returns the current operational status and active disruptions for a single trip.
//
// @Summary Get trip operational status
// @Tags disruptions
// @Produce json
// @Param id path int true "Positive trip ID" minimum(1)
// @Success 200 {object} disruptions.EntityOperationalStatus
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/trips/{id}/status [get]
func (h *DisruptionHandler) TripStatus(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}
	id, ok := parsePositivePathID(w, r, "id")
	if !ok {
		return
	}
	if err := rejectUnexpectedDisruptionQuery(r); err != nil {
		writeDisruptionError(w, err)
		return
	}
	status, err := h.service.TripStatus(r.Context(), id)
	if err != nil {
		writeDisruptionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// Create validates and creates a new transit disruption and associates affected transit entities.
//
// @Summary Create a transit disruption
// @Description Requires an administrator session cookie, trusted Origin, and X-CSRF-Token. Creates a disruption and links affected routes, stops, trips, vehicles, and segments.
// @Tags disruptions
// @Accept json
// @Produce json
// @Param X-CSRF-Token header string true "Session-bound CSRF token"
// @Param body body disruptions.CreateDisruptionInput true "Disruption creation payload"
// @Success 201 {object} disruptions.DisruptionResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/disruptions [post]
func (h *DisruptionHandler) Create(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}

	var input disruptions.CreateDisruptionInput
	if !decodeStrictJSONObject(w, r, &input) {
		h.auditDisruptionEvent(r.Context(), "create", "disruption", 0, disruptions.SourceTransitAdmin, "invalid_request")
		return
	}

	created, err := h.service.Create(r.Context(), input, disruptions.SourceTransitAdmin)
	if err != nil {
		h.auditDisruptionEvent(r.Context(), "create", "disruption", 0, disruptions.SourceTransitAdmin, disruptionAuditOutcome(err))
		writeDisruptionError(w, err)
		return
	}

	h.auditDisruptionEvent(r.Context(), "create", "disruption", created.ID, created.SourceService, "success")
	w.Header().Set("Location", apiBaseFromRequest(r)+"/disruptions/"+strconv.Itoa(created.ID))
	writeJSON(w, http.StatusCreated, created)
}

// ListRoadClosures returns road closures (`type='road_closure'`) matching optional filter criteria.
//
// @Summary List road closures
// @Description Lists road closure disruptions (`type='road_closure'`) filtered by optional status, route_id, stop_id, trip_id, vehicle_id, or source_service.
// @Tags road-closures
// @Produce json
// @Param status query string false "Disruption lifecycle status (scheduled, active, resolved)"
// @Param route_id query int false "Positive route ID" minimum(1)
// @Param stop_id query int false "Positive stop ID" minimum(1)
// @Param trip_id query int false "Positive trip ID" minimum(1)
// @Param vehicle_id query int false "Positive vehicle ID" minimum(1)
// @Param source_service query string false "Originating service identifier"
// @Success 200 {array} disruptions.DisruptionResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/road-closures [get]
func (h *DisruptionHandler) ListRoadClosures(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}
	filter, err := parseDisruptionListQuery(r, true)
	if err != nil {
		writeDisruptionError(w, err)
		return
	}
	if filter.Type != "" && filter.Type != disruptions.TypeRoadClosure {
		writeDisruptionError(w, &disruptions.InvalidInput{Message: "type must be road_closure for road-closures endpoint"})
		return
	}
	filter.Type = disruptions.TypeRoadClosure
	list, err := h.service.List(r.Context(), filter)
	if err != nil {
		writeDisruptionError(w, err)
		return
	}
	if list == nil {
		list = []disruptions.DisruptionResponse{}
	}
	writeJSON(w, http.StatusOK, list)
}

// GetRoadClosure returns a single road closure disruption (`type='road_closure'`) by identifier.
//
// @Summary Get road closure by ID
// @Description Returns a single road closure (`type='road_closure'`) and its affected routes, stops, trips, vehicles, and segments.
// @Tags road-closures
// @Produce json
// @Param id path int true "Positive road closure ID" minimum(1)
// @Success 200 {object} disruptions.DisruptionResponse
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/road-closures/{id} [get]
func (h *DisruptionHandler) GetRoadClosure(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}
	id, ok := parsePositivePathID(w, r, "id")
	if !ok {
		return
	}
	if err := rejectUnexpectedDisruptionQuery(r); err != nil {
		writeDisruptionError(w, err)
		return
	}
	item, err := h.service.GetRoadClosure(r.Context(), id)
	if err != nil {
		writeDisruptionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// CreateRoadClosure validates and creates a road closure disruption (`type='road_closure'`).
//
// @Summary Create a road closure
// @Description Accepts a Registration Service service-account Bearer JWT (SH-S07; no cookie, Origin, or CSRF required) or an administrator session cookie with trusted Origin and X-CSRF-Token. Accepts the shared SH-I06 address object and valid-until (unsigned integer seconds: Unix epoch timestamp >= 1000000000, or duration in seconds from start_time) as well as optional title, coordinates/radius spatial matching, and route/stop links. Returns 201 Created with the created road closure and its numeric id for subsequent PATCH /api/transport/v1/road-closures/{id} updates.
// @Tags road-closures
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param X-CSRF-Token header string false "Session-bound CSRF token (required for browser cookie sessions; omitted for Bearer JWTs)"
// @Param body body disruptions.RoadClosureInput true "Road closure payload"
// @Success 201 {object} disruptions.DisruptionResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Failure 502 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Failure 504 {object} map[string]string
// @Router /api/transport/v1/road-closures [post]
func (h *DisruptionHandler) CreateRoadClosure(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}

	defaultSource := roadClosureDefaultSource(r.Context())

	var input disruptions.RoadClosureInput
	if !decodeStrictJSONObject(w, r, &input) {
		h.auditDisruptionEvent(r.Context(), "create_road_closure", "disruption", 0, defaultSource, "invalid_request")
		return
	}

	created, err := h.service.CreateRoadClosure(r.Context(), input, defaultSource)
	if err != nil {
		h.auditDisruptionEvent(r.Context(), "create_road_closure", "disruption", 0, defaultSource, disruptionAuditOutcome(err))
		writeDisruptionError(w, err)
		return
	}

	w.Header().Set("Location", apiBaseFromRequest(r)+"/road-closures/"+strconv.Itoa(created.ID))
	h.auditDisruptionEvent(r.Context(), "create_road_closure", "disruption", created.ID, created.SourceService, "success")
	writeJSON(w, http.StatusCreated, created)
}

// UpdateRoadClosure applies partial updates (such as resolving or extending) to an existing road closure by ID.
//
// @Summary Update or resolve a road closure
// @Description Accepts a Registration Service service-account Bearer JWT (SH-S07; no cookie, Origin, or CSRF required) or an administrator session cookie with trusted Origin and X-CSRF-Token. Strictly parses JSON and partially updates or resolves an existing road closure (`type='road_closure'`) by id while preserving omitted fields and existing route/stop links. Supports valid-until (Unix epoch timestamp >= 1000000000, or duration in seconds from start_time/now, using the existing start_time when scheduled in the future).
// @Tags road-closures
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param X-CSRF-Token header string false "Session-bound CSRF token (required for browser cookie sessions; omitted for Bearer JWTs)"
// @Param id path int true "Positive road closure ID" minimum(1)
// @Param body body disruptions.UpdateRoadClosureInput true "Road closure partial update payload"
// @Success 200 {object} disruptions.DisruptionResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Failure 502 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Failure 504 {object} map[string]string
// @Router /api/transport/v1/road-closures/{id} [patch]
func (h *DisruptionHandler) UpdateRoadClosure(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}

	id, ok := parsePositivePathID(w, r, "id")
	if !ok {
		h.auditDisruptionEvent(r.Context(), "update_road_closure", "disruption", 0, "", "invalid_request")
		return
	}

	var input disruptions.UpdateRoadClosureInput
	keys, ok := decodeStrictJSONObjectWithKeys(w, r, &input)
	if !ok {
		h.auditDisruptionEvent(r.Context(), "update_road_closure", "disruption", id, "", "invalid_request")
		return
	}
	if _, hasEnd := keys["end_time"]; hasEnd {
		input.EndTimeSet = true
	}
	if _, hasDesc := keys["description"]; hasDesc {
		input.DescriptionSet = true
	}
	if _, hasArea := keys["affected_area"]; hasArea {
		input.AffectedAreaSet = true
	}

	updated, err := h.service.UpdateRoadClosure(r.Context(), id, input)
	if err != nil {
		h.auditDisruptionEvent(r.Context(), "update_road_closure", "disruption", id, "", disruptionAuditOutcome(err))
		writeDisruptionError(w, err)
		return
	}

	h.auditDisruptionEvent(r.Context(), "update_road_closure", "disruption", updated.ID, updated.SourceService, "success")
	writeJSON(w, http.StatusOK, updated)
}

func roadClosureDefaultSource(ctx context.Context) string {
	if _, hasSession := auth.SessionFromContext(ctx); hasSession {
		return disruptions.SourceTransitAdmin
	}
	if p, ok := auth.PrincipalFromContext(ctx); ok {
		candidate := strings.TrimSpace(p.Username)
		if candidate == "" {
			candidate = strings.TrimSpace(p.Subject)
		}
		if candidate != "" {
			if utf8.RuneCountInString(candidate) > disruptions.MaxSourceServiceRunes {
				runes := []rune(candidate)
				return strings.TrimSpace(string(runes[:disruptions.MaxSourceServiceRunes]))
			}
			return candidate
		}
	}
	return "external_service"
}

// Update applies partial updates to an existing transit disruption and its affected service links.
//
// @Summary Update a transit disruption
// @Description Requires an administrator session cookie, trusted Origin, and X-CSRF-Token. Updates disruption fields, operational status, and affected transit entities.
// @Tags disruptions
// @Accept json
// @Produce json
// @Param X-CSRF-Token header string true "Session-bound CSRF token"
// @Param id path int true "Positive disruption ID" minimum(1)
// @Param body body disruptions.UpdateDisruptionInput true "Disruption partial update payload"
// @Success 200 {object} disruptions.DisruptionResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/disruptions/{id} [patch]
func (h *DisruptionHandler) Update(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disruption service is unavailable"})
		return
	}

	id, ok := parsePositivePathID(w, r, "id")
	if !ok {
		h.auditDisruptionEvent(r.Context(), "update", "disruption", 0, "", "invalid_request")
		return
	}

	var input disruptions.UpdateDisruptionInput
	keys, ok := decodeStrictJSONObjectWithKeys(w, r, &input)
	if !ok {
		h.auditDisruptionEvent(r.Context(), "update", "disruption", id, "", "invalid_request")
		return
	}
	if _, hasEnd := keys["end_time"]; hasEnd {
		input.EndTimeSet = true
	}
	if _, hasDesc := keys["description"]; hasDesc {
		input.DescriptionSet = true
	}
	if _, hasArea := keys["affected_area"]; hasArea {
		input.AffectedAreaSet = true
	}

	updated, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.auditDisruptionEvent(r.Context(), "update", "disruption", id, "", disruptionAuditOutcome(err))
		writeDisruptionError(w, err)
		return
	}

	h.auditDisruptionEvent(r.Context(), "update", "disruption", updated.ID, updated.SourceService, "success")
	writeJSON(w, http.StatusOK, updated)
}

func (h *DisruptionHandler) auditDisruptionEvent(
	ctx context.Context,
	operation, resourceType string,
	resourceID int,
	sourceService, outcome string,
) {
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
		slog.String("operation", operation),
		slog.String("resource_type", resourceType),
		slog.String("outcome", outcome),
	}
	if resourceID > 0 {
		attrs = append(attrs, slog.Int("resource_id", resourceID))
	}
	if sourceService != "" {
		attrs = append(attrs, slog.String("source_service", sourceService))
	}
	h.audit.LogAttrs(ctx, slog.LevelInfo, "admin_disruption_operation", attrs...)
}

func disruptionAuditOutcome(err error) string {
	var invalid *disruptions.InvalidInput
	switch {
	case errors.As(err, &invalid):
		return "validation_error"
	case errors.Is(err, disruptions.ErrNotFound):
		return "not_found"
	default:
		return "internal_error"
	}
}

func parseDisruptionListQuery(r *http.Request, allowAllFilters bool) (disruptions.ListFilter, error) {
	var filter disruptions.ListFilter
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return filter, &disruptions.InvalidInput{Message: "invalid query string"}
	}

	allowed := map[string]bool{
		"type":     true,
		"route_id": true,
		"stop_id":  true,
		"trip_id":  true,
	}
	if allowAllFilters {
		allowed["status"] = true
		allowed["vehicle_id"] = true
		allowed["source_service"] = true
	}

	for key, entries := range values {
		if !allowed[key] || len(entries) != 1 || entries[0] == "" {
			return filter, &disruptions.InvalidInput{Message: "unknown, repeated, or empty query parameter: " + key}
		}
	}

	filter.Status = values.Get("status")
	filter.Type = values.Get("type")
	filter.SourceService = values.Get("source_service")

	for _, intParam := range []struct {
		name string
		dest *int
	}{
		{"route_id", &filter.RouteID},
		{"stop_id", &filter.StopID},
		{"trip_id", &filter.TripID},
		{"vehicle_id", &filter.VehicleID},
	} {
		if value := values.Get(intParam.name); value != "" {
			id, err := strconv.ParseInt(value, 10, 32)
			if err != nil || id <= 0 {
				return filter, &disruptions.InvalidInput{Message: intParam.name + " must be a positive 32-bit integer"}
			}
			*intParam.dest = int(id)
		}
	}
	return filter, nil
}

func rejectUnexpectedDisruptionQuery(r *http.Request) error {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return &disruptions.InvalidInput{Message: "invalid query string"}
	}
	for key := range values {
		return &disruptions.InvalidInput{Message: "unknown query parameter: " + key}
	}
	return nil
}

func decodeStrictJSONObjectWithKeys(w http.ResponseWriter, r *http.Request, target any) (map[string]json.RawMessage, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); err != nil || object == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be a JSON object"})
		return nil, false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must contain exactly one JSON object"})
		return nil, false
	}
	data, _ := json.Marshal(object)
	fields := json.NewDecoder(bytes.NewReader(data))
	fields.DisallowUnknownFields()
	if err := fields.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body contains an unknown field or invalid field type"})
		return nil, false
	}
	return object, true
}

func writeDisruptionError(w http.ResponseWriter, err error) {
	var invalid *disruptions.InvalidInput
	switch {
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": invalid.Message})
	case errors.Is(err, disruptions.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "transit resource not found"})
	default:
		log.Printf("disruption operation failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to complete disruption operation"})
	}
}

func apiBaseFromRequest(r *http.Request) string {
	const prefix = "/api/transport/"
	if r != nil && strings.HasPrefix(r.URL.Path, prefix) {
		rest := strings.TrimPrefix(r.URL.Path, prefix)
		if idx := strings.IndexByte(rest, '/'); idx > 0 {
			return prefix + rest[:idx]
		}
		if rest != "" {
			return prefix + rest
		}
	}
	return "/api/transport/v1"
}
