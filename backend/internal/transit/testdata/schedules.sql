-- Minimal independent fixture; integration tests use session-local temp tables.
CREATE TEMP TABLE routes(id integer primary key, number text, name text, direction text, is_active boolean);
CREATE TEMP TABLE stops(id integer primary key, name text, is_active boolean);
CREATE TEMP TABLE trips(id integer primary key, route_id integer, schedule_type text, is_active boolean);
CREATE TEMP TABLE route_stops(id integer primary key, route_id integer, stop_id integer, stop_sequence integer);
CREATE TEMP TABLE stop_times(id integer primary key, trip_id integer, route_id integer, route_stop_id integer, arrival_time interval, departure_time interval);
INSERT INTO routes VALUES (7,'7','Main','direction_0',true),(8,'8','Inactive','direction_1',false);
INSERT INTO stops VALUES (1,'Station',true),(2,'Inactive',false);
INSERT INTO trips VALUES (10,7,'weekday',true),(11,7,'weekend',true),(12,7,'daily',true),(13,7,'weekday',false),(14,8,'daily',true),(15,7,'daily',true);
INSERT INTO route_stops VALUES (20,7,1,1),(21,7,1,2),(22,7,2,3),(23,8,1,1);
INSERT INTO stop_times VALUES
 (1,10,7,20,'25:09:00','25:10:00'),(2,10,7,21,'26:09:00','26:10:00'),
 (3,11,7,20,'01:09:00','01:10:00'),(4,12,7,20,'01:29:00','01:30:00'),
 (5,13,7,20,'01:19:00','01:20:00'),(6,14,8,23,'01:19:00','01:20:00'),
 (7,12,7,22,'01:39:00','01:40:00');
SET LOCAL TIME ZONE 'Pacific/Auckland';
