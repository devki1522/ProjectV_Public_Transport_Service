package transit

import (
	"testing"
	"time"
)

func sec(h, m int) int64 {
	return int64(h*3600 + m*60)
}

func TestPlanJourneysReverseBishopWithEgressWalk(t *testing.T) {
	stops := []Stop{
		{ID: 1826, Name: "100 Old Carriage Dr.", Latitude: 43.391312, Longitude: -80.431101, IsActive: true},
		{ID: 1732, Name: "Conestoga College Doon Campus - Door 3", Latitude: 43.390298, Longitude: -80.403515, IsActive: true},
		{ID: 1054, Name: "990 Bishop St. N.", Latitude: 43.393636, Longitude: -80.332415, IsActive: true},
		{ID: 1051, Name: "1001 Bishop St. N.", Latitude: 43.393611, Longitude: -80.333524, IsActive: true},
	}
	routes := []Route{
		{ID: 16, Number: "16", Name: "Strasburg-Belmont", Direction: "direction_1", IsActive: true},
		{ID: 61, Number: "61", Name: "Preston", Direction: "direction_0", IsActive: true},
	}
	stopTimes := []JourneyStopTimeRow{
		{TripID: 167, RouteID: 16, ScheduleType: "weekday", StopID: 1826, StopSequence: 1, ArrivalSec: sec(18, 0), DepartureSec: sec(18, 0)},
		{TripID: 167, RouteID: 16, ScheduleType: "weekday", StopID: 1732, StopSequence: 2, ArrivalSec: sec(18, 12), DepartureSec: sec(18, 12)},
		{TripID: 3251, RouteID: 61, ScheduleType: "weekday", StopID: 1732, StopSequence: 1, ArrivalSec: sec(18, 18), DepartureSec: sec(18, 18)},
		{TripID: 3251, RouteID: 61, ScheduleType: "weekday", StopID: 1054, StopSequence: 2, ArrivalSec: sec(18, 45), DepartureSec: sec(18, 45)},
	}

	net := JourneyNetwork{Stops: stops, Routes: routes, StopTimes: stopTimes}
	from := time.Date(2026, 11, 18, 22, 55, 0, 0, time.UTC) // 17:55 EST (Wednesday)
	q := JourneyRepoQuery{
		OriginStopIDs:      []int{1826},
		DestinationStopIDs: []int{1051},
		From:               from,
		Until:              from.Add(3 * time.Hour),
		Limit:              5,
		MaxTransfers:       DefaultMaxTransfers,
		MaxWalkMeters:      DefaultMaxWalkMeters,
	}

	itineraries := PlanJourneys(net, q)
	if len(itineraries) != 1 {
		t.Fatalf("expected 1 itinerary with egress walk, got %d: %+v", len(itineraries), itineraries)
	}
	it := itineraries[0]
	if it.Transfers != 1 || len(it.Legs) != 3 || it.WalkingMinutes != 2 {
		t.Fatalf("unexpected itinerary summary: %+v", it)
	}
	if it.OriginStop.ID != 1826 || it.DestinationStop.ID != 1051 {
		t.Fatalf("unexpected endpoints: %+v -> %+v", it.OriginStop, it.DestinationStop)
	}
	if it.Legs[0].Mode != "transit" || it.Legs[1].Mode != "transit" || it.Legs[2].Mode != "walk" {
		t.Fatalf("unexpected leg modes: %+v", it.Legs)
	}
	walkLeg := it.Legs[2]
	if walkLeg.FromStopID != 1054 || walkLeg.ToStopID != 1051 || walkLeg.DistanceMeters < 90 || walkLeg.DurationMinutes != 2 {
		t.Fatalf("unexpected egress walk leg: %+v", walkLeg)
	}

	q.MaxWalkMeters = 0
	if disabled := PlanJourneys(net, q); len(disabled) != 0 {
		t.Fatalf("expected 0 itineraries when walking disabled, got %+v", disabled)
	}
}

