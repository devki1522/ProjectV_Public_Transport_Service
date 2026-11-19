package vehicles

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

const (
	defaultSpeedMultiplier = 1.0
	minSpeedMultiplier     = 0.25
	maxSpeedMultiplier     = 10.0
	earthRadiusMeters      = 6371000.0
)

// SimulatorConfig configures the built-in live bus movement simulator.
type SimulatorConfig struct {
	VehicleCount int
	TickInterval time.Duration
	Now          func() time.Time
}

type pathSegment struct {
	start          Coordinate
	end            Coordinate
	startDistM     float64
	endDistM       float64
	distanceMeters float64
	headingDeg     float64
}

type simulatedBus struct {
	vehicleNumber  string
	isAccessible   bool
	route          SimulatedRoute
	segments       []pathSegment
	totalMeters    float64
	distanceMeters float64
	direction      float64
	baseSpeedKmh   float64
	layoverTicks   int
	tickCount      int
}

// Simulator periodically moves simulated transit vehicles along real route geometries via Service.SubmitLocation.
type Simulator struct {
	service      *Service
	vehicleCount int
	tickInterval time.Duration
	now          func() time.Time

	mu              sync.Mutex
	baseCtx         context.Context
	baseCancel      context.CancelFunc
	runCancel       context.CancelFunc
	wg              sync.WaitGroup
	running         bool
	speedMultiplier float64
	buses           []simulatedBus
	lastTickAt      *time.Time
	totalUpdates    int64
}

// NewSimulator constructs a live bus simulator backed by service.
func NewSimulator(service *Service, cfg SimulatorConfig) *Simulator {
	count := cfg.VehicleCount
	if count <= 0 {
		count = 24
	}
	interval := cfg.TickInterval
	if interval <= 0 {
		interval = 3 * time.Second
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	baseCtx, baseCancel := context.WithCancel(context.Background())
	return &Simulator{
		service:         service,
		vehicleCount:    count,
		tickInterval:    interval,
		now:             now,
		baseCtx:         baseCtx,
		baseCancel:      baseCancel,
		speedMultiplier: defaultSpeedMultiplier,
	}
}

// Status returns a snapshot of the simulator state.
func (s *Simulator) Status() SimulatorStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *Simulator) statusLocked() SimulatorStatus {
	routeSet := make(map[int]struct{}, len(s.buses))
	for _, bus := range s.buses {
		routeSet[bus.route.RouteID] = struct{}{}
	}
	var lastTick *time.Time
	if s.lastTickAt != nil {
		t := *s.lastTickAt
		lastTick = &t
	}
	return SimulatorStatus{
		Running:          s.running,
		VehicleCount:     len(s.buses),
		RouteCount:       len(routeSet),
		TickIntervalMs:   int(s.tickInterval.Milliseconds()),
		SpeedMultiplier:  s.speedMultiplier,
		LastTickAt:       lastTick,
		TotalUpdatesSent: s.totalUpdates,
	}
}

// Start begins periodic location updates (optionally updating SpeedMultiplier) and performs an initial tick immediately.
func (s *Simulator) Start(speedMultiplier *float64) (SimulatorStatus, error) {
	s.mu.Lock()
	if speedMultiplier != nil {
		if !isFiniteInRange(*speedMultiplier, minSpeedMultiplier, maxSpeedMultiplier) {
			s.mu.Unlock()
			return SimulatorStatus{}, &InvalidInput{Message: "speed_multiplier must be between 0.25 and 10"}
		}
		s.speedMultiplier = *speedMultiplier
	}

	alreadyRunning := s.running
	if !alreadyRunning {
		runCtx, cancel := context.WithCancel(s.baseCtx)
		s.runCancel = cancel
		s.running = true
		s.wg.Add(1)
		go s.loop(runCtx)
	}
	s.mu.Unlock()

	if !alreadyRunning {
		initCtx, cancel := context.WithTimeout(s.baseCtx, 5*time.Second)
		defer cancel()
		if _, err := s.Step(initCtx); err != nil {
			log.Printf("vehicle simulator initial tick deferred: %v", err)
		}
	}

	return s.Status(), nil
}

// Stop pauses the periodic simulator loop without clearing existing simulated bus positions.
func (s *Simulator) Stop() SimulatorStatus {
	s.mu.Lock()
	cancel := s.runCancel
	s.runCancel = nil
	s.running = false
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	return s.Status()
}

