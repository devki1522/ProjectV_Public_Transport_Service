package vehicles

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memoryVehicleRepo struct {
	mu          sync.Mutex
	routes      []SimulatedRoute
	nextID      int
	vehicleByID map[int]ActiveVehicle
	numToID     map[string]int
}

func newMemoryVehicleRepo(routes []SimulatedRoute) *memoryVehicleRepo {
	return &memoryVehicleRepo{
		routes:      routes,
		nextID:      1,
		vehicleByID: make(map[int]ActiveVehicle),
		numToID:     make(map[string]int),
	}
}

func (m *memoryVehicleRepo) UpsertLocation(_ context.Context, p LocationUpdateParams) (LocationUpdateResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id, exists := m.numToID[p.VehicleNumber]
	created := !exists
	if !exists {
		id = m.nextID
		m.nextID++
		m.numToID[p.VehicleNumber] = id
	}

	var routeInfo SimulatedRoute
	for _, r := range m.routes {
		if r.RouteID == p.RouteID {
			routeInfo = r
			break
		}
	}

	item := ActiveVehicle{
		VehicleID:     id,
		VehicleNumber: p.VehicleNumber,
		IsAccessible:  p.IsAccessible,
		RouteID:       p.RouteID,
		RouteNumber:   routeInfo.RouteNumber,
		RouteName:     routeInfo.RouteName,
		Direction:     routeInfo.Direction,
		TripID:        p.TripID,
		Latitude:      p.Latitude,
		Longitude:     p.Longitude,
		Heading:       p.Heading,
		SpeedKmh:      p.SpeedKmh,
		Status:        p.Status,
		SourceService: p.SourceService,
		IsSimulated:   p.IsSimulated,
		RecordedAt:    p.RecordedAt,
		ReceivedAt:    p.ReceivedAt,
	}
	m.vehicleByID[id] = item
	return LocationUpdateResult{Vehicle: item, Created: created}, nil
}

func (m *memoryVehicleRepo) ListActiveVehicles(_ context.Context, cutoff time.Time, routeID int, _ string, _ int) ([]ActiveVehicle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ActiveVehicle, 0, len(m.vehicleByID))
	for _, v := range m.vehicleByID {
		if v.RecordedAt.Before(cutoff) {
			continue
		}
		if routeID != 0 && v.RouteID != routeID {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

func (m *memoryVehicleRepo) ListActiveVehiclesForStop(ctx context.Context, cutoff time.Time, _ int, routeID int) ([]ActiveVehicle, error) {
	return m.ListActiveVehicles(ctx, cutoff, routeID, "", 0)
}

func (m *memoryVehicleRepo) RouteExists(context.Context, int) error { return nil }
func (m *memoryVehicleRepo) TripExists(context.Context, int) error  { return nil }
func (m *memoryVehicleRepo) StopExists(context.Context, int) error  { return nil }
func (m *memoryVehicleRepo) RouteGeometry(context.Context, int) (RouteGeometry, error) {
	return RouteGeometry{}, nil
}
func (m *memoryVehicleRepo) LoadTripStops(context.Context, []int) ([]TripStopPoint, error) {
	return []TripStopPoint{}, nil
}
func (m *memoryVehicleRepo) LoadRouteStops(context.Context, []int) ([]RouteStopPoint, error) {
	return []RouteStopPoint{}, nil
}

func (m *memoryVehicleRepo) ActiveRoutesWithGeometries(context.Context, int) ([]SimulatedRoute, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]SimulatedRoute(nil), m.routes...), nil
}

func (m *memoryVehicleRepo) ClearSimulatedLocations(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, v := range m.vehicleByID {
		if v.IsSimulated {
			delete(m.vehicleByID, id)
		}
	}
	return nil
}

