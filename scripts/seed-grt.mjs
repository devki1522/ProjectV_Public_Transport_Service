import { readFileSync } from 'node:fs'
import { spawnSync } from 'node:child_process'
import { pathToFileURL } from 'node:url'

const root = new URL('../', import.meta.url)

export function loadSeedSQL() {
  const schema = readFileSync(new URL('database/migrations/001_initial_schema.sql', root), 'utf8')
  const seed = readFileSync(new URL('database/seeds/grt.sql', root), 'utf8')
  return `BEGIN;
SET LOCAL standard_conforming_strings = on;
SET LOCAL search_path = public;
SELECT pg_advisory_xact_lock(714208301);
DO $schema$
BEGIN
    IF to_regclass('public.routes') IS NULL THEN
        EXECUTE '${schema.replaceAll("'", "''")}';
    ELSIF to_regclass('public.route_patterns') IS NULL OR NOT EXISTS (
        SELECT 1 FROM information_schema.columns WHERE table_schema = 'public'
        AND table_name = 'routes' AND column_name = 'source_id'
    ) THEN
        RAISE EXCEPTION 'Existing database uses an older development schema. Recreate it from 001_initial_schema.sql before seeding.';
    END IF;
END $schema$;
${seed}
COMMIT;
`
}

function main() {
  const args = process.argv.slice(2)
  if (args.length && !(args.length === 1 && args[0] === '--local')) {
    throw new Error('Usage: npm run db:seed -- [--local]')
  }
  const local = args[0] === '--local'
  const command = local ? 'psql' : 'docker'
  const commandArgs = local ? ['-X', '-v', 'ON_ERROR_STOP=1'] : [
    'compose', 'exec', '-T', 'database', 'sh', '-c',
    'exec psql -X -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1',
  ]
  const result = spawnSync(command, commandArgs, {
    cwd: root, input: loadSeedSQL(), stdio: ['pipe', 'inherit', 'inherit'],
  })
  if (result.error) throw new Error(`Could not run ${command}: ${result.error.message}. See database/README.md.`)
  if (result.status !== 0) { process.exitCode = result.status || 1; return }
  console.log('GRT database seed complete.')
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { main() } catch (error) { console.error(error.message); process.exitCode = 1 }
}
