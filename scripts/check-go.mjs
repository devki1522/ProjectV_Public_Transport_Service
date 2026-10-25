import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

const mode = process.argv[2]
if (!['format', 'vet'].includes(mode)) {
  console.error('Usage: node scripts/check-go.mjs <format|vet>')
  process.exit(1)
}

const command = mode === 'format' ? 'gofmt' : 'go'
const result = spawnSync(command, mode === 'format' ? ['-l', '.'] : ['vet', './...'], {
  cwd: fileURLToPath(new URL('../backend/', import.meta.url)),
  encoding: 'utf8',
  stdio: mode === 'format' ? 'pipe' : 'inherit',
})

if (result.error) {
  console.error(`Could not run ${command}. Ensure Go's bin directory is on PATH.`)
  console.error(result.error.message)
  process.exit(1)
}
if (result.stderr) process.stderr.write(result.stderr)
if (result.status !== 0) process.exit(result.status ?? 1)
if (mode === 'format' && result.stdout.trim()) {
  console.error('Run gofmt -w on these files from backend/:')
  process.stderr.write(result.stdout)
  process.exit(1)
}
