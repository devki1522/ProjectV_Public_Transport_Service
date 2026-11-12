import { randomBytes } from 'node:crypto'
import { readFile, writeFile } from 'node:fs/promises'

const root = new URL('../', import.meta.url)

async function ensureExample(relativePath) {
  const target = new URL(relativePath, root)
  try {
    return await readFile(target, 'utf8')
  } catch (error) {
    if (error.code !== 'ENOENT') throw error
    const example = await readFile(new URL(`${relativePath}.example`, root), 'utf8')
    await writeFile(target, example, { flag: 'wx', mode: 0o600 })
    return example
  }
}

await ensureExample('.env')
let contents = await ensureExample('backend/.env')
function configured(key) {
  const match = contents.match(new RegExp(`^${key}=(.*)$`, 'm'))
  return match?.[1].trim().replace(/^(['"])(.*)\1$/, '$2')
}

if ((configured('APP_ENV') && configured('APP_ENV') !== 'development') ||
    (configured('AUTH_PROVIDER') && configured('AUTH_PROVIDER') !== 'mock')) {
  throw new Error('This setup command only configures local development with mock authentication. Existing environment/provider settings were preserved.')
}

const values = {
  APP_ENV: 'development',
  AUTH_PROVIDER: 'mock',
  AUTH_REQUIRED_ROLE: 'TransitAdministrator',
  AUTH_SESSION_COOKIE: 'transit_session',
  AUTH_COOKIE_SECURE: 'false',
  AUTH_TRUSTED_ORIGINS: 'http://localhost:3000',
  MOCK_JWT_ISSUER: 'transit-dev',
  MOCK_JWT_AUDIENCE: 'transit-api',
  MOCK_JWT_SECRET: randomBytes(32).toString('base64url'),
  MOCK_ADMIN_PASSWORD: randomBytes(24).toString('base64url'),
}

for (const [key, value] of Object.entries(values)) {
  if (configured(key)) continue
  const pattern = new RegExp(`^${key}=.*$`, 'm')
  contents = pattern.test(contents)
    ? contents.replace(pattern, `${key}=${value}`)
    : `${contents.trimEnd()}\n${key}=${value}\n`
}
await writeFile(new URL('backend/.env', root), contents, { mode: 0o600 })
console.log('Local mock authentication configured in backend/.env. Existing nonempty settings were preserved.')
console.log('Use transit.admin with MOCK_ADMIN_PASSWORD in backend/.env. Secrets are not printed.')