func TestPlanJourneysAccessWalkAndMidTransferWalk(t *testing.T) {
	t.Run("access walk to nearby rapid route", func(t *testing.T) {
		stops := []Stop{
			{ID: 1826, Name: "100 Old Carriage Dr.", Latitude: 43.391312, Longitude: -80.431101, IsActive: true},
			{ID: 2052, Name: "Homer Watson / Pioneer", Latitude: 43.393179, Longitude: -80.426850, IsActive: true}, // ~400m
			{ID: 1732, Name: "Conestoga College Doon Campus - Door 3", Latitude: 43.390298, Longitude: -80.403515, IsActive: true},
		}
		routes := []Route{
			{ID: 201, Number: "201", Name: "iXpress", Direction: "direction_0", IsActive: true},
		}
		stopTimes := []JourneyStopTimeRow{
			{TripID: 500, RouteID: 201, ScheduleType: "daily", StopID: 2052, StopSequence: 1, ArrivalSec: sec(18, 16), DepartureSec: sec(18, 16)},
			{TripID: 500, RouteID: 201, ScheduleType: "daily", StopID: 1732, StopSequence: 2, ArrivalSec: sec(18, 25), DepartureSec: sec(18, 25)},
		}

		from := time.Date(2026, 11, 18, 23, 0, 0, 0, time.UTC) // 18:00 EST
		itineraries := PlanJourneys(JourneyNetwork{Stops: stops, Routes: routes, StopTimes: stopTimes}, JourneyRepoQuery{
			OriginStopIDs:      []int{1826},
			DestinationStopIDs: []int{1732},
			From:               from,
			Until:              from.Add(2 * time.Hour),
			Limit:              5,
			MaxTransfers:       3,
			MaxWalkMeters:      600,
		})
		if len(itineraries) != 1 {
			t.Fatalf("expected 1 itinerary with access walk, got %+v", itineraries)
		}
		it := itineraries[0]
		if it.Transfers != 0 || len(it.Legs) != 2 || it.Legs[0].Mode != "walk" || it.Legs[1].Mode != "transit" {
			t.Fatalf("unexpected access-walk itinerary: %+v", it)
		}
		if it.Legs[0].FromStopID != 1826 || it.Legs[0].ToStopID != 2052 || !it.Legs[0].ScheduledArrival.Equal(it.Legs[1].ScheduledDeparture) {
			t.Fatalf("unexpected access walk timing: %+v", it.Legs)
		}
	})

	t.Run("mid-journey transfer walk between separate stops", func(t *testing.T) {
		stops := []Stop{
			{ID: 1, Name: "Start", Latitude: 43.4400, Longitude: -80.4900, IsActive: true},
			{ID: 2, Name: "Hub North", Latitude: 43.4500, Longitude: -80.4900, IsActive: true},
			{ID: 3, Name: "Hub South", Latitude: 43.4518, Longitude: -80.4900, IsActive: true}, // ~200m from Hub North
			{ID: 4, Name: "End", Latitude: 43.4650, Longitude: -80.4900, IsActive: true},
		}
		routes := []Route{
			{ID: 1, Number: "1", Name: "Line A", Direction: "direction_0", IsActive: true},
			{ID: 2, Number: "2", Name: "Line B", Direction: "direction_0", IsActive: true},
		}
		stopTimes := []JourneyStopTimeRow{
			{TripID: 10, RouteID: 1, ScheduleType: "daily", StopID: 1, StopSequence: 1, ArrivalSec: sec(12, 0), DepartureSec: sec(12, 0)},
			{TripID: 10, RouteID: 1, ScheduleType: "daily", StopID: 2, StopSequence: 2, ArrivalSec: sec(12, 10), DepartureSec: sec(12, 10)},
			{TripID: 20, RouteID: 2, ScheduleType: "daily", StopID: 3, StopSequence: 1, ArrivalSec: sec(12, 15), DepartureSec: sec(12, 15)},
			{TripID: 20, RouteID: 2, ScheduleType: "daily", StopID: 4, StopSequence: 2, ArrivalSec: sec(12, 25), DepartureSec: sec(12, 25)},
		}

		from := time.Date(2026, 11, 18, 16, 55, 0, 0, time.UTC) // 11:55 EST
		itineraries := PlanJourneys(JourneyNetwork{Stops: stops, Routes: routes, StopTimes: stopTimes}, JourneyRepoQuery{
			OriginStopIDs:      []int{1},
			DestinationStopIDs: []int{4},
			From:               from,
			Until:              from.Add(2 * time.Hour),
			Limit:              5,
			MaxTransfers:       3,
			MaxWalkMeters:      600,
		})
		if len(itineraries) != 1 {
			t.Fatalf("expected 1 transfer-walk itinerary, got %+v", itineraries)
		}
		it := itineraries[0]
		if it.Transfers != 1 || len(it.Legs) != 3 || it.Legs[1].Mode != "walk" {
			t.Fatalf("expected transit->walk->transit, got %+v", it.Legs)
		}
		if it.Legs[1].FromStopID != 2 || it.Legs[1].ToStopID != 3 || it.WalkingMinutes != it.Legs[1].DurationMinutes {
			t.Fatalf("unexpected transfer walk leg: %+v", it.Legs[1])
		}
	})
}

