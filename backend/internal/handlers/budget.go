package handlers

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/auth"
	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/budget"
)

// BudgetService describes Budget & Finance integration operations required by administrator HTTP handlers.
type BudgetService interface {
	GetAllocation(ctx context.Context) (budget.Allocation, error)
	SubmitExpense(ctx context.Context, input budget.ExpenseInput) (budget.ExpenseRecord, error)
	SubmitRevenue(ctx context.Context, input budget.RevenueInput) (budget.RevenueRecord, error)
	SubmitFundingRequest(ctx context.Context, input budget.FundingRequestInput) (budget.FundingRequestRecord, error)
}

// BudgetHandler serves administrator-only Budget & Finance allocation, expense, revenue, and funding request endpoints.
type BudgetHandler struct {
	service BudgetService
	audit   *slog.Logger
}

// NewBudgetHandler creates a BudgetHandler with default JSON audit logging to stdout.
func NewBudgetHandler(service BudgetService) *BudgetHandler {
	return NewBudgetHandlerWithAudit(service, nil)
}

// NewBudgetHandlerWithAudit creates a BudgetHandler with an explicit structured audit logger.
func NewBudgetHandlerWithAudit(service BudgetService, audit *slog.Logger) *BudgetHandler {
	if audit == nil {
		audit = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return &BudgetHandler{
		service: service,
		audit:   audit,
	}
}

// Allocation returns the transit department operating budget allocation from Budget & Finance.
//
// @Summary Get transit operating budget allocation
// @Description Requires an administrator session cookie. Queries Budget & Finance GET /api/budget/v1/allocations/transit.
// @Tags budget
// @Produce json
// @Success 200 {object} budget.Allocation
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Failure 504 {object} map[string]string
// @Router /api/transport/v1/admin/budget [get]
func (h *BudgetHandler) Allocation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "budget integration is not configured"})
		return
	}
	alloc, err := h.service.GetAllocation(r.Context())
	if err != nil {
		writeBudgetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, alloc)
}

// SubmitExpense forwards a transit operating expense to Budget & Finance.
//
// @Summary Submit transit operating expense
// @Description Requires an administrator session cookie, trusted Origin, and X-CSRF-Token. Forwards to POST /api/budget/v1/expenses.
// @Tags budget
// @Accept json
// @Produce json
// @Param X-CSRF-Token header string true "Session-bound CSRF token"
// @Param body body budget.ExpenseInput true "Operating expense payload"
// @Success 201 {object} budget.ExpenseRecord
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Failure 504 {object} map[string]string
// @Router /api/transport/v1/admin/budget/expenses [post]
func (h *BudgetHandler) SubmitExpense(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "budget integration is not configured"})
		return
	}
	var input budget.ExpenseInput
	if !decodeStrictJSONObject(w, r, &input) {
		h.auditBudgetEvent(r.Context(), "expense", "invalid_request")
		return
	}
	record, err := h.service.SubmitExpense(r.Context(), input)
	if err != nil {
		h.auditBudgetEvent(r.Context(), "expense", "failure")
		writeBudgetError(w, err)
		return
	}
	h.auditBudgetEvent(r.Context(), "expense", "success")
	writeJSON(w, http.StatusCreated, record)
}

// SubmitRevenue forwards transit fare revenue to Budget & Finance.
//
// @Summary Report transit fare revenue
// @Description Requires an administrator session cookie, trusted Origin, and X-CSRF-Token. Forwards to POST /api/budget/v1/revenue.
// @Tags budget
// @Accept json
// @Produce json
// @Param X-CSRF-Token header string true "Session-bound CSRF token"
// @Param body body budget.RevenueInput true "Fare revenue payload"
// @Success 201 {object} budget.RevenueRecord
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Failure 504 {object} map[string]string
// @Router /api/transport/v1/admin/budget/revenue [post]
func (h *BudgetHandler) SubmitRevenue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "budget integration is not configured"})
		return
	}
	var input budget.RevenueInput
	if !decodeStrictJSONObject(w, r, &input) {
		h.auditBudgetEvent(r.Context(), "revenue", "invalid_request")
		return
	}
	record, err := h.service.SubmitRevenue(r.Context(), input)
	if err != nil {
		h.auditBudgetEvent(r.Context(), "revenue", "failure")
		writeBudgetError(w, err)
		return
	}
	h.auditBudgetEvent(r.Context(), "revenue", "success")
	writeJSON(w, http.StatusCreated, record)
}

// SubmitFundingRequest forwards an additional funding request to Budget & Finance.
//
// @Summary Request additional transit funding
// @Description Requires an administrator session cookie, trusted Origin, and X-CSRF-Token. Forwards to POST /api/budget/v1/funding-requests.
// @Tags budget
// @Accept json
// @Produce json
// @Param X-CSRF-Token header string true "Session-bound CSRF token"
// @Param body body budget.FundingRequestInput true "Funding request payload"
// @Success 201 {object} budget.FundingRequestRecord
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Failure 504 {object} map[string]string
// @Router /api/transport/v1/admin/budget/funding-requests [post]
func (h *BudgetHandler) SubmitFundingRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "budget integration is not configured"})
		return
	}
	var input budget.FundingRequestInput
	if !decodeStrictJSONObject(w, r, &input) {
		h.auditBudgetEvent(r.Context(), "funding_request", "invalid_request")
		return
	}
	record, err := h.service.SubmitFundingRequest(r.Context(), input)
	if err != nil {
		h.auditBudgetEvent(r.Context(), "funding_request", "failure")
		writeBudgetError(w, err)
		return
	}
	h.auditBudgetEvent(r.Context(), "funding_request", "success")
	writeJSON(w, http.StatusCreated, record)
}

func (h *BudgetHandler) auditBudgetEvent(ctx context.Context, operation, outcome string) {
	if h.audit == nil {
		return
	}
	subject := "unknown"
	if principal, ok := auth.PrincipalFromContext(ctx); ok && principal.Subject != "" {
		subject = principal.Subject
	}
	h.audit.Info(
		"admin_budget_action",
		slog.String("subject", subject),
		slog.String("operation", operation),
		slog.String("outcome", outcome),
		slog.Time("timestamp", time.Now().UTC()),
	)
}

func writeBudgetError(w http.ResponseWriter, err error) {
	var invalid *budget.InvalidInput
	var rejected *budget.RejectedError
	var unavailable *budget.UpstreamUnavailableError
	var timeout *budget.UpstreamTimeoutError

	switch {
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": invalid.Error()})
	case errors.As(err, &rejected):
		writeJSON(w, rejected.StatusCode, map[string]string{"error": rejected.Error()})
	case errors.As(err, &timeout):
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": timeout.Error()})
	case errors.As(err, &unavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": unavailable.Error()})
	default:
		log.Printf("budget operation failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
