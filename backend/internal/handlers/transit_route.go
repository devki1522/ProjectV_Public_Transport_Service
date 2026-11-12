package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

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

// TransitRouteHandler serves HTTP requests for transit route management.
type TransitRouteHandler struct{ service TransitRouteService }

// NewTransitRouteHandler creates a route HTTP handler for the given service.
func NewTransitRouteHandler(service TransitRouteService) *TransitRouteHandler {
	return &TransitRouteHandler{service: service}
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

// List returns the transit routes available to clients.
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

// Get returns one transit route by identifier.
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

// Create validates and creates a transit route.
func (h *TransitRouteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input services.CreateTransitRouteInput
	if !decodeTransitRouteInput(w, r, &input) {
		return
	}
	route, err := h.service.Create(r.Context(), input)
	if err != nil {
		writeTransitRouteError(w, err)
		return
	}
	w.Header().Set("Location", "/api/routes/"+strconv.Itoa(route.ID))
	writeJSON(w, http.StatusCreated, transitRouteResponse(route))
}

// Update validates and updates a transit route.
func (h *TransitRouteHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := transitRouteID(w, r)
	if !ok {
		return
	}
	var input services.UpdateTransitRouteInput
	if !decodeTransitRouteInput(w, r, &input) {
		return
	}
	route, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		writeTransitRouteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, transitRouteResponse(route))
}

// Delete deactivates a transit route without removing related data.
func (h *TransitRouteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := transitRouteID(w, r)
	if !ok {
		return
	}
	if err := h.service.Deactivate(r.Context(), id); err != nil {
		writeTransitRouteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