func TestPlanJourneysMultiTransferUpToThreeTransfers(t *testing.T) {
	stops := []Stop{
		{ID: 1, Name: "S1", Latitude: 43.40, Longitude: -80.40, IsActive: true},
		{ID: 2, Name: "S2", Latitude: 43.42, Longitude: -80.40, IsActive: true},
		{ID: 3, Name: "S3", Latitude: 43.44, Longitude: -80.40, IsActive: true},
		{ID: 4, Name: "S4", Latitude: 43.46, Longitude: -80.40, IsActive: true},
		{ID: 5, Name: "S5", Latitude: 43.48, Longitude: -80.40, IsActive: true},
	}
	routes := []Route{
		{ID: 1, Number: "1", Name: "R1", Direction: "direction_0", IsActive: true},
		{ID: 2, Number: "2", Name: "R2", Direction: "direction_0", IsActive: true},
		{ID: 3, Number: "3", Name: "R3", Direction: "direction_0", IsActive: true},
		{ID: 4, Number: "4", Name: "R4", Direction: "direction_0", IsActive: true},
	}
	stopTimes := []JourneyStopTimeRow{
		{TripID: 1, RouteID: 1, ScheduleType: "daily", StopID: 1, StopSequence: 1, ArrivalSec: sec(10, 0), DepartureSec: sec(10, 0)},
		{TripID: 1, RouteID: 1, ScheduleType: "daily", StopID: 2, StopSequence: 2, ArrivalSec: sec(10, 10), DepartureSec: sec(10, 10)},
		{TripID: 2, RouteID: 2, ScheduleType: "daily", StopID: 2, StopSequence: 1, ArrivalSec: sec(10, 15), DepartureSec: sec(10, 15)},
		{TripID: 2, RouteID: 2, ScheduleType: "daily", StopID: 3, StopSequence: 2, ArrivalSec: sec(10, 25), DepartureSec: sec(10, 25)},
		{TripID: 3, RouteID: 3, ScheduleType: "daily", StopID: 3, StopSequence: 1, ArrivalSec: sec(10, 30), DepartureSec: sec(10, 30)},
		{TripID: 3, RouteID: 3, ScheduleType: "daily", StopID: 4, StopSequence: 2, ArrivalSec: sec(10, 40), DepartureSec: sec(10, 40)},
		{TripID: 4, RouteID: 4, ScheduleType: "daily", StopID: 4, StopSequence: 1, ArrivalSec: sec(10, 45), DepartureSec: sec(10, 45)},
		{TripID: 4, RouteID: 4, ScheduleType: "daily", StopID: 5, StopSequence: 2, ArrivalSec: sec(10, 55), DepartureSec: sec(10, 55)},
	}
	net := JourneyNetwork{Stops: stops, Routes: routes, StopTimes: stopTimes}
	from := time.Date(2026, 11, 18, 14, 55, 0, 0, time.UTC)

	// 2 transfers (S1 -> S4)
	if res := PlanJourneys(net, JourneyRepoQuery{OriginStopIDs: []int{1}, DestinationStopIDs: []int{4}, From: from, Until: from.Add(2 * time.Hour), Limit: 5, MaxTransfers: 1, MaxWalkMeters: 600}); len(res) != 0 {
		t.Fatalf("expected 0 itineraries with MaxTransfers=1, got %+v", res)
	}
	res2 := PlanJourneys(net, JourneyRepoQuery{OriginStopIDs: []int{1}, DestinationStopIDs: []int{4}, From: from, Until: from.Add(2 * time.Hour), Limit: 5, MaxTransfers: 3, MaxWalkMeters: 600})
	if len(res2) != 1 || res2[0].Transfers != 2 || len(res2[0].Legs) != 3 {
		t.Fatalf("expected 2-transfer itinerary, got %+v", res2)
	}

	// 3 transfers (S1 -> S5)
	if res := PlanJourneys(net, JourneyRepoQuery{OriginStopIDs: []int{1}, DestinationStopIDs: []int{5}, From: from, Until: from.Add(2 * time.Hour), Limit: 5, MaxTransfers: 2, MaxWalkMeters: 600}); len(res) != 0 {
		t.Fatalf("expected 0 itineraries with MaxTransfers=2, got %+v", res)
	}
	res3 := PlanJourneys(net, JourneyRepoQuery{OriginStopIDs: []int{1}, DestinationStopIDs: []int{5}, From: from, Until: from.Add(2 * time.Hour), Limit: 5, MaxTransfers: 3, MaxWalkMeters: 600})
	if len(res3) != 1 || res3[0].Transfers != 3 || len(res3[0].Legs) != 4 {
		t.Fatalf("expected 3-transfer itinerary, got %+v", res3)
	}
}