func TestSimulatorStepMovesBusesAndComputesHeadings(t *testing.T) {
	now := time.Date(2026, 11, 18, 17, 0, 0, 0, time.UTC)
	tripID := 501
	repo := newMemoryVehicleRepo([]SimulatedRoute{
		{
			RouteGeometry: RouteGeometry{
				RouteID:     7,
				RouteNumber: "7",
				RouteName:   "King",
				Direction:   "direction_0",
				Coordinates: []Coordinate{
					{Lat: 43.4500, Lng: -80.4900},
					{Lat: 43.4700, Lng: -80.5100},
				},
			},
			SampleTripID: &tripID,
		},
	})
	svc := NewServiceWithClock(repo, func() time.Time { return now })
	sim := NewSimulator(svc, SimulatorConfig{
		VehicleCount: 2,
		TickInterval: 5 * time.Second,
		Now:          func() time.Time { return now },
	})
	defer sim.Close()

	status1, err := sim.Step(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status1.VehicleCount != 2 || status1.RouteCount != 1 || status1.TotalUpdatesSent != 2 {
		t.Fatalf("unexpected status after first step: %+v", status1)
	}

	firstSnapshot, err := svc.ListActiveVehicles(context.Background(), ActiveVehiclesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(firstSnapshot) != 2 {
		t.Fatalf("expected 2 simulated buses, got %d", len(firstSnapshot))
	}
	for _, v := range firstSnapshot {
		if !v.IsSimulated || v.SourceService != "simulator" || v.Heading == nil || *v.Heading < 0 || *v.Heading > 360 {
			t.Fatalf("invalid simulated vehicle: %+v", v)
		}
	}

	now = now.Add(5 * time.Second)
	status2, err := sim.Step(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status2.TotalUpdatesSent != 4 {
		t.Fatalf("total updates = %d, want 4", status2.TotalUpdatesSent)
	}

	secondSnapshot, _ := svc.ListActiveVehicles(context.Background(), ActiveVehiclesQuery{})
	byNum := make(map[string]ActiveVehicle, len(secondSnapshot))
	for _, v := range secondSnapshot {
		byNum[v.VehicleNumber] = v
	}
	for _, prev := range firstSnapshot {
		curr := byNum[prev.VehicleNumber]
		if curr.Latitude == prev.Latitude && curr.Longitude == prev.Longitude {
			t.Fatalf("expected bus %s to move on step, stayed at (%v, %v)", prev.VehicleNumber, prev.Latitude, prev.Longitude)
		}
	}

	// Verify Reset clears simulated positions.
	resetStatus, err := sim.Reset(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resetStatus.VehicleCount != 0 || resetStatus.TotalUpdatesSent != 0 {
		t.Fatalf("unexpected reset status: %+v", resetStatus)
	}
	afterReset, _ := svc.ListActiveVehicles(context.Background(), ActiveVehiclesQuery{})
	if len(afterReset) != 0 {
		t.Fatalf("expected 0 simulated vehicles after reset, got %d", len(afterReset))
	}
}

func TestSimulatorLazyLoadsAfterRoutesSeededAndStartStop(t *testing.T) {
	now := time.Date(2026, 11, 18, 17, 0, 0, 0, time.UTC)
	repo := newMemoryVehicleRepo(nil)
	svc := NewServiceWithClock(repo, func() time.Time { return now })
	sim := NewSimulator(svc, SimulatorConfig{
		VehicleCount: 1,
		TickInterval: 500 * time.Millisecond,
		Now:          func() time.Time { return now },
	})
	defer sim.Close()

	// Starting before routes exist does not fail; routes may be seeded afterwards.
	status, err := sim.Start(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Running || status.VehicleCount != 0 {
		t.Fatalf("expected running empty simulator before seeding, got %+v", status)
	}

	repo.mu.Lock()
	repo.routes = []SimulatedRoute{
		{
			RouteGeometry: RouteGeometry{
				RouteID:     1,
				RouteNumber: "1",
				RouteName:   "Queen-River",
				Direction:   "direction_0",
				Coordinates: []Coordinate{
					{Lat: 43.44, Lng: -80.49},
					{Lat: 43.45, Lng: -80.48},
				},
			},
		},
	}
	repo.mu.Unlock()

	stepStatus, err := sim.Step(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stepStatus.VehicleCount != 1 {
		t.Fatalf("expected simulator to lazy-load bus after routes seeded, got %+v", stepStatus)
	}

	stopped := sim.Stop()
	if stopped.Running {
		t.Fatalf("expected simulator stopped, got %+v", stopped)
	}

	if _, err := sim.Start(floatPtr(20)); err == nil {
		t.Fatal("expected invalid speed multiplier error")
	}
}
