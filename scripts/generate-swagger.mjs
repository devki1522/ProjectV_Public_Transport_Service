import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

// Pin the generator so local and CI output stays reproducible.
const result = spawnSync('go', [
  'run', 'github.com/swaggo/swag/cmd/swag@v1.16.6', 'init',
  '-g', 'main.go', '-d', 'cmd/api,internal', '--parseInternal',
  '--output', 'docs', '--outputTypes', 'go,json,yaml',
], {
  cwd: fileURLToPath(new URL('../backend/', import.meta.url)),
  stdio: 'inherit',
})
if (result.error) console.error(result.error.message)
process.exit(result.status ?? 1)
