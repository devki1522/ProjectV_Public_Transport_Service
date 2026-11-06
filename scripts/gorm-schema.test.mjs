import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'
import { PGlite } from '@electric-sql/pglite'

async function catalog(db) {
  const columns = (await db.query(`SELECT table_name, column_name, ordinal_position,
    data_type, character_maximum_length, is_nullable, column_default,
    is_identity, identity_generation
    FROM information_schema.columns WHERE table_schema = 'public'
    ORDER BY table_name, ordinal_position`)).rows
  // Compare index semantics, including unique constraints' backing indexes.
  const indexes = (await db.query(`SELECT t.relname AS table_name,
    i.indisunique, i.indisprimary,
    ARRAY(SELECT pg_get_indexdef(i.indexrelid, n, true)
      FROM generate_series(1, i.indnkeyatts) n) AS columns,
    pg_get_expr(i.indpred, i.indrelid) AS predicate
    FROM pg_index i JOIN pg_class t ON t.oid = i.indrelid
    JOIN pg_namespace ns ON ns.oid = t.relnamespace
    WHERE ns.nspname = 'public'`)).rows
  const constraints = (await db.query(`SELECT t.relname AS table_name,
    c.contype, pg_get_constraintdef(c.oid) AS definition
    FROM pg_constraint c JOIN pg_class t ON t.oid = c.conrelid
    JOIN pg_namespace ns ON ns.oid = t.relnamespace
    WHERE ns.nspname = 'public' AND c.contype IN ('f', 'c')`)).rows
  const sorted = rows => rows.sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b)))
  return { columns, indexes: sorted(indexes), constraints: sorted(constraints) }
}

test('GORM generates the same PostgreSQL schema as the SQL migration', async () => {
  const temp = mkdtempSync(join(tmpdir(), 'projectv-schema-'))
  const output = join(temp, 'ddl.json')
  const expected = new PGlite()
  const actual = new PGlite()
  try {
    const result = spawnSync('go', ['test', './internal/models', '-run', '^TestSchemaDDL$', '-count=1'], {
      cwd: fileURLToPath(new URL('../backend/', import.meta.url)),
      env: { ...process.env, PROJECTV_SCHEMA_DDL_OUTPUT: output },
      encoding: 'utf8',
    })
    assert.ifError(result.error)
    assert.equal(result.status, 0, result.stdout + result.stderr)
    const statements = JSON.parse(readFileSync(output, 'utf8'))
    await expected.exec(readFileSync(new URL('../database/migrations/001_initial_schema.sql', import.meta.url), 'utf8'))
    for (const statement of statements) await actual.exec(statement)
    assert.deepEqual(await catalog(actual), await catalog(expected))
  } finally {
    await expected.close()
    await actual.close()
    rmSync(temp, { recursive: true, force: true })
  }
})
