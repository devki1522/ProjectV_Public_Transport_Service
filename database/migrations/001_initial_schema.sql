-- This is the initial PostgreSQL schema.
-- Apply to an empty database. During early class development, edit this file
-- and recreate local databases when needed. Use migrations once data must persist.

CREATE TABLE routes (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    direction VARCHAR(20) NOT NULL
        -- GTFS 0/1 identify opposite traversals, not compass bearings.
        CHECK (direction IN ('northbound', 'southbound', 'eastbound', 'westbound', 'direction_0', 'direction_1')),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    source_id TEXT UNIQUE,
    route_type INTEGER CHECK (route_type >= 0),
    CONSTRAINT uq_routes_number_direction UNIQUE (number, direction)
);

CREATE TABLE stops (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    latitude DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    is_accessible BOOLEAN NOT NULL DEFAULT FALSE,
    source_id TEXT UNIQUE,
    stop_code TEXT,
    platform_code TEXT,
    stop_url TEXT,
    location_type INTEGER NOT NULL DEFAULT 0 CHECK (location_type BETWEEN 0 AND 4),
    parent_station_id INTEGER REFERENCES stops(id) ON DELETE RESTRICT,
    -- Preserve GTFS unknown (0), accessible (1), inaccessible (2).
    wheelchair_boarding INTEGER NOT NULL DEFAULT 0 CHECK (wheelchair_boarding BETWEEN 0 AND 2)
);

CREATE TABLE route_stops (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    route_id INTEGER NOT NULL REFERENCES routes(id) ON DELETE RESTRICT,
    stop_id INTEGER NOT NULL REFERENCES stops(id) ON DELETE RESTRICT,
    stop_sequence INTEGER NOT NULL CHECK (stop_sequence > 0),
    CONSTRAINT uq_route_stops_sequence UNIQUE (route_id, stop_sequence),
    CONSTRAINT uq_route_stops_id_route UNIQUE (id, route_id)
);

CREATE TABLE trips (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    route_id INTEGER NOT NULL REFERENCES routes(id) ON DELETE RESTRICT,
    schedule_type VARCHAR(20) NOT NULL
        CHECK (schedule_type IN ('weekday', 'weekend', 'daily')),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    CONSTRAINT uq_trips_id_route UNIQUE (id, route_id)
);

CREATE TABLE stop_times (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    trip_id INTEGER NOT NULL,
    route_stop_id INTEGER NOT NULL,
    route_id INTEGER NOT NULL,
    arrival_time INTERVAL NOT NULL,
    departure_time INTERVAL NOT NULL,
    CONSTRAINT fk_stop_times_trip_route FOREIGN KEY (trip_id, route_id)
        REFERENCES trips(id, route_id) ON DELETE CASCADE,
    CONSTRAINT fk_stop_times_route_stop_route FOREIGN KEY (route_stop_id, route_id)
        REFERENCES route_stops(id, route_id) ON DELETE RESTRICT,
    CONSTRAINT uq_stop_times_trip_route_stop UNIQUE (trip_id, route_stop_id),
    CONSTRAINT ck_stop_times_arrival_nonnegative CHECK (arrival_time >= INTERVAL '0 seconds'),
    CONSTRAINT ck_stop_times_departure_order CHECK (departure_time >= arrival_time)
);

CREATE TABLE vehicles (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number VARCHAR(20) NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    is_accessible BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE disruptions (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    type VARCHAR(30) NOT NULL CHECK (type IN (
        'delay', 'detour', 'cancellation', 'stop_closure',
        'road_closure', 'construction', 'emergency',
        'traffic_restriction', 'accessibility'
    )),
    title VARCHAR(100) NOT NULL,
    description TEXT,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ,
    status VARCHAR(20) NOT NULL DEFAULT 'scheduled'
        CHECK (status IN ('scheduled', 'active', 'resolved')),
    source_service VARCHAR(50) NOT NULL,
    CONSTRAINT ck_disruptions_time_order CHECK (end_time IS NULL OR end_time >= start_time)
);

CREATE TABLE disruption_routes (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    disruption_id INTEGER NOT NULL REFERENCES disruptions(id) ON DELETE CASCADE,
    route_id INTEGER NOT NULL REFERENCES routes(id) ON DELETE RESTRICT,
    stop_id INTEGER REFERENCES stops(id) ON DELETE RESTRICT
);

-- One route-wide entry per disruption/route.
CREATE UNIQUE INDEX uq_disruption_route_wide
    ON disruption_routes(disruption_id, route_id) WHERE stop_id IS NULL;
-- One entry per explicitly affected stop on a route.
CREATE UNIQUE INDEX uq_disruption_route_stop
    ON disruption_routes(disruption_id, route_id, stop_id) WHERE stop_id IS NOT NULL;

CREATE INDEX idx_route_stops_stop_id ON route_stops(stop_id);
CREATE INDEX idx_trips_route_id ON trips(route_id);
CREATE INDEX idx_stop_times_route_stop_id ON stop_times(route_stop_id);
CREATE INDEX idx_disruption_routes_route_id ON disruption_routes(route_id);
CREATE INDEX idx_disruption_routes_stop_id ON disruption_routes(stop_id);

-- Application rules not enforced by this schema:
-- * For a disruption_routes row with stop_id, confirm the stop belongs to the route.
-- * Require at least one disruption_routes row before publishing a disruption.
-- * Define how a disruption changes routing and departures; an alert alone is not a cancellation engine.
-- * A route's complete ordered stop list and schedule should be edited transactionally.

CREATE INDEX idx_stops_parent_station_id ON stops(parent_station_id);

CREATE TABLE route_shapes (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id TEXT NOT NULL UNIQUE,
    -- GeoJSON LineString with WGS84 [longitude, latitude] coordinates.
    geometry JSONB NOT NULL CHECK (COALESCE((
        geometry->>'type' = 'LineString'
        AND jsonb_typeof(geometry->'coordinates') = 'array'
        AND jsonb_array_length(geometry->'coordinates') >= 2
    ), FALSE))
);

-- A direction-specific route may have several branches/short turns. Keeping
-- these separate avoids inventing an ordered list by concatenating branches.
CREATE TABLE route_patterns (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id TEXT NOT NULL UNIQUE,
    route_id INTEGER NOT NULL REFERENCES routes(id) ON DELETE RESTRICT,
    shape_id INTEGER NOT NULL REFERENCES route_shapes(id) ON DELETE RESTRICT,
    headsign TEXT NOT NULL,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX idx_route_patterns_route_id ON route_patterns(route_id);
CREATE INDEX idx_route_patterns_shape_id ON route_patterns(shape_id);
CREATE UNIQUE INDEX uq_route_patterns_primary ON route_patterns(route_id) WHERE is_primary;

CREATE TABLE route_pattern_stops (
    pattern_id INTEGER NOT NULL REFERENCES route_patterns(id) ON DELETE CASCADE,
    stop_id INTEGER NOT NULL REFERENCES stops(id) ON DELETE RESTRICT,
    stop_sequence INTEGER NOT NULL CHECK (stop_sequence > 0),
    PRIMARY KEY (pattern_id, stop_sequence)
);
CREATE INDEX idx_route_pattern_stops_stop_id ON route_pattern_stops(stop_id);

-- Records the exact offline snapshot; repeating it does not overwrite edits.
CREATE TABLE seed_imports (
    name TEXT PRIMARY KEY,
    sha256 TEXT NOT NULL CHECK (length(sha256) = 64),
    imported_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
