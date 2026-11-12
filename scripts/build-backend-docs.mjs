import { spawn, spawnSync } from 'node:child_process'
import { once } from 'node:events'
import { createServer } from 'node:net'
import { mkdirSync, readFileSync, writeFileSync, rmSync, readdirSync } from 'node:fs'
import { dirname, join, posix, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { load } from 'cheerio'
import { zipSync } from 'fflate'

// Upgrade deliberately and rebuild/verify the snapshot when changing this version.
const pkgsiteVersion = 'v0.6.0'
const root = fileURLToPath(new URL('../', import.meta.url))
const backend = join(root, 'backend')
const output = join(root, 'docs/backend')
const html = join(output, 'html')
const tools = join(root, 'build/tools')
const binary = join(tools, process.platform === 'win32' ? 'pkgsite.exe' : 'pkgsite')
const escape = (s) => s.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;')
function go(args, env = {}) {
  const result = spawnSync('go', args, { cwd: backend, encoding: 'utf8', env: { ...process.env, ...env } })
  if (result.error) throw result.error
  if (result.status !== 0) throw new Error(result.stderr || result.stdout)
  return result.stdout.trim()
}
function save(path, data) {
  const target = resolve(html, path)
  if (!target.startsWith(resolve(html) + sep)) {
    throw new Error('Output path escapes documentation directory: ' + path)
  }
  mkdirSync(dirname(target), { recursive: true })
  writeFileSync(target, data)
}
const link = (from, to) => posix.relative(posix.dirname(from), to) || posix.basename(to)
const modulePath = go(['list', '-m'])
const packages = go(['list', './...']).split('\n')
mkdirSync(tools, { recursive: true })
console.log('Building official pkgsite ' + pkgsiteVersion)
go(['install', 'golang.org/x/pkgsite/cmd/pkgsite@' + pkgsiteVersion], { GOBIN: tools })
const socket = createServer()
socket.listen(0, '127.0.0.1')
await once(socket, 'listening')
const port = socket.address().port
await new Promise((done) => socket.close(done))
const origin = 'http://127.0.0.1:' + port
const server = spawn(binary, ['-http', '127.0.0.1:' + port, '-list=false', '.'], { cwd: backend, stdio: ['ignore', 'pipe', 'pipe'] })
let logs = ''
server.stdout.on('data', (data) => { logs += data })
server.stderr.on('data', (data) => { logs += data })
const stop = () => server.kill()
process.once('SIGINT', stop)
process.once('SIGTERM', stop)
const assets = new Set()
const pages = new Map()
const sources = new Map()
const pagePath = (path) => 'packages/' + (path === modulePath ? '' : path.slice(modulePath.length + 1) + '/') + 'index.html'
const normalize = (path) => path.replace(modulePath + '@v0.0.0', modulePath)
for (const pkg of packages) {
  let path = pkg
  while (path.length >= modulePath.length) {
    pages.set('/' + path, pagePath(path))
    if (path === modulePath) break
    path = path.slice(0, path.lastIndexOf('/'))
  }
}
// Generate local, line-addressable source pages; never depend on machine-specific /files URLs.
for (const pkg of packages) {
  const dir = pkg === modulePath ? '' : pkg.slice(modulePath.length + 1)
  const files = go(['list', '-f', '{{join .GoFiles "\\n"}}', pkg]).split('\n').filter(Boolean)
  for (const name of files) {
    const path = posix.join(dir, name)
    sources.set(path, 'source/' + path + '.html')
  }
}
function navigation(from) {
  return '<nav style="padding:1rem;background:#e7f5f8;color:#111"><a href="' + link(from, 'index.html') +
    '">Backend package index</a> · Static pkgsite snapshot · External dependency links require internet.</nav>'
}
async function get(path) {
  const response = await fetch(origin + path, { signal: AbortSignal.timeout(30000) })
  if (!response.ok) throw new Error('pkgsite returned ' + response.status + ' for ' + path)
  return response
}
try {
  const deadline = Date.now() + 120000
  while (true) {
    try {
      const response = await fetch(origin + '/' + modulePath, { signal: AbortSignal.timeout(2000) })
      if (response.ok && (await response.text()).includes(modulePath)) break
    } catch { }
    if (server.exitCode !== null || Date.now() > deadline) throw new Error('pkgsite did not become ready:\n' + logs)
    await new Promise((done) => setTimeout(done, 300))
  }
  // Delete only the generated HTML and archive under the fixed build/docs/backend directory.
  rmSync(html, { recursive: true, force: true })
  rmSync(join(output, 'backend-documentation.zip'), { force: true })
  mkdirSync(html, { recursive: true })
  for (const [url, file] of pages) {
    console.log('Exporting ' + url)
    const $ = load(await (await get(url)).text())
    if (packages.includes(url.slice(1)) && !$('.Documentation-content').length) {
      throw new Error('Missing rendered package documentation: ' + url)
    }
    // Server-backed search, version tabs and module scripts cannot run from file://.
    // Keep pkgsite's rendered documentation/CSS and native anchor navigation.
    $('script, form, select, button, dialog, .go-SearchForm, .js-canonicalURLPath, .js-playgroundVars').remove()
    $('body').prepend(navigation(file))
    $('head').append('<style>.UnitDirectories-internal,.UnitDirectories tr.hidden{display:table-row!important} .Documentation-content{display:block!important} :target{scroll-margin-top:2rem}</style>')
    $('[href], [src]').each((_, element) => {
      const node = $(element)
      const attr = node.attr('href') !== undefined ? 'href' : 'src'
      const raw = node.attr(attr)
      if (!raw || raw.startsWith('#') || /^(data:|mailto:|https?:\/\/)/.test(raw)) return
      const target = new URL(raw, origin + url)
      const path = normalize(decodeURI(target.pathname))
      if (path.startsWith('/static/') || path.startsWith('/third_party/')) {
        assets.add(path)
        node.attr(attr, link(file, path.slice(1)) + target.hash)
      } else if (path.startsWith('/files/')) {
        const marker = '/' + modulePath + '/'
        const source = path.slice(path.lastIndexOf(marker) + marker.length)
        if (sources.has(source)) node.attr(attr, link(file, sources.get(source)) + target.hash)
        else node.replaceWith(node.contents())
      } else if (pages.has(path) && !target.search) {
        node.attr(attr, link(file, pages.get(path)) + target.hash)
      } else if (path === '/') {
        node.attr(attr, link(file, 'index.html'))
      } else if (pages.has(path) && target.search) {
        node.replaceWith(node.contents())
      } else {
        node.attr(attr, 'https://pkg.go.dev' + path + target.search + target.hash)
      }
    })
    save(file, $.html())
  }
  // CSS can reference other CSS, fonts and images. Export the full reachable asset graph.
  for (const path of assets) {
    const response = await get(path)
    let content = Buffer.from(await response.arrayBuffer())
    if (path.endsWith('.css')) {
      let css = content.toString()
      css = css.replace(/url\(\s*(['"]?)([^'")]+)\1\s*\)/g, (match, quote, raw) => {
        if (/^(data:|https?:|\/\/|#)/.test(raw)) return match
        // Upstream header CSS has a legacy image URL not served by local pkgsite.
        if (raw === '/images/menu-24px.svg') raw = '/static/shared/icon/menu_gm_grey_24dp.svg'
        const target = new URL(raw, origin + path)
        assets.add(target.pathname)
        return 'url("' + link(path.slice(1), target.pathname.slice(1)) + target.hash + '")'
      })
      content = Buffer.from(css)
    }
    save(path.slice(1), content)
  }
  for (const [path, file] of sources) {
    const source = readFileSync(join(backend, path), 'utf8')
    save(file, '<!doctype html><html><head><meta charset="utf-8"><title>' + escape(path) +
      '</title><style>body{font:16px system-ui}pre{line-height:1.5}pre span{display:block}pre span:target{background:#fff3ac}</style></head><body>' +
      navigation(file) + '<h1>' + escape(path) + '</h1><pre>' +
      source.split('\n').map((line, i) => '<span id="L' + (i + 1) + '"><a href="#L' + (i + 1) + '">' +
        (i + 1) + '</a>  ' + escape(line) + '</span>').join('') + '</pre></body></html>')
  }
  save('index.html', '<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Public Transport backend documentation</title>' +
    '<style>body{max-width:70rem;margin:3rem auto;padding:1rem;font:18px/1.6 system-ui}a{color:#007d9c}</style></head><body><h1>Public Transport backend</h1>' +
    '<p>Generated from Go source comments using official pkgsite ' + pkgsiteVersion + '. Open any package below. Use your browser’s Find command to search a page.</p><ul>' +
    packages.map((pkg) => '<li><a href="' + pagePath(pkg) + '">' + escape(pkg) + '</a></li>').join('') +
    '</ul><p>Package documentation and source are available offline. Links to dependencies and Go websites require internet. Server search, version history and playground controls are omitted.</p>' +
    '<p>Rebuild from the repository root with <code>npm run docs:build</code>.</p></body></html>')
  const pkgsiteDir = JSON.parse(go(['mod', 'download', '-json', 'golang.org/x/pkgsite@' + pkgsiteVersion])).Dir
  save('PKGSITE-LICENSE', readFileSync(join(pkgsiteDir, 'LICENSE')))
  save('build-info.json', JSON.stringify({ pkgsiteVersion, module: modulePath, packages, go: go(['version']) }, null, 2) + '\n')
  // Fail the build on broken local HTML links, images, CSS references, or source anchors.
  const files = {}
  function collect(dir, prefix = '') {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = posix.join(prefix, entry.name)
      if (entry.isDirectory()) collect(join(dir, entry.name), path)
      else files[path] = readFileSync(join(dir, entry.name))
    }
  }
  collect(html)
  for (const [file, data] of Object.entries(files)) {
    if (!file.endsWith('.html')) continue
    const $ = load(data.toString())
    $('[href], [src]').each((_, element) => {
      for (const attr of ['href', 'src']) {
        const raw = $(element).attr(attr)
        if (!raw || /^(https?:|mailto:|data:|\/\/)/.test(raw)) continue
        const target = new URL(raw, 'https://snapshot.invalid/' + file)
        const path = decodeURI(target.pathname.slice(1))
        if (!files[path]) throw new Error('Broken local link: ' + file + ' -> ' + raw)
        if (target.hash && path.startsWith('source/')) {
          const source = load(files[path].toString())
          if (!source('[id]').toArray().some((el) => source(el).attr('id') === target.hash.slice(1))) {
            throw new Error('Missing source anchor: ' + raw)
          }
        }
      }
    })
  }
  const zip = Object.fromEntries(Object.entries(files).map(([path, data]) => ['html/' + path, data]))
  writeFileSync(join(output, 'backend-documentation.zip'), zipSync(zip))
  console.log('Verified ' + pages.size + ' documentation pages; created ' + join(output, 'backend-documentation.zip'))
} finally {
  stop()
  process.removeListener('SIGINT', stop)
  process.removeListener('SIGTERM', stop)
}
