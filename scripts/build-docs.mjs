import { spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { zipSync } from 'fflate'

const root = fileURLToPath(new URL('../', import.meta.url))
const output = join(root, 'build', 'docs')
const html = join(output, 'html')
const archive = join(output, 'design-information.zip')
mkdirSync(output, { recursive: true })
// Clear only our generated outputs so removed pages cannot enter a new submission.
rmSync(html, { recursive: true, force: true })
rmSync(archive, { force: true })

const result = spawnSync('doxygen', ['Doxyfile'], { cwd: root, stdio: 'inherit' })
if (result.error) {
  console.error('Could not run Doxygen. Install it and add it to PATH.')
  console.error(result.error.message)
  process.exit(1)
}
if (result.status !== 0) process.exit(result.status ?? 1)
if (!existsSync(join(html, 'index.html'))) {
  console.error('Doxygen did not produce html/index.html.')
  process.exit(1)
}

const files = {}
function collect(directory, prefix) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name)
    const name = `${prefix}/${entry.name}`
    if (entry.isDirectory()) collect(path, name)
    else if (entry.isFile()) files[name] = readFileSync(path)
  }
}
collect(html, 'html')
writeFileSync(archive, zipSync(files))
console.log(`Created ${archive}`)