func TestPlanJourneysZeroCoordinatesNoFalseWalk(t *testing.T) {
	net := JourneyNetwork{
		Stops: []Stop{
			{ID: 1, Name: "Stop One", Latitude: 0, Longitude: 0, IsActive: true},
			{ID: 2, Name: "Stop Two", Latitude: 0, Longitude: 0, IsActive: true},
		},
	}
	from := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	res := PlanJourneys(net, JourneyRepoQuery{
		OriginStopIDs:      []int{1},
		DestinationStopIDs: []int{2},
		From:               from,
		Until:              from.Add(2 * time.Hour),
		Limit:              5,
		MaxTransfers:       3,
		MaxWalkMeters:      600,
	})
	if len(res) != 0 {
		t.Fatalf("expected no false walk between (0,0) stops, got %+v", res)
	}
}

func TestPlanJourneysDominanceAndAfterMidnightMultipleOptions(t *testing.T) {
	stops := []Stop{
		{ID: 1, Name: "A", Latitude: 43.40, Longitude: -80.40, IsActive: true},
		{ID: 2, Name: "B", Latitude: 43.45, Longitude: -80.45, IsActive: true},
		{ID: 3, Name: "C", Latitude: 43.48, Longitude: -80.48, IsActive: true},
	}
	routes := []Route{
		{ID: 1, Number: "1", Name: "Direct", Direction: "direction_0", IsActive: true},
		{ID: 2, Number: "2", Name: "Leg1", Direction: "direction_0", IsActive: true},
		{ID: 3, Number: "3", Name: "Leg2", Direction: "direction_0", IsActive: true},
	}
	stopTimes := []JourneyStopTimeRow{
		// Earlier departure, same arrival -> dominated by Trip 102
		{TripID: 101, RouteID: 1, ScheduleType: "daily", StopID: 1, StopSequence: 1, ArrivalSec: sec(11, 50), DepartureSec: sec(11, 50)},
		{TripID: 101, RouteID: 1, ScheduleType: "daily", StopID: 3, StopSequence: 2, ArrivalSec: sec(12, 30), DepartureSec: sec(12, 30)},
		// Direct 12:00 -> 12:30
		{TripID: 102, RouteID: 1, ScheduleType: "daily", StopID: 1, StopSequence: 1, ArrivalSec: sec(12, 0), DepartureSec: sec(12, 0)},
		{TripID: 102, RouteID: 1, ScheduleType: "daily", StopID: 3, StopSequence: 2, ArrivalSec: sec(12, 30), DepartureSec: sec(12, 30)},
		// Faster 1-transfer 12:05 -> 12:25 (non-dominated: arrives earlier, 1 transfer vs 0)
		{TripID: 201, RouteID: 2, ScheduleType: "daily", StopID: 1, StopSequence: 1, ArrivalSec: sec(12, 5), DepartureSec: sec(12, 5)},
		{TripID: 201, RouteID: 2, ScheduleType: "daily", StopID: 2, StopSequence: 2, ArrivalSec: sec(12, 12), DepartureSec: sec(12, 12)},
		{TripID: 301, RouteID: 3, ScheduleType: "daily", StopID: 2, StopSequence: 1, ArrivalSec: sec(12, 15), DepartureSec: sec(12, 15)},
		{TripID: 301, RouteID: 3, ScheduleType: "daily", StopID: 3, StopSequence: 2, ArrivalSec: sec(12, 25), DepartureSec: sec(12, 25)},
		// After-midnight weekday trip (25:10 on Friday 2026-11-13 = 2026-11-14T06:10Z)
		{TripID: 401, RouteID: 1, ScheduleType: "weekday", StopID: 1, StopSequence: 1, ArrivalSec: sec(25, 10), DepartureSec: sec(25, 10)},
		{TripID: 401, RouteID: 1, ScheduleType: "weekday", StopID: 3, StopSequence: 2, ArrivalSec: sec(25, 30), DepartureSec: sec(25, 30)},
	}
	net := JourneyNetwork{Stops: stops, Routes: routes, StopTimes: stopTimes}

	from := time.Date(2026, 11, 13, 16, 45, 0, 0, time.UTC) // 11:45 EST Friday
	res := PlanJourneys(net, JourneyRepoQuery{
		OriginStopIDs:      []int{1},
		DestinationStopIDs: []int{3},
		From:               from,
		Until:              from.Add(2 * time.Hour),
		Limit:              5,
		MaxTransfers:       3,
		MaxWalkMeters:      600,
	})
	if len(res) != 2 {
		t.Fatalf("expected 2 non-dominated itineraries (dominated 101 pruned), got %d: %+v", len(res), res)
	}
	if res[0].Transfers != 1 || res[1].Transfers != 0 || res[1].Legs[0].TripID != 102 {
		t.Fatalf("unexpected non-dominated ordering: %+v", res)
	}

	overnightFrom := time.Date(2026, 11, 14, 6, 0, 0, 0, time.UTC) // 01:00 EST Saturday
	overnight := PlanJourneys(net, JourneyRepoQuery{
		OriginStopIDs:      []int{1},
		DestinationStopIDs: []int{3},
		From:               overnightFrom,
		Until:              overnightFrom.Add(time.Hour),
		Limit:              5,
		MaxTransfers:       3,
		MaxWalkMeters:      600,
	})
	if len(overnight) != 1 || overnight[0].Legs[0].TripID != 401 || overnight[0].Legs[0].ServiceDate != "2026-11-13" {
		t.Fatalf("expected after-midnight Friday trip 401, got %+v", overnight)
	}
}

