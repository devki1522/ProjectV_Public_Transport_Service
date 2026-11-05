import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { PGlite } from '@electric-sql/pglite'
import { loadSeedSQL } from './seed-grt.mjs'

test('SQL seed loads valid linked data, preserves edits on rerun, and refuses conflicts', async () => {
  const sql = loadSeedSQL()
  const db = new PGlite()
  try {
    await db.exec(sql)
    const counts = (await db.query(`SELECT
      (SELECT count(*)::int FROM routes) AS routes,
      (SELECT count(*)::int FROM stops) AS stops,
      (SELECT count(*)::int FROM route_shapes) AS shapes,
      (SELECT count(*)::int FROM route_patterns) AS patterns,
      (SELECT count(*)::int FROM route_pattern_stops) AS pattern_stops,
      (SELECT count(*)::int FROM route_stops) AS route_stops,
      (SELECT count(*)::int FROM trips) AS trips,
      (SELECT count(*)::int FROM stop_times) AS stop_times,
      (SELECT count(*)::int FROM seed_imports) AS imports`)).rows[0]
    assert.deepEqual(counts, { routes: 104, stops: 2297, shapes: 183, patterns: 186,
      pattern_stops: 4523, route_stops: 2862, trips: 6666, stop_times: 190327, imports: 2 })
    assert.deepEqual((await db.query(`SELECT schedule_type, count(*)::int AS n
      FROM trips GROUP BY schedule_type ORDER BY schedule_type`)).rows,
    [{ schedule_type: 'weekday', n: 3974 }, { schedule_type: 'weekend', n: 2692 }])
    assert.equal((await db.query('SELECT count(DISTINCT route_id)::int AS n FROM trips')).rows[0].n, 104)
    assert.equal((await db.query('SELECT count(*)::int AS n FROM stops WHERE parent_station_id IS NOT NULL')).rows[0].n, 71)
    assert.ok((await db.query("SELECT count(*)::int AS n FROM stop_times WHERE arrival_time > INTERVAL '24 hours'")).rows[0].n > 0)
    assert.ok((await db.query(`SELECT count(*)::int AS n FROM (
      SELECT pattern_id, stop_id FROM route_pattern_stops GROUP BY pattern_id, stop_id HAVING count(*) > 1
    ) repeated_stops`)).rows[0].n > 0)
    assert.equal((await db.query(`SELECT count(*)::int AS n FROM stop_times st
      JOIN trips t ON t.id = st.trip_id JOIN route_stops rs ON rs.id = st.route_stop_id
      WHERE st.route_id <> t.route_id OR st.route_id <> rs.route_id`)).rows[0].n, 0)
    assert.equal((await db.query(`SELECT count(*)::int AS n FROM (
      SELECT route_id FROM route_patterns GROUP BY route_id HAVING count(*) FILTER (WHERE is_primary) <> 1
    ) invalid_primary`)).rows[0].n, 0)
    assert.equal((await db.query(`SELECT count(*)::int AS n FROM (
      (SELECT route_id, stop_id, stop_sequence FROM route_stops EXCEPT
       SELECT p.route_id, ps.stop_id, ps.stop_sequence FROM route_patterns p
       JOIN route_pattern_stops ps ON ps.pattern_id = p.id WHERE p.is_primary)
      UNION ALL
      (SELECT p.route_id, ps.stop_id, ps.stop_sequence FROM route_patterns p
       JOIN route_pattern_stops ps ON ps.pattern_id = p.id WHERE p.is_primary
       EXCEPT SELECT route_id, stop_id, stop_sequence FROM route_stops)
    ) differences`)).rows[0].n, 0)

    await assert.rejects(db.exec(`INSERT INTO stop_times (trip_id, route_stop_id, route_id, arrival_time, departure_time)
      SELECT t.id, rs.id, t.route_id, INTERVAL '1 hour', INTERVAL '1 hour'
      FROM trips t CROSS JOIN route_stops rs WHERE t.route_id <> rs.route_id LIMIT 1`), /foreign key/)
    await assert.rejects(db.exec('UPDATE stops SET latitude = 91 WHERE id = 1'), /check constraint/)
    await assert.rejects(db.exec("UPDATE stop_times SET departure_time = arrival_time - INTERVAL '1 second' WHERE id = 1"), /check constraint/)

    const IDs = (await db.query('SELECT id, source_id FROM stops ORDER BY id')).rows
    await db.exec("UPDATE stops SET name = 'Team edit' WHERE id = 1; UPDATE trips SET is_active = false WHERE id = 1")
    await db.exec(sql)
    assert.deepEqual((await db.query('SELECT id, source_id FROM stops ORDER BY id')).rows, IDs)
    assert.equal((await db.query('SELECT name FROM stops WHERE id = 1')).rows[0].name, 'Team edit')
    assert.equal((await db.query('SELECT is_active FROM trips WHERE id = 1')).rows[0].is_active, false)
    assert.equal((await db.query('SELECT count(*)::int AS n FROM stop_times')).rows[0].n, 190327)
    // The explicit internal trip IDs must not break subsequent generated IDs.
    assert.equal((await db.query("INSERT INTO trips (route_id,schedule_type) SELECT id,'daily' FROM routes LIMIT 1 RETURNING id")).rows[0].id, 6667)
  } finally { await db.close() }

  const conflicting = new PGlite()
  try {
    await conflicting.exec(readFileSync(new URL('../database/migrations/001_initial_schema.sql', import.meta.url), 'utf8'))
    await conflicting.exec("INSERT INTO routes (number,name,direction) VALUES ('test','Existing route','northbound')")
    await assert.rejects(conflicting.exec(sql), /requires empty transit location tables/)
    await conflicting.exec('ROLLBACK')
    assert.equal((await conflicting.query('SELECT name FROM routes')).rows[0].name, 'Existing route')
    assert.equal((await conflicting.query('SELECT count(*)::int AS n FROM stops')).rows[0].n, 0)
    assert.equal((await conflicting.query('SELECT count(*)::int AS n FROM seed_imports')).rows[0].n, 0)
  } finally { await conflicting.close() }
})
