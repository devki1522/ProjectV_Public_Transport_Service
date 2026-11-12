import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtemp, mkdir, readFile, writeFile, copyFile } from 'node:fs/promises'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'
import { parseEnv } from 'node:util'

const root = fileURLToPath(new URL('../', import.meta.url))

async function fixture(contents) {
  const base = join(root, 'build', 'auth-setup-tests')
  await mkdir(base, { recursive: true })
  const directory = await mkdtemp(join(base, 'case-'))
  await mkdir(join(directory, 'scripts'))
  await mkdir(join(directory, 'backend'))
  await copyFile(new URL('./setup-auth.mjs', import.meta.url), join(directory, 'scripts', 'setup-auth.mjs'))
  await writeFile(join(directory, '.env.example'), 'POSTGRES_DB=public_transport\n')
  await writeFile(join(directory, 'backend', '.env.example'), 'DATABASE_URL=preserved-database\nMOCK_JWT_SECRET=\n')
  if (contents) await writeFile(join(directory, 'backend', '.env'), contents)
  const run = () => spawnSync(process.execPath, ['scripts/setup-auth.mjs'], { cwd: directory, encoding: 'utf8' })
  const read = () => readFile(join(directory, 'backend', '.env'), 'utf8')
  return { run, read }
}

test('setup creates secrets without exposing them and preserves configuration on repeat runs', async () => {
  const { run, read } = await fixture()
  const first = run()
  assert.equal(first.status, 0)
  const contents = await read()
  const settings = parseEnv(contents)
  assert.equal(settings.DATABASE_URL, 'preserved-database')
  for (const key of ['MOCK_JWT_SECRET', 'MOCK_ADMIN_PASSWORD', 'MOCK_USER_PASSWORD']) {
    assert(settings[key].length >= 32)
    assert(!first.stdout.includes(settings[key]))
  }
  assert.equal(run().status, 0)
  assert.equal(await read(), contents)
})

test('setup refuses production and real-provider environments without modifying the backend file', async () => {
  for (const contents of ['APP_ENV=production\n', 'AUTH_PROVIDER=registration\n']) {
    const { run, read } = await fixture(contents)
    assert.notEqual(run().status, 0)
    assert.equal(await read(), contents)
  }
})

test('setup preserves existing credentials and database settings', async () => {
  const contents = 'DATABASE_URL=keep-me\nMOCK_ADMIN_PASSWORD=existing-password\n'
  const { run, read } = await fixture(contents)
  assert.equal(run().status, 0)
  const settings = parseEnv(await read())
  assert.equal(settings.DATABASE_URL, 'keep-me')
  assert.equal(settings.MOCK_ADMIN_PASSWORD, 'existing-password')
})