func TestPlanJourneys24HourWindowAcrossOvernightGap(t *testing.T) {
	stops := []Stop{
		{ID: 1, Name: "Origin", Latitude: 43.45, Longitude: -80.49, IsActive: true},
		{ID: 2, Name: "Mid", Latitude: 43.46, Longitude: -80.47, IsActive: true},
		{ID: 3, Name: "Dest", Latitude: 43.47, Longitude: -80.45, IsActive: true},
	}
	routes := []Route{
		{ID: 1, Number: "1", Name: "Morning A", Direction: "direction_0", IsActive: true},
		{ID: 2, Number: "2", Name: "Morning B", Direction: "direction_0", IsActive: true},
	}
	stopTimes := []JourneyStopTimeRow{
		{TripID: 1, RouteID: 1, ScheduleType: "daily", StopID: 1, StopSequence: 1, ArrivalSec: sec(6, 0), DepartureSec: sec(6, 0)},
		{TripID: 1, RouteID: 1, ScheduleType: "daily", StopID: 2, StopSequence: 2, ArrivalSec: sec(6, 20), DepartureSec: sec(6, 20)},
		{TripID: 2, RouteID: 2, ScheduleType: "daily", StopID: 2, StopSequence: 1, ArrivalSec: sec(6, 25), DepartureSec: sec(6, 25)},
		{TripID: 2, RouteID: 2, ScheduleType: "daily", StopID: 3, StopSequence: 2, ArrivalSec: sec(6, 45), DepartureSec: sec(6, 45)},
	}
	net := JourneyNetwork{Stops: stops, Routes: routes, StopTimes: stopTimes}

	// Search at 01:00 EST (5 hours before the 06:00 EST trip) with a 24-hour window.
	from := time.Date(2026, 11, 18, 6, 0, 0, 0, time.UTC)
	res := PlanJourneys(net, JourneyRepoQuery{
		OriginStopIDs:      []int{1},
		DestinationStopIDs: []int{3},
		From:               from,
		Until:              from.Add(24 * time.Hour),
		Limit:              5,
		MaxTransfers:       3,
		MaxWalkMeters:      600,
	})
	if len(res) != 1 || res[0].Transfers != 1 || res[0].DurationMinutes != 45 {
		t.Fatalf("expected 1 morning itinerary despite 5h wait from search start, got %+v", res)
	}
}

