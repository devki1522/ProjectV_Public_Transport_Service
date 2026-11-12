package handlers

import (
	"context"
	"log"
	"net/http"

	"github.com/devki1522/ProjectV_Public_Transport_Service/backend/internal/services"
)

// DashboardReader describes the dashboard operations needed by AdminHandler.
type DashboardReader interface {
	Summary(context.Context) (services.DashboardSummary, error)
}

// AdminHandler serves administrator health and dashboard endpoints.
type AdminHandler struct {
	dashboard DashboardReader
}

// NewAdminHandler creates an AdminHandler backed by the supplied dashboard service.
func NewAdminHandler(dashboard DashboardReader) *AdminHandler {
	return &AdminHandler{dashboard: dashboard}
}

// Health confirms administrator authentication and authorization succeeded.
// @Summary Administrator health
// @Tags administration
// @Description Requires a valid administrator session cookie.
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/admin/health [get]
func (h *AdminHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Dashboard returns real route counts from PostgreSQL.
// @Summary Administrator dashboard
// @Tags administration
// @Description Requires a valid administrator session cookie.
// @Produce json
// @Success 200 {object} services.DashboardSummary
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/admin/dashboard [get]
func (h *AdminHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	summary, err := h.dashboard.Summary(r.Context())
	if err != nil {
		log.Printf("admin dashboard failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to load dashboard summary"})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
