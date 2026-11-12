package services

import (
	"context"

	"gorm.io/gorm"
)

// DashboardSummary holds real transit route inventory counts from PostgreSQL.
type DashboardSummary struct {
	TotalRoutes    int64 `json:"total_routes"`
	ActiveRoutes   int64 `json:"active_routes"`
	InactiveRoutes int64 `json:"inactive_routes"`
}

// DashboardRepository queries route counts for the administrator dashboard.
type DashboardRepository interface {
	RouteCounts(context.Context) (DashboardSummary, error)
}

// PostgresDashboardRepository implements DashboardRepository using GORM and PostgreSQL.
type PostgresDashboardRepository struct {
	db *gorm.DB
}

// NewPostgresDashboardRepository creates a PostgreSQL-backed dashboard repository.
func NewPostgresDashboardRepository(db *gorm.DB) *PostgresDashboardRepository {
	return &PostgresDashboardRepository{db: db}
}

// RouteCounts returns the total, active, and inactive route counts from the routes table.
func (r *PostgresDashboardRepository) RouteCounts(ctx context.Context) (DashboardSummary, error) {
	var summary DashboardSummary
	err := r.db.WithContext(ctx).
		Table("routes").
		Select(
			"COUNT(*) AS total_routes, " +
				"COALESCE(SUM(CASE WHEN is_active THEN 1 ELSE 0 END), 0) AS active_routes, " +
				"COALESCE(SUM(CASE WHEN NOT is_active THEN 1 ELSE 0 END), 0) AS inactive_routes",
		).
		Scan(&summary).Error
	return summary, err
}

// AdminDashboardService provides administrator dashboard metrics from the repository.
type AdminDashboardService struct {
	repo DashboardRepository
}

// NewAdminDashboardService creates an AdminDashboardService backed by repo.
func NewAdminDashboardService(repo DashboardRepository) *AdminDashboardService {
	return &AdminDashboardService{repo: repo}
}

// Summary retrieves the current dashboard summary from the repository.
func (s *AdminDashboardService) Summary(ctx context.Context) (DashboardSummary, error) {
	return s.repo.RouteCounts(ctx)
}