func TestPlanJourneysOrderedLegStops(t *testing.T) {
	stops := []Stop{
		{ID: 1, Name: "Central Station", Latitude: 43.450, Longitude: -80.490, IsActive: true},
		{ID: 2, Name: "Midway Stop", Latitude: 43.460, Longitude: -80.480, IsActive: true},
		{ID: 3, Name: "Fairway Station", Latitude: 43.470, Longitude: -80.470, IsActive: true},
	}
	routes := []Route{
		{ID: 7, Number: "7", Name: "King", Direction: "direction_0", IsActive: true},
	}
	stopTimes := []JourneyStopTimeRow{
		{TripID: 100, RouteID: 7, ScheduleType: "daily", RouteStopID: 701, StopID: 1, StopSequence: 1, ArrivalSec: sec(9, 0), DepartureSec: sec(9, 0)},
		{TripID: 100, RouteID: 7, ScheduleType: "daily", RouteStopID: 702, StopID: 2, StopSequence: 2, ArrivalSec: sec(9, 8), DepartureSec: sec(9, 9)},
		{TripID: 100, RouteID: 7, ScheduleType: "daily", RouteStopID: 703, StopID: 3, StopSequence: 3, ArrivalSec: sec(9, 20), DepartureSec: sec(9, 20)},
	}
	from := time.Date(2026, 11, 18, 13, 55, 0, 0, time.UTC) // 08:55 EST
	res := PlanJourneys(JourneyNetwork{Stops: stops, Routes: routes, StopTimes: stopTimes}, JourneyRepoQuery{
		OriginStopIDs:      []int{1},
		DestinationStopIDs: []int{3},
		From:               from,
		Until:              from.Add(2 * time.Hour),
		Limit:              5,
		MaxTransfers:       3,
		MaxWalkMeters:      600,
	})
	if len(res) != 1 || len(res[0].Legs) != 1 {
		t.Fatalf("expected 1 direct itinerary, got %+v", res)
	}
	legStops := res[0].Legs[0].Stops
	if len(legStops) != 3 {
		t.Fatalf("expected 3 ordered stops on leg, got %+v", legStops)
	}
	if legStops[0].RouteStopID != 701 || legStops[1].RouteStopID != 702 || legStops[1].StopName != "Midway Stop" || legStops[2].RouteStopID != 703 {
		t.Fatalf("unexpected ordered leg stops: %+v", legStops)
	}
}

