package handlers

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/auth"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/staffhr"
)

// StaffHRService describes Staff/HR integration operations required by administrator HTTP handlers.
type StaffHRService interface {
	Overview(ctx context.Context, departmentCode string) (staffhr.StaffOverview, error)
	AddEmployee(ctx context.Context, input staffhr.AddEmployeeInput) (staffhr.StaffMember, error)
}

// StaffHRHandler serves administrator-only Staff/HR employee count and onboarding endpoints.
type StaffHRHandler struct {
	service StaffHRService
	audit   *slog.Logger
}

// NewStaffHRHandler creates a StaffHRHandler with default JSON audit logging to stdout.
func NewStaffHRHandler(service StaffHRService) *StaffHRHandler {
	return NewStaffHRHandlerWithAudit(service, nil)
}

// NewStaffHRHandlerWithAudit creates a StaffHRHandler with an explicit structured audit logger.
func NewStaffHRHandlerWithAudit(service StaffHRService, audit *slog.Logger) *StaffHRHandler {
	if audit == nil {
		audit = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return &StaffHRHandler{
		service: service,
		audit:   audit,
	}
}

// Overview returns department employee counts and staff directory entries from CMES Staff/HR.
//
// @Summary Get Staff/HR employee count and directory
// @Description Requires an administrator session cookie. Queries CMES Staff/HR department headcounts and staff directory (defaults to PTR).
// @Tags staffhr
// @Produce json
// @Param department query string false "Department code filter (defaults to configured PTR)"
// @Success 200 {object} staffhr.StaffOverview
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Failure 504 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/admin/staff [get]
func (h *StaffHRHandler) Overview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "staff/HR integration is not configured"})
		return
	}

	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeStaffHRError(w, &staffhr.InvalidInput{Message: "invalid query string"})
		return
	}
	for key, entries := range values {
		if key != "department" || len(entries) != 1 || entries[0] == "" {
			writeStaffHRError(w, &staffhr.InvalidInput{Message: "unknown, repeated, or empty query parameter: " + key})
			return
		}
	}

	overview, err := h.service.Overview(r.Context(), values.Get("department"))
	if err != nil {
		writeStaffHRError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

// AddEmployee validates and submits a new employee onboarding request to CMES Staff/HR.
//
// @Summary Add an employee to CMES Staff/HR
// @Description Requires an administrator session cookie, trusted Origin, and X-CSRF-Token. Forwards employee onboarding to CMES Staff/HR via POST /staff.
// @Tags staffhr
// @Accept json
// @Produce json
// @Param X-CSRF-Token header string true "Session-bound CSRF token"
// @Param body body staffhr.AddEmployeeInput true "Employee onboarding payload"
// @Success 201 {object} staffhr.StaffMember
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Failure 504 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/admin/staff/employees [post]
func (h *StaffHRHandler) AddEmployee(w http.ResponseWriter, r *http.Request) {
	h.handleAddEmployee(w, r, "create")
}

// PutEmployee validates and submits an employee onboarding request via HTTP PUT to CMES Staff/HR.
//
// @Summary Add an employee to CMES Staff/HR (PUT)
// @Description Requires an administrator session cookie, trusted Origin, and X-CSRF-Token. Equivalent to POST /admin/staff/employees for REST clients using PUT.
// @Tags staffhr
// @Accept json
// @Produce json
// @Param X-CSRF-Token header string true "Session-bound CSRF token"
// @Param body body staffhr.AddEmployeeInput true "Employee onboarding payload"
// @Success 201 {object} staffhr.StaffMember
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Failure 504 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/transport/v1/admin/staff/employees [put]
func (h *StaffHRHandler) PutEmployee(w http.ResponseWriter, r *http.Request) {
	h.handleAddEmployee(w, r, "put")
}

func (h *StaffHRHandler) handleAddEmployee(w http.ResponseWriter, r *http.Request, operation string) {
	w.Header().Set("Cache-Control", "no-store")
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "staff/HR integration is not configured"})
		return
	}

	var input staffhr.AddEmployeeInput
	if !decodeStrictJSONObject(w, r, &input) {
		h.auditStaffEvent(r.Context(), operation, "", "invalid_request")
		return
	}

	created, err := h.service.AddEmployee(r.Context(), input)
	if err != nil {
		h.auditStaffEvent(r.Context(), operation, "", staffAuditOutcome(err))
		writeStaffHRError(w, err)
		return
	}

	h.auditStaffEvent(r.Context(), operation, created.StaffNumber, "success")
	writeJSON(w, http.StatusCreated, created)
}

func (h *StaffHRHandler) auditStaffEvent(ctx context.Context, operation, staffNumber, outcome string) {
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
		slog.String("resource_type", "staff_employee"),
		slog.String("outcome", outcome),
	}
	if staffNumber != "" {
		attrs = append(attrs, slog.String("staff_number", staffNumber))
	}
	h.audit.LogAttrs(ctx, slog.LevelInfo, "admin_staff_add", attrs...)
}

func staffAuditOutcome(err error) string {
	var invalid *staffhr.InvalidInput
	switch {
	case errors.As(err, &invalid):
		return "validation_error"
	case errors.Is(err, staffhr.ErrConflict):
		return "conflict"
	case errors.Is(err, staffhr.ErrNotFound):
		return "not_found"
	case errors.Is(err, staffhr.ErrUpstreamTimeout):
		return "upstream_timeout"
	case errors.Is(err, staffhr.ErrNotConfigured), errors.Is(err, staffhr.ErrUpstreamUnavailable):
		return "upstream_unavailable"
	default:
		return "internal_error"
	}
}

func writeStaffHRError(w http.ResponseWriter, err error) {
	var invalid *staffhr.InvalidInput
	switch {
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": invalid.Message})
	case errors.Is(err, staffhr.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "requested staff or department was not found"})
	case errors.Is(err, staffhr.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, staffhr.ErrUpstreamTimeout):
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "staff/HR service timed out; please try again"})
	case errors.Is(err, staffhr.ErrNotConfigured), errors.Is(err, staffhr.ErrUpstreamUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "staff/HR service is currently unavailable"})
	default:
		log.Printf("staff/HR operation failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to complete staff/HR request"})
	}
}