// Close stops the simulator and waits for its background goroutine to exit.
func (s *Simulator) Close() {
	s.Stop()
	s.baseCancel()
	s.wg.Wait()
}

// Reset removes simulated vehicle positions and re-seeds initial positions if currently running.
func (s *Simulator) Reset(ctx context.Context) (SimulatorStatus, error) {
	if s.service == nil {
		return SimulatorStatus{}, fmt.Errorf("vehicle service is not configured")
	}
	if err := s.service.ClearSimulated(ctx); err != nil {
		return SimulatorStatus{}, err
	}

	s.mu.Lock()
	s.buses = nil
	s.lastTickAt = nil
	s.totalUpdates = 0
	running := s.running
	s.mu.Unlock()

	if running {
		return s.Step(ctx)
	}
	return s.Status(), nil
}

// Step advances every simulated vehicle by one tick interval and persists its location via Service.SubmitLocation.
func (s *Simulator) Step(ctx context.Context) (SimulatorStatus, error) {
	if s.service == nil {
		return SimulatorStatus{}, fmt.Errorf("vehicle service is not configured")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.buses) == 0 {
		if err := s.loadBusesLocked(ctx); err != nil {
			return s.statusLocked(), err
		}
	}
	if len(s.buses) == 0 {
		return s.statusLocked(), nil
	}

	nowUTC := s.now().UTC()
	dtSeconds := s.tickInterval.Seconds() * s.speedMultiplier
	var firstErr error
	sentThisTick := 0

	for i := range s.buses {
		update := advanceBus(&s.buses[i], dtSeconds, nowUTC)
		if _, err := s.service.SubmitLocation(ctx, update, "simulator"); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		sentThisTick++
	}

	if sentThisTick > 0 {
		s.totalUpdates += int64(sentThisTick)
		s.lastTickAt = &nowUTC
	}
	return s.statusLocked(), firstErr
}

func (s *Simulator) loop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tickCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if _, err := s.Step(tickCtx); err != nil && ctx.Err() == nil {
				log.Printf("vehicle simulator tick error: %v", err)
			}
			cancel()
		}
	}
}

func (s *Simulator) loadBusesLocked(ctx context.Context) error {
	routes, err := s.service.ActiveRoutesWithGeometries(ctx, s.vehicleCount)
	if err != nil {
		return err
	}
	if len(routes) == 0 {
		return nil
	}

	totalToSpawn := s.vehicleCount
	if totalToSpawn > len(routes)*2 {
		// Avoid piling dozens of identical simulated buses onto very tiny test catalogs while still scaling on real fleets.
		if len(routes) <= 3 {
			totalToSpawn = len(routes) * 2
		}
	}

	buses := make([]simulatedBus, 0, totalToSpawn)
	for i := 0; i < totalToSpawn; i++ {
		route := routes[i%len(routes)]
		segments, totalMeters := buildSegments(route.Coordinates)
		if len(segments) == 0 {
			continue
		}
		initialRatio := float64((i*37)%90+5) / 100.0
		buses = append(buses, simulatedBus{
			vehicleNumber:  fmt.Sprintf("SIM-%03d", i+1),
			isAccessible:   i%4 != 3,
			route:          route,
			segments:       segments,
			totalMeters:    totalMeters,
			distanceMeters: totalMeters * initialRatio,
			direction:      1,
			baseSpeedKmh:   28.0 + float64(i%5)*3.5,
			tickCount:      i,
		})
	}
	s.buses = buses
	return nil
}