func TestPlanJourneysDisruptions(t *testing.T) {
	ptrInt := func(v int) *int { return &v }
	ptrTime := func(v time.Time) *time.Time { return &v }

	stops := []Stop{
		{ID: 1, Name: "Origin", Latitude: 43.4400, Longitude: -80.4900, IsActive: true},
		{ID: 2, Name: "Nearby Origin", Latitude: 43.4410, Longitude: -80.4900, IsActive: true}, // ~111m from Origin
		{ID: 3, Name: "Mid Hub", Latitude: 43.4550, Longitude: -80.4800, IsActive: true},
		{ID: 4, Name: "Destination", Latitude: 43.4700, Longitude: -80.4700, IsActive: true},
	}
	routes := []Route{
		{ID: 1, Number: "1", Name: "Primary", Direction: "direction_0", IsActive: true},
		{ID: 2, Number: "2", Name: "Backup", Direction: "direction_0", IsActive: true},
	}
	// Route 1: Origin(1) -> Mid Hub(3) -> Destination(4) at 10:00, 10:15, 10:30 EST (15:00Z, 15:15Z, 15:30Z)
	// Also Route 1 later trip at 12:00 -> 12:15 -> 12:30 EST (17:00Z, 17:15Z, 17:30Z)
	// Route 2: Nearby Origin(2) -> Destination(4) at 10:05 -> 10:40 EST (15:05Z -> 15:40Z)
	stopTimes := []JourneyStopTimeRow{
		{TripID: 11, RouteID: 1, ScheduleType: "daily", RouteStopID: 101, StopID: 1, StopSequence: 1, ArrivalSec: sec(10, 0), DepartureSec: sec(10, 0)},
		{TripID: 11, RouteID: 1, ScheduleType: "daily", RouteStopID: 102, StopID: 3, StopSequence: 2, ArrivalSec: sec(10, 15), DepartureSec: sec(10, 15)},
		{TripID: 11, RouteID: 1, ScheduleType: "daily", RouteStopID: 103, StopID: 4, StopSequence: 3, ArrivalSec: sec(10, 30), DepartureSec: sec(10, 30)},
		{TripID: 12, RouteID: 1, ScheduleType: "daily", RouteStopID: 101, StopID: 1, StopSequence: 1, ArrivalSec: sec(12, 0), DepartureSec: sec(12, 0)},
		{TripID: 12, RouteID: 1, ScheduleType: "daily", RouteStopID: 102, StopID: 3, StopSequence: 2, ArrivalSec: sec(12, 15), DepartureSec: sec(12, 15)},
		{TripID: 12, RouteID: 1, ScheduleType: "daily", RouteStopID: 103, StopID: 4, StopSequence: 3, ArrivalSec: sec(12, 30), DepartureSec: sec(12, 30)},
		{TripID: 21, RouteID: 2, ScheduleType: "daily", RouteStopID: 201, StopID: 2, StopSequence: 1, ArrivalSec: sec(10, 5), DepartureSec: sec(10, 5)},
		{TripID: 21, RouteID: 2, ScheduleType: "daily", RouteStopID: 202, StopID: 4, StopSequence: 2, ArrivalSec: sec(10, 40), DepartureSec: sec(10, 40)},
	}

	windowFrom := time.Date(2026, 11, 18, 14, 55, 0, 0, time.UTC) // 09:55 EST
	baseQuery := JourneyRepoQuery{
		OriginStopIDs:      []int{1},
		DestinationStopIDs: []int{4},
		From:               windowFrom,
		Until:              windowFrom.Add(3 * time.Hour),
		Limit:              5,
		MaxTransfers:       3,
		MaxWalkMeters:      600,
	}

	t.Run("route-wide cancellation excludes cancelled trip and allows later trip outside window", func(t *testing.T) {
		cancelStart := time.Date(2026, 11, 18, 14, 50, 0, 0, time.UTC)
		cancelEnd := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)
		plan := PlanJourneysDetailed(JourneyNetwork{
			Stops:     stops,
			Routes:    routes,
			StopTimes: stopTimes,
			Disruptions: []JourneyDisruption{
				{ID: 1, Type: "cancellation", Title: "Route 1 morning cancellation", StartTime: cancelStart, EndTime: ptrTime(cancelEnd), Status: "active", RouteID: 1, StopID: nil},
				{ID: 99, Type: "cancellation", Title: "Unrelated route cancellation", StartTime: cancelStart, EndTime: ptrTime(cancelEnd), Status: "active", RouteID: 999, StopID: nil},
			},
		}, baseQuery)

		if len(plan.Itineraries) != 2 {
			t.Fatalf("expected backup Route 2 and afternoon Route 1, got %+v", plan.Itineraries)
		}
		if plan.Itineraries[0].Legs[1].TripID != 21 || plan.Itineraries[1].Legs[0].TripID != 12 {
			t.Fatalf("unexpected itineraries after cancellation: %+v", plan.Itineraries)
		}
		if len(plan.ExcludedDisruptions) != 1 || plan.ExcludedDisruptions[0].ID != 1 {
			t.Fatalf("expected only triggered disruption 1, got %+v", plan.ExcludedDisruptions)
		}
	})

	t.Run("stop_closure routes around closed origin stop via nearby open stop", func(t *testing.T) {
		start := time.Date(2026, 11, 18, 14, 0, 0, 0, time.UTC)
		plan := PlanJourneysDetailed(JourneyNetwork{
			Stops:     stops,
			Routes:    routes,
			StopTimes: stopTimes,
			Disruptions: []JourneyDisruption{
				{ID: 2, Type: "stop_closure", Title: "Origin stop closed", StartTime: start, EndTime: nil, Status: "active", RouteID: 1, StopID: ptrInt(1)},
			},
		}, baseQuery)

		if len(plan.Itineraries) != 1 || plan.Itineraries[0].Legs[0].Mode != "walk" || plan.Itineraries[0].Legs[1].TripID != 21 {
			t.Fatalf("expected walk to nearby open stop + Route 2, got %+v", plan.Itineraries)
		}
		if len(plan.ExcludedDisruptions) != 1 || plan.ExcludedDisruptions[0].ID != 2 {
			t.Fatalf("expected disruption 2 recorded, got %+v", plan.ExcludedDisruptions)
		}
	})

	t.Run("segment-level road_closure severs route through intermediate stop", func(t *testing.T) {
		start := time.Date(2026, 11, 18, 14, 0, 0, 0, time.UTC)
		plan := PlanJourneysDetailed(JourneyNetwork{
			Stops:     stops,
			Routes:    routes,
			StopTimes: stopTimes,
			Disruptions: []JourneyDisruption{
				{ID: 3, Type: "road_closure", Title: "Mid Hub road closure", StartTime: start, EndTime: nil, Status: "scheduled", RouteID: 1, StopID: ptrInt(3)},
			},
		}, baseQuery)

		for _, it := range plan.Itineraries {
			for _, leg := range it.Legs {
				if leg.Mode == "transit" && leg.Route != nil && leg.Route.ID == 1 {
					t.Fatalf("route 1 should be severed through closed Mid Hub, got %+v", it)
				}
			}
		}
		if len(plan.Itineraries) != 1 || plan.Itineraries[0].Legs[1].TripID != 21 {
			t.Fatalf("expected fallback to Route 2, got %+v", plan.Itineraries)
		}
	})

	t.Run("stop-level detour blocks bypassed stop while keeping non-bypassed segment usable", func(t *testing.T) {
		start := time.Date(2026, 11, 18, 14, 0, 0, 0, time.UTC)
		detourNet := JourneyNetwork{
			Stops:     stops,
			Routes:    routes,
			StopTimes: stopTimes,
			Disruptions: []JourneyDisruption{
				{ID: 4, Type: "detour", Title: "Detour bypassing Mid Hub", StartTime: start, EndTime: nil, Status: "active", RouteID: 1, StopID: ptrInt(3)},
			},
		}
		// Origin(1) -> Mid Hub(3) is blocked because Mid Hub is bypassed by detour.
		toMid := PlanJourneysDetailed(detourNet, JourneyRepoQuery{
			OriginStopIDs:      []int{1},
			DestinationStopIDs: []int{3},
			From:               windowFrom,
			Until:              windowFrom.Add(time.Hour),
			Limit:              5,
			MaxTransfers:       3,
			MaxWalkMeters:      0,
		})
		if len(toMid.Itineraries) != 0 || len(toMid.ExcludedDisruptions) != 1 {
			t.Fatalf("expected Mid Hub to be blocked by detour, got %+v", toMid)
		}
		// Origin(1) -> Destination(4) on Route 1 stays usable because neither endpoint is bypassed.
		toDest := PlanJourneysDetailed(detourNet, JourneyRepoQuery{
			OriginStopIDs:      []int{1},
			DestinationStopIDs: []int{4},
			From:               windowFrom,
			Until:              windowFrom.Add(time.Hour),
			Limit:              5,
			MaxTransfers:       3,
			MaxWalkMeters:      0,
		})
		if len(toDest.Itineraries) != 1 || toDest.Itineraries[0].Legs[0].TripID != 11 {
			t.Fatalf("expected Origin->Destination on Route 1 to remain usable, got %+v", toDest)
		}
	})

	t.Run("resolved and advisory delay or accessibility disruptions do not block itineraries", func(t *testing.T) {
		start := time.Date(2026, 11, 18, 14, 0, 0, 0, time.UTC)
		plan := PlanJourneysDetailed(JourneyNetwork{
			Stops:     stops,
			Routes:    routes,
			StopTimes: stopTimes,
			Disruptions: []JourneyDisruption{
				{ID: 5, Type: "cancellation", Title: "Resolved cancellation", StartTime: start, Status: "resolved", RouteID: 1, StopID: nil},
				{ID: 6, Type: "delay", Title: "Minor traffic delay", StartTime: start, Status: "active", RouteID: 1, StopID: nil},
				{ID: 7, Type: "accessibility", Title: "Ramp notice", StartTime: start, Status: "active", RouteID: 1, StopID: ptrInt(1)},
			},
		}, baseQuery)
		if len(plan.Itineraries) < 2 || plan.Itineraries[0].Legs[0].TripID != 11 || len(plan.ExcludedDisruptions) != 0 {
			t.Fatalf("resolved/advisory disruptions should not block trip 11, got %+v", plan)
		}
	})
}
