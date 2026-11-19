package transit

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

const (
	// DefaultMaxTransfers is the default upper bound on transit transfers per itinerary.
	DefaultMaxTransfers = 3
	// MaxAllowedTransfers is the maximum configurable number of transit transfers.
	MaxAllowedTransfers = 3
	// DefaultMaxWalkMeters is the default straight-line walking radius in meters per walk leg.
	DefaultMaxWalkMeters = 600
	// MaxAllowedWalkMeters is the maximum configurable walking radius in meters per walk leg.
	MaxAllowedWalkMeters = 1500

	maxTransferWalkMeters    = 500
	walkDetourFactor         = 1.25
	walkSpeedMetersPerMinute = 75.0
	maxTransferWait          = 60 * time.Minute
	maxJourneyDuration       = 4 * time.Hour
	maxPlannerIterations     = 40

	gridCellLatDeg = 0.006
	gridCellLonDeg = 0.008
	earthRadiusM   = 6371000.0
)

type walkEdge struct {
	From            Stop
	To              Stop
	DistanceMeters  int
	Duration        time.Duration
	DurationMinutes int
}

type serviceDayInfo struct {
	DateStr      string
	Year         int
	Month        time.Month
	Day          int
	ScheduleType string
}

type tripStopTime struct {
	RouteStopID  int
	StopID       int
	StopSequence int
	Arrival      time.Time
	Departure    time.Time
}

type tripInstance struct {
	TripID      int
	RouteID     int
	ServiceDate string
	Stops       []tripStopTime
}

type raptorPattern struct {
	RouteID int
	Route   Route
	StopIDs []int
	Trips   []tripInstance
}

type patternStopRef struct {
	PatternIdx int
	Pos        int
}

type raptorLabel struct {
	Reached     bool
	IsWalk      bool
	PrevRound   int
	PrevStopID  int
	OriginDep   time.Time
	Arrival     time.Time
	Departure   time.Time
	PatternIdx  int
	TripIdx     int
	BoardPos    int
	AlightPos   int
	Walk        walkEdge
	AccessWalk  walkEdge
	UsedRouteID int
}

type gridCell struct {
	x, y int
}

type routeStopKey struct {
	RouteID int
	StopID  int
}

type disruptionIndex struct {
	stopClosures    map[int][]JourneyDisruption
	routeWide       map[int][]JourneyDisruption
	routeStopSever  map[routeStopKey][]JourneyDisruption
	routeStopDetour map[routeStopKey][]JourneyDisruption
	triggered       map[int]DisruptionNotice
}

func newDisruptionIndex(disruptions []JourneyDisruption) *disruptionIndex {
	idx := &disruptionIndex{
		stopClosures:    make(map[int][]JourneyDisruption),
		routeWide:       make(map[int][]JourneyDisruption),
		routeStopSever:  make(map[routeStopKey][]JourneyDisruption),
		routeStopDetour: make(map[routeStopKey][]JourneyDisruption),
		triggered:       make(map[int]DisruptionNotice),
	}
	for _, d := range disruptions {
		if d.Status != "active" && d.Status != "scheduled" {
			continue
		}
		switch d.Type {
		case "cancellation", "stop_closure", "road_closure", "emergency", "detour":
		default:
			continue
		}
		if d.StopID == nil {
			if d.Type != "detour" {
				idx.routeWide[d.RouteID] = append(idx.routeWide[d.RouteID], d)
			}
			continue
		}
		stopID := *d.StopID
		if d.Type == "stop_closure" {
			idx.stopClosures[stopID] = append(idx.stopClosures[stopID], d)
			continue
		}
		key := routeStopKey{RouteID: d.RouteID, StopID: stopID}
		if d.Type == "detour" {
			idx.routeStopDetour[key] = append(idx.routeStopDetour[key], d)
			continue
		}
		idx.routeStopSever[key] = append(idx.routeStopSever[key], d)
	}
	return idx
}

func coversInstant(d JourneyDisruption, t time.Time) bool {
	return !t.Before(d.StartTime) && (d.EndTime == nil || !t.After(*d.EndTime))
}

func overlapsInterval(d JourneyDisruption, start, end time.Time) bool {
	return !end.Before(d.StartTime) && (d.EndTime == nil || !start.After(*d.EndTime))
}

func (idx *disruptionIndex) record(d JourneyDisruption) {
	if _, exists := idx.triggered[d.ID]; exists {
		return
	}
	idx.triggered[d.ID] = DisruptionNotice{
		ID:        d.ID,
		Type:      d.Type,
		Title:     d.Title,
		Status:    d.Status,
		StartTime: d.StartTime,
		EndTime:   d.EndTime,
	}
}