func advanceBus(bus *simulatedBus, dtSeconds float64, nowUTC time.Time) LocationUpdateInput {
	bus.tickCount++
	status := StatusInService
	speedKmh := bus.baseSpeedKmh

	if bus.totalMeters < 1 || len(bus.segments) == 0 {
		coord := bus.route.Coordinates[0]
		zero := 0.0
		status = StatusLayover
		return LocationUpdateInput{
			VehicleNumber: bus.vehicleNumber,
			IsAccessible:  &bus.isAccessible,
			RouteID:       bus.route.RouteID,
			TripID:        bus.route.SampleTripID,
			Latitude:      &coord.Lat,
			Longitude:     &coord.Lng,
			Heading:       &zero,
			SpeedKmh:      &zero,
			Status:        status,
			SourceService: "simulator",
			IsSimulated:   true,
			RecordedAt:    &nowUTC,
		}
	}

	if bus.layoverTicks > 0 {
		bus.layoverTicks--
		status = StatusLayover
		speedKmh = 0
	} else {
		// Showcase realistic delayed buses cleanly without flooding every route with delays.
		if bus.tickCount%16 >= 12 && bus.isAccessible {
			speedKmh = bus.baseSpeedKmh * 0.6
			status = StatusDelayed
		}
		stepMeters := (speedKmh * 1000.0 / 3600.0) * dtSeconds
		nextDist := bus.distanceMeters + bus.direction*stepMeters
		if nextDist >= bus.totalMeters {
			bus.distanceMeters = bus.totalMeters
			bus.direction = -1
			bus.layoverTicks = 2
			status = StatusLayover
			speedKmh = 0
		} else if nextDist <= 0 {
			bus.distanceMeters = 0
			bus.direction = 1
			bus.layoverTicks = 2
			status = StatusLayover
			speedKmh = 0
		} else {
			bus.distanceMeters = nextDist
		}
	}

	pos, segHeading := interpolateAlongSegments(bus.segments, bus.distanceMeters)
	if bus.direction < 0 {
		segHeading = math.Mod(segHeading+180.0, 360.0)
	}
	isAccessible := bus.isAccessible
	headingCopy := math.Round(segHeading*10) / 10
	speedCopy := math.Round(speedKmh*10) / 10

	return LocationUpdateInput{
		VehicleNumber: bus.vehicleNumber,
		IsAccessible:  &isAccessible,
		RouteID:       bus.route.RouteID,
		TripID:        bus.route.SampleTripID,
		Latitude:      &pos.Lat,
		Longitude:     &pos.Lng,
		Heading:       &headingCopy,
		SpeedKmh:      &speedCopy,
		Status:        status,
		SourceService: "simulator",
		IsSimulated:   true,
		RecordedAt:    &nowUTC,
	}
}

func buildSegments(coords []Coordinate) ([]pathSegment, float64) {
	if len(coords) < 2 {
		return nil, 0
	}
	segments := make([]pathSegment, 0, len(coords)-1)
	cumDist := 0.0
	for i := 0; i < len(coords)-1; i++ {
		a, b := coords[i], coords[i+1]
		dist := haversineMeters(a, b)
		if dist < 0.1 {
			continue
		}
		segments = append(segments, pathSegment{
			start:          a,
			end:            b,
			startDistM:     cumDist,
			endDistM:       cumDist + dist,
			distanceMeters: dist,
			headingDeg:     bearingDegrees(a, b),
		})
		cumDist += dist
	}
	return segments, cumDist
}

func interpolateAlongSegments(segments []pathSegment, targetDist float64) (Coordinate, float64) {
	if len(segments) == 0 {
		return Coordinate{}, 0
	}
	if targetDist <= 0 {
		return segments[0].start, segments[0].headingDeg
	}
	last := segments[len(segments)-1]
	if targetDist >= last.endDistM {
		return last.end, last.headingDeg
	}
	for _, seg := range segments {
		if targetDist <= seg.endDistM {
			t := (targetDist - seg.startDistM) / seg.distanceMeters
			if t < 0 {
				t = 0
			} else if t > 1 {
				t = 1
			}
			return Coordinate{
				Lat: seg.start.Lat + (seg.end.Lat-seg.start.Lat)*t,
				Lng: seg.start.Lng + (seg.end.Lng-seg.start.Lng)*t,
			}, seg.headingDeg
		}
	}
	return last.end, last.headingDeg
}

func haversineMeters(a, b Coordinate) float64 {
	const rad = math.Pi / 180.0
	lat1, lon1 := a.Lat*rad, a.Lng*rad
	lat2, lon2 := b.Lat*rad, b.Lng*rad
	dLat := lat2 - lat1
	dLon := lon2 - lon1
	sinLat := math.Sin(dLat / 2)
	sinLon := math.Sin(dLon / 2)
	h := sinLat*sinLat + math.Cos(lat1)*math.Cos(lat2)*sinLon*sinLon
	if h > 1 {
		h = 1
	}
	return 2 * earthRadiusMeters * math.Asin(math.Sqrt(h))
}

func bearingDegrees(a, b Coordinate) float64 {
	const rad = math.Pi / 180.0
	lat1, lon1 := a.Lat*rad, a.Lng*rad
	lat2, lon2 := b.Lat*rad, b.Lng*rad
	dLon := lon2 - lon1
	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)
	deg := math.Mod(math.Atan2(y, x)*(180.0/math.Pi)+360.0, 360.0)
	if math.IsNaN(deg) {
		return 0
	}
	return deg
}
