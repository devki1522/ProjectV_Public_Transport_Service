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
	"os"
	"strconv"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/auth"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/models"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/services"
	"github.com/go-chi/chi/v5"
)

// TransitRouteService describes what these HTTP handlers need from the application layer.
type TransitRouteService interface {
	List(context.Context) ([]models.Route, error)
	Get(context.Context, int) (models.Route, error)
	Create(context.Context, services.CreateTransitRouteInput) (models.Route, error)
	Update(context.Context, int, services.UpdateTransitRouteInput) (models.Route, error)
	Deactivate(context.Context, int) error
}

type TransitRouteHandler struct {
	service TransitRouteService
	audit   *slog.Logger
}

func NewTransitRouteHandler(service TransitRouteService) *TransitRouteHandler {
	return &TransitRouteHandler{
		service: service,
		audit:   slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	}
}

// NewTransitRouteHandlerWithAudit creates a TransitRouteHandler with an explicit structured audit logger.
func NewTransitRouteHandlerWithAudit(service TransitRouteService, audit *slog.Logger) *TransitRouteHandler {
	if audit == nil {
		audit = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return &TransitRouteHandler{service: service, audit: audit}
}

// TransitRouteResponse intentionally excludes GORM associations from API responses.
type TransitRouteResponse struct {
	ID        int     `json:"id"`
	Number    string  `json:"number"`
	Name      string  `json:"name"`
	Direction string  `json:"direction"`
	IsActive  bool    `json:"is_active"`
	SourceID  *string `json:"source_id"`
	RouteType *int    `json:"route_type"`
}

func transitRouteResponse(route models.Route) TransitRouteResponse {
	return TransitRouteResponse{route.ID, route.Number, route.Name, route.Direction, route.IsActive, route.SourceID, route.RouteType}
}

func (h *TransitRouteHandler) List(w http.ResponseWriter, r *http.Request) {
	routes, err := h.service.List(r.Context())
	if err != nil {
		writeTransitRouteError(w, err)
		return
	}
	response := make([]TransitRouteResponse, 0, len(routes))
	for _, route := range routes {
		response = append(response, transitRouteResponse(route))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *TransitRouteHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := transitRouteID(w, r)
	if !ok {
		return
	}
	route, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeTransitRouteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, transitRouteResponse(route))
}

func (h *TransitRouteHandler) Create(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input services.CreateTransitRouteInput
	if !decodeTransitRouteInput(w, r, &input) {
		h.auditRouteMutation(r.Context(), "create", 0, "invalid_request")
		return
	}
	route, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.auditRouteMutation(r.Context(), "create", 0, auditOutcome(err))
		writeTransitRouteError(w, err)
		return
	}
	h.auditRouteMutation(r.Context(), "create", route.ID, "success")
	w.Header().Set("Location", "/api/routes/"+strconv.Itoa(route.ID))
	writeJSON(w, http.StatusCreated, transitRouteResponse(route))
}

func (h *TransitRouteHandler) Update(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id, ok := transitRouteID(w, r)
	if !ok {
		h.auditRouteMutation(r.Context(), "update", 0, "invalid_request")
		return
	}
	var input services.UpdateTransitRouteInput
	if !decodeTransitRouteInput(w, r, &input) {
		h.auditRouteMutation(r.Context(), "update", id, "invalid_request")
		return
	}
	route, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.auditRouteMutation(r.Context(), "update", id, auditOutcome(err))
		writeTransitRouteError(w, err)
		return
	}
	h.auditRouteMutation(r.Context(), "update", route.ID, "success")
	writeJSON(w, http.StatusOK, transitRouteResponse(route))
}

func (h *TransitRouteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id, ok := transitRouteID(w, r)
	if !ok {
		h.auditRouteMutation(r.Context(), "deactivate", 0, "invalid_request")
		return
	}
	if err := h.service.Deactivate(r.Context(), id); err != nil {
		h.auditRouteMutation(r.Context(), "deactivate", id, auditOutcome(err))
		writeTransitRouteError(w, err)
		return
	}
	h.auditRouteMutation(r.Context(), "deactivate", id, "success")
	w.WriteHeader(http.StatusNoContent)
}

func (h *TransitRouteHandler) auditRouteMutation(ctx context.Context, operation string, resourceID int, outcome string) {
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
		slog.String("resource_type", "route"),
		slog.String("outcome", outcome),
	}
	if resourceID > 0 {
		attrs = append(attrs, slog.Int("resource_id", resourceID))
	}
	h.audit.LogAttrs(ctx, slog.LevelInfo, "admin_route_mutation", attrs...)
}

func auditOutcome(err error) string {
	var validation *services.ValidationError
	switch {
	case errors.As(err, &validation):
		return "validation_error"
	case errors.Is(err, services.ErrTransitRouteNotFound):
		return "not_found"
	case errors.Is(err, services.ErrTransitRouteConflict):
		return "conflict"
	default:
		return "internal_error"
	}
}

func transitRouteID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 32)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id must be a positive 32-bit integer"})
		return 0, false
	}
	return int(id), true
}

func decodeTransitRouteInput(w http.ResponseWriter, r *http.Request, input any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	// Decode a raw object first so JSON null cannot silently become an empty input.
	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); err != nil || object == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be a JSON object"})
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must contain exactly one JSON object"})
		return false
	}
	for _, value := range object {
		if string(value) == "null" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "fields cannot be null; omit unchanged fields"})
			return false
		}
	}
	data, _ := json.Marshal(object)
	// Use a second strict decoder to reject unknown fields and invalid field types.
	fields := json.NewDecoder(bytes.NewReader(data))
	fields.DisallowUnknownFields()
	if err := fields.Decode(input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body contains an unknown field or invalid field type"})
		return false
	}
	return true
}

func writeTransitRouteError(w http.ResponseWriter, err error) {
	var validation *services.ValidationError
	switch {
	case errors.As(err, &validation):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": validation.Error()})
	case errors.Is(err, services.ErrTransitRouteNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, services.ErrTransitRouteConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		log.Printf("route operation failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to complete route operation"})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("encode response: %v", err)
	}
}