func (idx *disruptionIndex) notices() []DisruptionNotice {
	if len(idx.triggered) == 0 {
		return nil
	}
	out := make([]DisruptionNotice, 0, len(idx.triggered))
	for _, notice := range idx.triggered {
		out = append(out, notice)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].StartTime.Equal(out[j].StartTime) {
			return out[i].StartTime.Before(out[j].StartTime)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (idx *disruptionIndex) isStopClosedAt(stopID int, t time.Time) bool {
	if idx == nil {
		return false
	}
	for _, d := range idx.stopClosures[stopID] {
		if coversInstant(d, t) {
			idx.record(d)
			return true
		}
	}
	return false
}

func (idx *disruptionIndex) isRouteWideUnavailable(routeID int, dep, arr time.Time) bool {
	if idx == nil {
		return false
	}
	for _, d := range idx.routeWide[routeID] {
		if overlapsInterval(d, dep, arr) {
			idx.record(d)
			return true
		}
	}
	return false
}

func (idx *disruptionIndex) seversRouteSegmentAt(routeID, stopID int, t time.Time) bool {
	if idx == nil {
		return false
	}
	if idx.isRouteWideUnavailable(routeID, t, t) {
		return true
	}
	for _, d := range idx.routeStopSever[routeStopKey{RouteID: routeID, StopID: stopID}] {
		if coversInstant(d, t) {
			idx.record(d)
			return true
		}
	}
	return false
}

func (idx *disruptionIndex) isRouteStopBlockedAt(routeID, stopID int, t time.Time) bool {
	if idx == nil {
		return false
	}
	if idx.isStopClosedAt(stopID, t) {
		return true
	}
	if idx.seversRouteSegmentAt(routeID, stopID, t) {
		return true
	}
	for _, d := range idx.routeStopDetour[routeStopKey{RouteID: routeID, StopID: stopID}] {
		if coversInstant(d, t) {
			idx.record(d)
			return true
		}
	}
	return false
}

type stopGrid struct {
	cells map[gridCell][]Stop
}

func newStopGrid(stops []Stop) *stopGrid {
	cells := make(map[gridCell][]Stop, len(stops))
	for _, stop := range stops {
		if !hasCoordinates(stop) {
			continue
		}
		cell := cellForStop(stop)
		cells[cell] = append(cells[cell], stop)
	}
	return &stopGrid{cells: cells}
}

func cellForStop(stop Stop) gridCell {
	return gridCell{
		x: int(math.Floor(stop.Longitude / gridCellLonDeg)),
		y: int(math.Floor(stop.Latitude / gridCellLatDeg)),
	}
}

func hasCoordinates(stop Stop) bool {
	return !(stop.Latitude == 0 && stop.Longitude == 0)
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const rad = math.Pi / 180.0
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	if a > 1 {
		a = 1
	}
	return 2 * earthRadiusM * math.Asin(math.Sqrt(a))
}

func walkEdgeBetween(a, b Stop, maxStraightMeters int) (walkEdge, bool) {
	if a.ID == b.ID {
		return walkEdge{From: a, To: b}, true
	}
	if maxStraightMeters <= 0 || !hasCoordinates(a) || !hasCoordinates(b) {
		return walkEdge{}, false
	}
	straight := haversineMeters(a.Latitude, a.Longitude, b.Latitude, b.Longitude)
	if straight > float64(maxStraightMeters) {
		return walkEdge{}, false
	}
	walkMeters := int(math.Round(straight * walkDetourFactor))
	if walkMeters < 1 {
		walkMeters = 1
	}
	walkMinutes := int(math.Ceil(float64(walkMeters) / walkSpeedMetersPerMinute))
	if walkMinutes < 1 {
		walkMinutes = 1
	}
	return walkEdge{
		From:            a,
		To:              b,
		DistanceMeters:  walkMeters,
		Duration:        time.Duration(walkMinutes) * time.Minute,
		DurationMinutes: walkMinutes,
	}, true
}

func (g *stopGrid) neighbors(center Stop, radiusMeters int) []walkEdge {
	if radiusMeters <= 0 || !hasCoordinates(center) {
		return nil
	}
	c := cellForStop(center)
	cosLat := math.Max(0.2, math.Abs(math.Cos(center.Latitude*math.Pi/180.0)))
	dy := int(math.Ceil(float64(radiusMeters) / (111320.0 * gridCellLatDeg)))
	dx := int(math.Ceil(float64(radiusMeters) / (111320.0 * cosLat * gridCellLonDeg)))

	var edges []walkEdge
	for x := c.x - dx; x <= c.x+dx; x++ {
		for y := c.y - dy; y <= c.y+dy; y++ {
			for _, candidate := range g.cells[gridCell{x: x, y: y}] {
				if candidate.ID == center.ID {
					continue
				}
				if edge, ok := walkEdgeBetween(center, candidate, radiusMeters); ok {
					edges = append(edges, edge)
				}
			}
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].DistanceMeters != edges[j].DistanceMeters {
			return edges[i].DistanceMeters < edges[j].DistanceMeters
		}
		return edges[i].To.ID < edges[j].To.ID
	})
	return edges
}

func loadAgencyLocation() *time.Location {
	loc, err := time.LoadLocation(AgencyTimezone)
	if err != nil {
		return time.FixedZone("EST", -5*3600)
	}
	return loc
}

func candidateServiceDays(from, until time.Time, loc *time.Location) []serviceDayInfo {
	fromLocal := from.In(loc)
	endLocal := until.Add(maxJourneyDuration).In(loc)
	startDate := time.Date(fromLocal.Year(), fromLocal.Month(), fromLocal.Day()-1, 0, 0, 0, 0, loc)
	endDate := time.Date(endLocal.Year(), endLocal.Month(), endLocal.Day(), 0, 0, 0, 0, loc)

	var days []serviceDayInfo
	for cur := startDate; !cur.After(endDate); cur = time.Date(cur.Year(), cur.Month(), cur.Day()+1, 0, 0, 0, 0, loc) {
		sched := "weekday"
		if wd := cur.Weekday(); wd == time.Saturday || wd == time.Sunday {
			sched = "weekend"
		}
		days = append(days, serviceDayInfo{
			DateStr:      cur.Format(time.DateOnly),
			Year:         cur.Year(),
			Month:        cur.Month(),
			Day:          cur.Day(),
			ScheduleType: sched,
		})
	}
	return days
}

func neededScheduleTypes(from, until time.Time) []string {
	days := candidateServiceDays(from, until, loadAgencyLocation())
	seen := map[string]bool{"daily": true}
	for _, day := range days {
		seen[day.ScheduleType] = true
	}
	types := make([]string, 0, len(seen))
	for _, candidate := range []string{"daily", "weekday", "weekend"} {
		if seen[candidate] {
			types = append(types, candidate)
		}
	}
	return types
}

func serviceTime(day serviceDayInfo, offsetSec int64, loc *time.Location) time.Time {
	extraDays := int(offsetSec / 86400)
	rem := int(offsetSec % 86400)
	if rem < 0 {
		extraDays--
		rem += 86400
	}
	hour := rem / 3600
	minute := (rem % 3600) / 60
	second := rem % 60
	return time.Date(day.Year, day.Month, day.Day+extraDays, hour, minute, second, 0, loc).UTC()
}

func buildPatterns(network JourneyNetwork, q JourneyRepoQuery, loc *time.Location, routeByID map[int]Route, stopByID map[int]Stop) ([]raptorPattern, map[int][]patternStopRef) {
	type tripGroup struct {
		TripID       int
		RouteID      int
		ScheduleType string
		Rows         []JourneyStopTimeRow
	}
	groupsMap := make(map[int]*tripGroup)
	var tripOrder []int
	for _, row := range network.StopTimes {
		if _, ok := routeByID[row.RouteID]; !ok {
			continue
		}
		if _, ok := stopByID[row.StopID]; !ok {
			continue
		}
		g, exists := groupsMap[row.TripID]
		if !exists {
			g = &tripGroup{TripID: row.TripID, RouteID: row.RouteID, ScheduleType: row.ScheduleType}
			groupsMap[row.TripID] = g
			tripOrder = append(tripOrder, row.TripID)
		}
		g.Rows = append(g.Rows, row)
	}

	days := candidateServiceDays(q.From, q.Until, loc)
	windowEnd := q.Until.Add(maxJourneyDuration)

	type patternKey struct {
		RouteID int
		Sig     string
	}
	patternIndexByKey := make(map[patternKey]int)
	var patterns []raptorPattern

	var sigBuf strings.Builder
	for _, tripID := range tripOrder {
		g := groupsMap[tripID]
		if len(g.Rows) < 2 {
			continue
		}
		sorted := true
		for i := 1; i < len(g.Rows); i++ {
			if g.Rows[i].StopSequence <= g.Rows[i-1].StopSequence {
				sorted = false
				break
			}
		}
		if !sorted {
			sort.Slice(g.Rows, func(i, j int) bool {
				return g.Rows[i].StopSequence < g.Rows[j].StopSequence
			})
		}

		sigBuf.Reset()
		stopIDs := make([]int, len(g.Rows))
		for i, r := range g.Rows {
			stopIDs[i] = r.StopID
			if i > 0 {
				sigBuf.WriteByte(',')
			}
			sigBuf.WriteString(strconv.Itoa(r.StopID))
		}
		key := patternKey{RouteID: g.RouteID, Sig: sigBuf.String()}
		pIdx, ok := patternIndexByKey[key]
		if !ok {
			pIdx = len(patterns)
			patternIndexByKey[key] = pIdx
			patterns = append(patterns, raptorPattern{
				RouteID: g.RouteID,
				Route:   routeByID[g.RouteID],
				StopIDs: stopIDs,
			})
		}

		for _, day := range days {
			if g.ScheduleType != "daily" && g.ScheduleType != day.ScheduleType {
				continue
			}
			firstDep := serviceTime(day, g.Rows[0].DepartureSec, loc)
			lastArr := serviceTime(day, g.Rows[len(g.Rows)-1].ArrivalSec, loc)
			if lastArr.Before(q.From) || !firstDep.Before(windowEnd) {
				continue
			}
			stops := make([]tripStopTime, len(g.Rows))
			for i, r := range g.Rows {
				stops[i] = tripStopTime{
					RouteStopID:  r.RouteStopID,
					StopID:       r.StopID,
					StopSequence: r.StopSequence,
					Arrival:      serviceTime(day, r.ArrivalSec, loc),
					Departure:    serviceTime(day, r.DepartureSec, loc),
				}
			}
			patterns[pIdx].Trips = append(patterns[pIdx].Trips, tripInstance{
				TripID:      g.TripID,
				RouteID:     g.RouteID,
				ServiceDate: day.DateStr,
				Stops:       stops,
			})
		}
	}

	stopToPatterns := make(map[int][]patternStopRef)
	for pIdx := range patterns {
		trips := patterns[pIdx].Trips
		sort.Slice(trips, func(i, j int) bool {
			if !trips[i].Stops[0].Departure.Equal(trips[j].Stops[0].Departure) {
				return trips[i].Stops[0].Departure.Before(trips[j].Stops[0].Departure)
			}
			if trips[i].TripID != trips[j].TripID {
				return trips[i].TripID < trips[j].TripID
			}
			return trips[i].ServiceDate < trips[j].ServiceDate
		})
		patterns[pIdx].Trips = trips
		if len(trips) == 0 {
			continue
		}
		for pos, stopID := range patterns[pIdx].StopIDs {
			stopToPatterns[stopID] = append(stopToPatterns[stopID], patternStopRef{PatternIdx: pIdx, Pos: pos})
		}
	}

	return patterns, stopToPatterns
}

// PlanJourneys computes direct, multi-transfer, and walking-connected itineraries using iterative RAPTOR.
func PlanJourneys(network JourneyNetwork, q JourneyRepoQuery) []Itinerary {
	return PlanJourneysDetailed(network, q).Itineraries
}

// PlanJourneysDetailed computes itineraries while excluding disrupted trips, closed stops, and unavailable segments.
func PlanJourneysDetailed(network JourneyNetwork, q JourneyRepoQuery) JourneyPlan {
	if len(q.OriginStopIDs) == 0 || len(q.DestinationStopIDs) == 0 || q.Limit <= 0 {
		return JourneyPlan{Itineraries: []Itinerary{}}
	}
	maxTransfers := q.MaxTransfers
	if maxTransfers < 0 {
		maxTransfers = 0
	}
	if maxTransfers > MaxAllowedTransfers {
		maxTransfers = MaxAllowedTransfers
	}
	maxWalkMeters := q.MaxWalkMeters
	if maxWalkMeters < 0 {
		maxWalkMeters = 0
	}
	if maxWalkMeters > MaxAllowedWalkMeters {
		maxWalkMeters = MaxAllowedWalkMeters
	}

	stopByID := make(map[int]Stop, len(network.Stops))
	for _, stop := range network.Stops {
		if stop.IsActive {
			stopByID[stop.ID] = stop
		}
	}
	routeByID := make(map[int]Route, len(network.Routes))
	for _, route := range network.Routes {
		if route.IsActive {
			routeByID[route.ID] = route
		}
	}

	grid := newStopGrid(network.Stops)
	accessByStop := make(map[int]walkEdge)
	setAccess := func(stopID int, edge walkEdge) {
		prev, exists := accessByStop[stopID]
		if !exists || edge.DistanceMeters < prev.DistanceMeters || (edge.DistanceMeters == prev.DistanceMeters && edge.From.ID < prev.From.ID) {
			accessByStop[stopID] = edge
		}
	}
	for _, origID := range q.OriginStopIDs {
		orig, ok := stopByID[origID]
		if !ok {
			continue
		}
		setAccess(orig.ID, walkEdge{From: orig, To: orig})
		for _, nb := range grid.neighbors(orig, maxWalkMeters) {
			setAccess(nb.To.ID, nb)
		}
	}

	egressByStop := make(map[int]walkEdge)
	setEgress := func(stopID int, edge walkEdge) {
		prev, exists := egressByStop[stopID]
		if !exists || edge.DistanceMeters < prev.DistanceMeters || (edge.DistanceMeters == prev.DistanceMeters && edge.To.ID < prev.To.ID) {
			egressByStop[stopID] = edge
		}
	}
	for _, destID := range q.DestinationStopIDs {
		dest, ok := stopByID[destID]
		if !ok {
			continue
		}
		setEgress(dest.ID, walkEdge{From: dest, To: dest})
		for _, nb := range grid.neighbors(dest, maxWalkMeters) {
			// Flip edge direction: egress walks from nearby alighting stop -> destination stop.
			setEgress(nb.To.ID, walkEdge{
				From:            nb.To,
				To:              dest,
				DistanceMeters:  nb.DistanceMeters,
				Duration:        nb.Duration,
				DurationMinutes: nb.DurationMinutes,
			})
		}
	}

	if len(accessByStop) == 0 || len(egressByStop) == 0 {
		return JourneyPlan{Itineraries: []Itinerary{}}
	}

	loc := loadAgencyLocation()
	patterns, stopToPatterns := buildPatterns(network, q, loc, routeByID, stopByID)
	disruptions := newDisruptionIndex(network.Disruptions)
	maxRounds := maxTransfers + 1

	transferRadius := maxWalkMeters
	if transferRadius > maxTransferWalkMeters {
		transferRadius = maxTransferWalkMeters
	}
	transferCache := make(map[int][]walkEdge)
	getTransferNeighbors := func(stopID int) []walkEdge {
		if transferRadius <= 0 {
			return nil
		}
		if cached, ok := transferCache[stopID]; ok {
			return cached
		}
		edges := grid.neighbors(stopByID[stopID], transferRadius)
		transferCache[stopID] = edges
		return edges
	}

	var candidates []Itinerary

	// Direct walk-only option if an origin stop and distinct destination stop are within maxWalkMeters.
	if maxWalkMeters > 0 {
		var bestDirectWalk *walkEdge
		for _, origID := range q.OriginStopIDs {
			orig, ok := stopByID[origID]
			if !ok {
				continue
			}
			for _, destID := range q.DestinationStopIDs {
				if origID == destID {
					continue
				}
				dest, ok := stopByID[destID]
				if !ok {
					continue
				}
				edge, ok := walkEdgeBetween(orig, dest, maxWalkMeters)
				if !ok {
					continue
				}
				if bestDirectWalk == nil || edge.DistanceMeters < bestDirectWalk.DistanceMeters {
					e := edge
					bestDirectWalk = &e
				}
			}
		}
		if bestDirectWalk != nil {
			dep := q.From
			arr := dep.Add(bestDirectWalk.Duration)
			candidates = append(candidates, Itinerary{
				OriginStop:         bestDirectWalk.From,
				DestinationStop:    bestDirectWalk.To,
				ScheduledDeparture: dep,
				ScheduledArrival:   arr,
				DurationMinutes:    bestDirectWalk.DurationMinutes,
				WalkingMinutes:     bestDirectWalk.DurationMinutes,
				Transfers:          0,
				Legs: []JourneyLeg{{
					Mode:               "walk",
					FromStopID:         bestDirectWalk.From.ID,
					FromStopName:       bestDirectWalk.From.Name,
					ToStopID:           bestDirectWalk.To.ID,
					ToStopName:         bestDirectWalk.To.Name,
					DistanceMeters:     bestDirectWalk.DistanceMeters,
					ScheduledDeparture: dep,
					ScheduledArrival:   arr,
					DurationMinutes:    bestDirectWalk.DurationMinutes,
				}},
			})
		}
	}

	searchFrom := q.From
	for iter := 0; iter < maxPlannerIterations && searchFrom.Before(q.Until); iter++ {
		passItineraries := runRaptorPass(
			searchFrom,
			q.Until,
			maxRounds,
			patterns,
			stopToPatterns,
			stopByID,
			accessByStop,
			egressByStop,
			getTransferNeighbors,
			disruptions,
		)
		if len(passItineraries) == 0 {
			break
		}

		earliestDep := passItineraries[0].ScheduledDeparture
		for _, it := range passItineraries[1:] {
			if it.ScheduledDeparture.Before(earliestDep) {
				earliestDep = it.ScheduledDeparture
			}
		}
		candidates = append(candidates, passItineraries...)

		nextFrom := earliestDep.Add(time.Minute)
		if !nextFrom.After(searchFrom) {
			nextFrom = searchFrom.Add(time.Minute)
		}

		pruned := pruneAndSortItineraries(candidates)
		if len(pruned) >= q.Limit && pruned[q.Limit-1].ScheduledArrival.Before(nextFrom) {
			candidates = pruned
			break
		}
		candidates = pruned
		searchFrom = nextFrom
	}

	candidates = pruneAndSortItineraries(candidates)
	if len(candidates) > q.Limit {
		candidates = candidates[:q.Limit]
	}
	return JourneyPlan{
		Itineraries:         candidates,
		ExcludedDisruptions: disruptions.notices(),
	}
}

func runRaptorPass(
	searchFrom time.Time,
	windowUntil time.Time,
	maxRounds int,
	patterns []raptorPattern,
	stopToPatterns map[int][]patternStopRef,
	stopByID map[int]Stop,
	accessByStop map[int]walkEdge,
	egressByStop map[int]walkEdge,
	getTransferNeighbors func(int) []walkEdge,
	disruptions *disruptionIndex,
) []Itinerary {
	bestTransit := make([]map[int]raptorLabel, maxRounds+1)
	bestAny := make([]map[int]raptorLabel, maxRounds+1)
	for r := 0; r <= maxRounds; r++ {
		bestTransit[r] = make(map[int]raptorLabel)
		bestAny[r] = make(map[int]raptorLabel)
	}
	earliestTransit := make(map[int]time.Time)
	earliestAny := make(map[int]time.Time)

	markedStops := make(map[int]bool, len(accessByStop))
	for stopID, edge := range accessByStop {
		arr := searchFrom.Add(edge.Duration)
		bestAny[0][stopID] = raptorLabel{
			Reached:    true,
			Arrival:    arr,
			AccessWalk: edge,
		}
		earliestAny[stopID] = arr
		markedStops[stopID] = true
	}

	routeUsedInChain := func(round int, stopID int, targetRouteID int) bool {
		r := round
		s := stopID
		for r > 0 {
			lbl, ok := bestAny[r][s]
			if !ok || !lbl.Reached {
				break
			}
			if lbl.IsWalk {
				lbl, ok = bestTransit[r][lbl.PrevStopID]
				if !ok || !lbl.Reached {
					break
				}
			}
			if lbl.UsedRouteID == targetRouteID {
				return true
			}
			s = lbl.PrevStopID
			r = lbl.PrevRound
		}
		return false
	}

	for round := 1; round <= maxRounds && len(markedStops) > 0; round++ {
		markedPatterns := make(map[int]int)
		for stopID := range markedStops {
			for _, ref := range stopToPatterns[stopID] {
				if prevPos, exists := markedPatterns[ref.PatternIdx]; !exists || ref.Pos < prevPos {
					markedPatterns[ref.PatternIdx] = ref.Pos
				}
			}
		}

		patternIndices := make([]int, 0, len(markedPatterns))
		for pIdx := range markedPatterns {
			patternIndices = append(patternIndices, pIdx)
		}
		sort.Ints(patternIndices)

		transitImproved := make(map[int]bool)

		for _, pIdx := range patternIndices {
			pattern := patterns[pIdx]
			if len(pattern.Trips) == 0 {
				continue
			}
			minPos := markedPatterns[pIdx]
			activeTripIdx := -1
			boardPos := -1
			boardPrevStop := 0
			var boardAccess walkEdge
			var boardOriginDep time.Time

			for pos := minPos; pos < len(pattern.StopIDs); pos++ {
				stopID := pattern.StopIDs[pos]

				if activeTripIdx >= 0 {
					tr := pattern.Trips[activeTripIdx]
					arr := tr.Stops[pos].Arrival
					if disruptions.isRouteWideUnavailable(pattern.RouteID, tr.Stops[boardPos].Departure, arr) ||
						disruptions.seversRouteSegmentAt(pattern.RouteID, stopID, arr) {
						activeTripIdx = -1
						boardPos = -1
					} else if !disruptions.isRouteStopBlockedAt(pattern.RouteID, stopID, arr) &&
						!arr.Before(tr.Stops[boardPos].Departure) && arr.Sub(boardOriginDep) <= maxJourneyDuration {
						prevBest, hasBest := bestTransit[round][stopID]
						prevEarliest, hasEarliest := earliestTransit[stopID]
						if (!hasBest || arr.Before(prevBest.Arrival)) && (!hasEarliest || arr.Before(prevEarliest)) {
							bestTransit[round][stopID] = raptorLabel{
								Reached:     true,
								PrevRound:   round - 1,
								PrevStopID:  boardPrevStop,
								OriginDep:   boardOriginDep,
								Arrival:     arr,
								Departure:   tr.Stops[boardPos].Departure,
								PatternIdx:  pIdx,
								TripIdx:     activeTripIdx,
								BoardPos:    boardPos,
								AlightPos:   pos,
								AccessWalk:  boardAccess,
								UsedRouteID: pattern.RouteID,
							}
							earliestTransit[stopID] = arr
							if ea, ok := earliestAny[stopID]; !ok || arr.Before(ea) {
								earliestAny[stopID] = arr
							}
							transitImproved[stopID] = true
						}
					}
				}

				prevLbl, canBoardHere := bestAny[round-1][stopID]
				if !canBoardHere || !prevLbl.Reached {
					continue
				}
				if round > 1 && routeUsedInChain(round-1, stopID, pattern.RouteID) {
					continue
				}

				var maxBoard time.Time
				if round == 1 {
					maxBoard = windowUntil.Add(prevLbl.AccessWalk.Duration)
				} else {
					maxBoard = prevLbl.Arrival.Add(maxTransferWait)
				}

				bestCandidateIdx := -1
				var bestCandidateDep time.Time
				for idx := range pattern.Trips {
					tr := &pattern.Trips[idx]
					if tr.Stops[0].Departure.After(maxBoard) {
						break
					}
					dep := tr.Stops[pos].Departure
					if dep.Before(prevLbl.Arrival) {
						continue
					}
					if round == 1 {
						if !dep.Before(maxBoard) {
							continue
						}
					} else if dep.After(maxBoard) {
						continue
					}
					if disruptions.isRouteWideUnavailable(pattern.RouteID, dep, dep) ||
						disruptions.isRouteStopBlockedAt(pattern.RouteID, stopID, dep) {
						continue
					}
					if bestCandidateIdx < 0 || dep.Before(bestCandidateDep) {
						bestCandidateIdx = idx
						bestCandidateDep = dep
					}
				}

				if bestCandidateIdx >= 0 {
					var candOriginDep time.Time
					if round == 1 {
						candOriginDep = bestCandidateDep.Add(-prevLbl.AccessWalk.Duration)
					} else {
						candOriginDep = prevLbl.OriginDep
					}
					if activeTripIdx < 0 || bestCandidateDep.Before(pattern.Trips[activeTripIdx].Stops[pos].Departure) {
						activeTripIdx = bestCandidateIdx
						boardPos = pos
						boardPrevStop = stopID
						boardAccess = prevLbl.AccessWalk
						boardOriginDep = candOriginDep
					} else if round == 1 && bestCandidateIdx == activeTripIdx && pos > boardPos {
						// In round 1, prefer a closer access stop for the exact same trip if it leaves later and walks no farther.
						if candOriginDep.After(boardOriginDep) && prevLbl.AccessWalk.DistanceMeters <= boardAccess.DistanceMeters {
							boardPos = pos
							boardPrevStop = stopID
							boardAccess = prevLbl.AccessWalk
							boardOriginDep = candOriginDep
						}
					}
				}
			}
		}

		nextMarked := make(map[int]bool, len(transitImproved))
		for stopID := range transitImproved {
			lbl := bestTransit[round][stopID]
			bestAny[round][stopID] = lbl
			nextMarked[stopID] = true
		}

		// Relax walking transfers out of stops reached by transit in this round (never chaining two walks).
		for stopID := range transitImproved {
			transitLbl := bestTransit[round][stopID]
			for _, edge := range getTransferNeighbors(stopID) {
				walkArr := transitLbl.Arrival.Add(edge.Duration)
				if walkArr.Sub(transitLbl.OriginDep) > maxJourneyDuration {
					continue
				}
				prevAny, hasAny := bestAny[round][edge.To.ID]
				prevEarliest, hasEarliest := earliestAny[edge.To.ID]
				if (!hasAny || walkArr.Before(prevAny.Arrival)) && (!hasEarliest || walkArr.Before(prevEarliest)) {
					bestAny[round][edge.To.ID] = raptorLabel{
						Reached:    true,
						IsWalk:     true,
						PrevRound:  round,
						PrevStopID: stopID,
						OriginDep:  transitLbl.OriginDep,
						Departure:  transitLbl.Arrival,
						Arrival:    walkArr,
						Walk:       edge,
					}
					earliestAny[edge.To.ID] = walkArr
					nextMarked[edge.To.ID] = true
				}
			}
		}

		markedStops = nextMarked
	}

	var results []Itinerary
	for round := 1; round <= maxRounds; round++ {
		for alightStopID, egressEdge := range egressByStop {
			lbl, ok := bestTransit[round][alightStopID]
			if !ok || !lbl.Reached {
				continue
			}
			it, valid := reconstructItinerary(round, alightStopID, egressEdge, bestTransit, bestAny, patterns, stopByID, disruptions, searchFrom, windowUntil)
			if valid {
				results = append(results, it)
			}
		}
	}
	return results
}

func reconstructItinerary(
	round int,
	alightStopID int,
	egressEdge walkEdge,
	bestTransit []map[int]raptorLabel,
	bestAny []map[int]raptorLabel,
	patterns []raptorPattern,
	stopByID map[int]Stop,
	disruptions *disruptionIndex,
	windowFrom time.Time,
	windowUntil time.Time,
) (Itinerary, bool) {
	var reversed []JourneyLeg
	curRound := round
	curStop := alightStopID
	var firstAccess walkEdge
	seenRoutes := make(map[int]bool, round)
	transitLegs := 0

	for curRound > 0 {
		lbl, ok := bestTransit[curRound][curStop]
		if !ok || !lbl.Reached {
			return Itinerary{}, false
		}
		if seenRoutes[lbl.UsedRouteID] {
			return Itinerary{}, false
		}
		seenRoutes[lbl.UsedRouteID] = true

		pattern := patterns[lbl.PatternIdx]
		tr := pattern.Trips[lbl.TripIdx]
		boardStop := stopByID[pattern.StopIDs[lbl.BoardPos]]
		alightStop := stopByID[pattern.StopIDs[lbl.AlightPos]]
		routeCopy := pattern.Route
		if disruptions.isRouteWideUnavailable(pattern.RouteID, lbl.Departure, lbl.Arrival) ||
			disruptions.isRouteStopBlockedAt(pattern.RouteID, boardStop.ID, lbl.Departure) ||
			disruptions.isRouteStopBlockedAt(pattern.RouteID, alightStop.ID, lbl.Arrival) {
			return Itinerary{}, false
		}
		legStops := make([]StopTime, 0, lbl.AlightPos-lbl.BoardPos+1)
		for pos := lbl.BoardPos; pos <= lbl.AlightPos; pos++ {
			ts := tr.Stops[pos]
			if pos > lbl.BoardPos && pos < lbl.AlightPos && disruptions.seversRouteSegmentAt(pattern.RouteID, ts.StopID, ts.Arrival) {
				return Itinerary{}, false
			}
			legStops = append(legStops, StopTime{
				RouteStopID:        ts.RouteStopID,
				StopID:             ts.StopID,
				StopName:           stopByID[ts.StopID].Name,
				StopSequence:       ts.StopSequence,
				ScheduledArrival:   ts.Arrival,
				ScheduledDeparture: ts.Departure,
			})
		}

		reversed = append(reversed, JourneyLeg{
			Mode:               "transit",
			TripID:             tr.TripID,
			Route:              &routeCopy,
			ServiceDate:        tr.ServiceDate,
			FromStopID:         boardStop.ID,
			FromStopName:       boardStop.Name,
			FromStopSequence:   tr.Stops[lbl.BoardPos].StopSequence,
			ToStopID:           alightStop.ID,
			ToStopName:         alightStop.Name,
			ToStopSequence:     tr.Stops[lbl.AlightPos].StopSequence,
			StopCount:          tr.Stops[lbl.AlightPos].StopSequence - tr.Stops[lbl.BoardPos].StopSequence,
			ScheduledDeparture: lbl.Departure,
			ScheduledArrival:   lbl.Arrival,
			DurationMinutes:    minutesBetween(lbl.Departure, lbl.Arrival),
			Stops:              legStops,
		})
		transitLegs++

		if lbl.PrevRound == 0 {
			firstAccess = lbl.AccessWalk
			break
		}

		prevAnyLbl, ok := bestAny[lbl.PrevRound][lbl.PrevStopID]
		if !ok || !prevAnyLbl.Reached {
			return Itinerary{}, false
		}
		if prevAnyLbl.IsWalk {
			reversed = append(reversed, JourneyLeg{
				Mode:               "walk",
				FromStopID:         prevAnyLbl.Walk.From.ID,
				FromStopName:       prevAnyLbl.Walk.From.Name,
				ToStopID:           prevAnyLbl.Walk.To.ID,
				ToStopName:         prevAnyLbl.Walk.To.Name,
				DistanceMeters:     prevAnyLbl.Walk.DistanceMeters,
				ScheduledDeparture: prevAnyLbl.Departure,
				ScheduledArrival:   prevAnyLbl.Arrival,
				DurationMinutes:    prevAnyLbl.Walk.DurationMinutes,
			})
			curStop = prevAnyLbl.PrevStopID
			curRound = prevAnyLbl.PrevRound
		} else {
			curStop = lbl.PrevStopID
			curRound = lbl.PrevRound
		}
	}

	if transitLegs == 0 {
		return Itinerary{}, false
	}

	legs := make([]JourneyLeg, 0, len(reversed)+2)
	if firstAccess.DistanceMeters > 0 {
		firstTransitDep := reversed[len(reversed)-1].ScheduledDeparture
		legs = append(legs, JourneyLeg{
			Mode:               "walk",
			FromStopID:         firstAccess.From.ID,
			FromStopName:       firstAccess.From.Name,
			ToStopID:           firstAccess.To.ID,
			ToStopName:         firstAccess.To.Name,
			DistanceMeters:     firstAccess.DistanceMeters,
			ScheduledDeparture: firstTransitDep.Add(-firstAccess.Duration),
			ScheduledArrival:   firstTransitDep,
			DurationMinutes:    firstAccess.DurationMinutes,
		})
	}
	for i := len(reversed) - 1; i >= 0; i-- {
		legs = append(legs, reversed[i])
	}
	if egressEdge.DistanceMeters > 0 {
		lastTransitArr := legs[len(legs)-1].ScheduledArrival
		legs = append(legs, JourneyLeg{
			Mode:               "walk",
			FromStopID:         egressEdge.From.ID,
			FromStopName:       egressEdge.From.Name,
			ToStopID:           egressEdge.To.ID,
			ToStopName:         egressEdge.To.Name,
			DistanceMeters:     egressEdge.DistanceMeters,
			ScheduledDeparture: lastTransitArr,
			ScheduledArrival:   lastTransitArr.Add(egressEdge.Duration),
			DurationMinutes:    egressEdge.DurationMinutes,
		})
	}

	dep := legs[0].ScheduledDeparture
	arr := legs[len(legs)-1].ScheduledArrival
	if dep.Before(windowFrom) || !dep.Before(windowUntil) {
		return Itinerary{}, false
	}
	if arr.Sub(dep) > maxJourneyDuration {
		return Itinerary{}, false
	}

	originStop := stopByID[legs[0].FromStopID]
	destStop := stopByID[legs[len(legs)-1].ToStopID]
	if originStop.ID == destStop.ID {
		return Itinerary{}, false
	}

	walkingMinutes := 0
	for _, leg := range legs {
		if leg.Mode == "walk" {
			walkingMinutes += leg.DurationMinutes
		}
	}

	return Itinerary{
		OriginStop:         originStop,
		DestinationStop:    destStop,
		ScheduledDeparture: dep,
		ScheduledArrival:   arr,
		DurationMinutes:    minutesBetween(dep, arr),
		WalkingMinutes:     walkingMinutes,
		Transfers:          transitLegs - 1,
		Legs:               legs,
	}, true
}

func itinerarySignature(it Itinerary) string {
	var b strings.Builder
	for i, leg := range it.Legs {
		if i > 0 {
			b.WriteByte('|')
		}
		fmt.Fprintf(&b, "%s:%d:%d:%d:%d", leg.Mode, leg.TripID, leg.FromStopID, leg.ToStopID, leg.ScheduledDeparture.Unix())
	}
	return b.String()
}

func itineraryDominates(a, b Itinerary) bool {
	if a.ScheduledDeparture.Before(b.ScheduledDeparture) {
		return false
	}
	if a.ScheduledArrival.After(b.ScheduledArrival) {
		return false
	}
	if a.Transfers > b.Transfers {
		return false
	}
	if a.WalkingMinutes > b.WalkingMinutes {
		return false
	}
	return a.ScheduledDeparture.After(b.ScheduledDeparture) ||
		a.ScheduledArrival.Before(b.ScheduledArrival) ||
		a.Transfers < b.Transfers ||
		a.WalkingMinutes < b.WalkingMinutes
}

func pruneAndSortItineraries(candidates []Itinerary) []Itinerary {
	if len(candidates) == 0 {
		return []Itinerary{}
	}
	seen := make(map[string]struct{}, len(candidates))
	unique := make([]Itinerary, 0, len(candidates))
	for _, it := range candidates {
		sig := itinerarySignature(it)
		if _, exists := seen[sig]; exists {
			continue
		}
		seen[sig] = struct{}{}
		unique = append(unique, it)
	}

	kept := make([]Itinerary, 0, len(unique))
	for i, a := range unique {
		dominated := false
		for j, b := range unique {
			if i == j {
				continue
			}
			if itineraryDominates(b, a) {
				dominated = true
				break
			}
		}
		if !dominated {
			kept = append(kept, a)
		}
	}

	sortItineraries(kept)
	return kept
}
