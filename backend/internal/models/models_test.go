package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTableNames(t *testing.T) {
	tests := []struct {
		name     string
		model    interface{ TableName() string }
		expected string
	}{
		{"Route", Route{}, "routes"},
		{"Stop", Stop{}, "stops"},
		{"RouteStop", RouteStop{}, "route_stops"},
		{"Trip", Trip{}, "trips"},
		{"StopTime", StopTime{}, "stop_times"},
		{"Vehicle", Vehicle{}, "vehicles"},
		{"Disruption", Disruption{}, "disruptions"},
		{"DisruptionRoute", DisruptionRoute{}, "disruption_routes"},
		{"RouteShape", RouteShape{}, "route_shapes"},
		{"RoutePattern", RoutePattern{}, "route_patterns"},
		{"RoutePatternStop", RoutePatternStop{}, "route_pattern_stops"},
		{"SeedImport", SeedImport{}, "seed_imports"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.model.TableName(); got != tc.expected {
				t.Errorf("%s.TableName() = %q; want %q", tc.name, got, tc.expected)
			}
		})
	}
}

func TestJSONSerialization(t *testing.T) {
	sourceID := "GRT-100"
	routeType := 3
	now := time.Now().Truncate(time.Second)

	route := Route{
		ID:        1,
		Number:    "7",
		Name:      "Main",
		Direction: "direction_0",
		IsActive:  true,
		SourceID:  &sourceID,
		RouteType: &routeType,
	}

	data, err := json.Marshal(route)
	if err != nil {
		t.Fatalf("failed to marshal Route: %v", err)
	}

	var unmarshaled Route
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal Route: %v", err)
	}

	if unmarshaled.ID != route.ID || unmarshaled.Number != route.Number || *unmarshaled.SourceID != sourceID {
		t.Errorf("unmarshaled Route mismatch: got %+v, want %+v", unmarshaled, route)
	}

	disruption := Disruption{
		ID:            10,
		Type:          "detour",
		Title:         "Construction Detour",
		StartTime:     now,
		Status:        "active",
		SourceService: "transit",
	}

	dData, err := json.Marshal(disruption)
	if err != nil {
		t.Fatalf("failed to marshal Disruption: %v", err)
	}

	var unmarshaledD Disruption
	if err := json.Unmarshal(dData, &unmarshaledD); err != nil {
		t.Fatalf("failed to unmarshal Disruption: %v", err)
	}

	if unmarshaledD.Title != disruption.Title || unmarshaledD.Status != disruption.Status {
		t.Errorf("unmarshaled Disruption mismatch: got %+v, want %+v", unmarshaledD, disruption)
	}
}

func TestRouteShapeGeometry(t *testing.T) {
	geomJSON := `{"type":"LineString","coordinates":[[-80.5,43.5],[-80.4,43.4]]}`
	shape := RouteShape{
		ID:       1,
		SourceID: "shape-1",
		Geometry: json.RawMessage(geomJSON),
	}

	data, err := json.Marshal(shape)
	if err != nil {
		t.Fatalf("failed to marshal RouteShape: %v", err)
	}

	var unmarshaled RouteShape
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal RouteShape: %v", err)
	}

	if string(unmarshaled.Geometry) != geomJSON {
		t.Errorf("unmarshaled geometry = %s, want %s", string(unmarshaled.Geometry), geomJSON)
	}
}

func TestStopAccessibilityPreservesThreeStates(t *testing.T) {
	for _, state := range []int{WheelchairBoardingUnknown, WheelchairBoardingAccessible, WheelchairBoardingInaccessible} {
		data, err := json.Marshal(Stop{WheelchairBoarding: state})
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		if fields["wheelchair_boarding"] != float64(state) {
			t.Fatalf("accessibility state %d was lost: %s", state, data)
		}
		if _, exists := fields["is_accessible"]; exists {
			t.Fatalf("redundant accessibility boolean exposed: %s", data)
		}
	}
}
