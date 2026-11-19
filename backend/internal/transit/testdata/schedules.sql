-- Minimal independent fixture; integration tests use session-local temp tables.
CREATE TEMP TABLE routes(id integer primary key, number text, name text, direction text, is_active boolean);
CREATE TEMP TABLE stops(id integer primary key, name text, latitude double precision DEFAULT 0, longitude double precision DEFAULT 0, wheelchair_boarding integer DEFAULT 0, is_active boolean);
CREATE TEMP TABLE trips(id integer primary key, route_id integer, schedule_type text, is_active boolean);
CREATE TEMP TABLE route_stops(id integer primary key, route_id integer, stop_id integer, stop_sequence integer);
CREATE TEMP TABLE stop_times(id integer primary key, trip_id integer, route_id integer, route_stop_id integer, arrival_time interval, departure_time interval);
CREATE TEMP TABLE disruptions(id integer primary key, type varchar(30), title varchar(100), description text, start_time timestamptz, end_time timestamptz, status varchar(20), source_service varchar(50));
CREATE TEMP TABLE disruption_routes(id integer primary key, disruption_id integer, route_id integer, stop_id integer);
INSERT INTO routes VALUES (7,'7','Main','direction_0',true),(8,'8','Inactive','direction_1',false),(9,'9','Crosstown','direction_0',true),(10,'10','Connector','direction_0',true),(11,'11','Northbound','direction_0',true);
INSERT INTO stops(id, name, latitude, longitude, is_active) VALUES (1,'Station',0,0,true),(2,'Inactive',0,0,false),(3,'Midtown Hub',43.4500,-80.4900,true),(4,'University Terminal',43.4700,-80.5400,true),(5,'Parkside',43.4300,-80.4400,true),(6,'Opposite University',43.4708,-80.5400,true),(7,'Northfield',43.4950,-80.5600,true);
INSERT INTO trips VALUES (10,7,'weekday',true),(11,7,'weekend',true),(12,7,'daily',true),(13,7,'weekday',false),(14,8,'daily',true),(15,7,'daily',true),(16,9,'daily',true),(17,10,'daily',true),(18,11,'daily',true);
INSERT INTO route_stops VALUES (20,7,1,1),(21,7,1,2),(22,7,2,3),(23,8,1,1),(24,9,5,1),(25,9,3,2),(26,10,3,1),(27,10,4,2),(28,11,4,1),(29,11,7,2);
INSERT INTO stop_times VALUES
 (1,10,7,20,'25:09:00','25:10:00'),(2,10,7,21,'26:09:00','26:10:00'),
 (3,11,7,20,'01:09:00','01:10:00'),(4,12,7,20,'01:29:00','01:30:00'),
 (5,13,7,20,'01:19:00','01:20:00'),(6,14,8,23,'01:19:00','01:20:00'),
 (7,12,7,22,'01:39:00','01:40:00'),(8,16,9,24,'08:00:00','08:00:00'),
 (9,16,9,25,'08:15:00','08:15:00'),(10,17,10,26,'08:25:00','08:25:00'),
 (11,17,10,27,'08:40:00','08:40:00'),(12,18,11,28,'08:45:00','08:45:00'),
 (13,18,11,29,'09:00:00','09:00:00');
SET LOCAL TIME ZONE 'Pacific/Auckland';
