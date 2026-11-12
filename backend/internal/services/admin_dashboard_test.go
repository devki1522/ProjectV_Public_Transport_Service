package services

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type fakeDashboardRepo struct {
	summary DashboardSummary
	err     error
	called  bool
}

func (f *fakeDashboardRepo) RouteCounts(context.Context) (DashboardSummary, error) {
	f.called = true
	return f.summary, f.err
}

func TestAdminDashboardServiceReturnsRepositoryCounts(t *testing.T) {
	repo := &fakeDashboardRepo{
		summary: DashboardSummary{TotalRoutes: 12, ActiveRoutes: 9, InactiveRoutes: 3},
	}
	service := NewAdminDashboardService(repo)

	got, err := service.Summary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !repo.called {
		t.Fatal("expected repository to be called")
	}
	if got != repo.summary {
		t.Fatalf("Summary() = %+v, want %+v", got, repo.summary)
	}
}

func TestAdminDashboardServicePropagatesErrors(t *testing.T) {
	wantErr := errors.New("database error")
	service := NewAdminDashboardService(&fakeDashboardRepo{err: wantErr})
	if _, err := service.Summary(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("got err = %v, want %v", err, wantErr)
	}
}

func TestPostgresDashboardRepositoryRouteCounts(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	rows := sqlmock.NewRows([]string{"total_routes", "active_routes", "inactive_routes"}).
		AddRow(10, 8, 2)
	mock.ExpectQuery(`SELECT COUNT\(\*\) AS total_routes.*FROM "routes"`).WillReturnRows(rows)

	repo := NewPostgresDashboardRepository(db)
	got, err := repo.RouteCounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalRoutes != 10 || got.ActiveRoutes != 8 || got.InactiveRoutes != 2 {
		t.Fatalf("unexpected counts: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
