#!/usr/bin/env node
import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const rootDir = fileURLToPath(new URL('..', import.meta.url))
const backendDir = path.join(rootDir, 'backend')
const frontendDir = path.join(rootDir, 'frontend')

// ANSI formatting helpers
const c = {
  reset: '\x1b[0m',
  bold: '\x1b[1m',
  dim: '\x1b[2m',
  green: '\x1b[32m',
  red: '\x1b[31m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  cyan: '\x1b[36m',
  gray: '\x1b[90m',
  bgGreen: '\x1b[42m\x1b[30m',
  bgRed: '\x1b[41m\x1b[37m',
}

function hr(char = '─', len = 70) {
  return c.gray + char.repeat(len) + c.reset
}

function printHeader(title) {
  console.log('\n' + hr('═'))
  console.log(` ${c.bold}${c.cyan}${title}${c.reset}`)
  console.log(hr('═'))
}

// 1. Run Go Backend Tests
async function runBackendTests() {
  printHeader('1. Backend Tests (Go REST API & Services)')

  return new Promise((resolve) => {
    const startTime = Date.now()
    const proc = spawn('go', ['test', '-json', '-count=1', './...'], {
      cwd: backendDir,
      env: { ...process.env, CGO_ENABLED: '0' },
    })

    const packageResults = new Map()
    let totalTests = 0
    let passedTests = 0
    let failedTests = 0
    const failures = []
    let buffer = ''

    proc.stdout.on('data', (chunk) => {
      buffer += chunk.toString()
      const lines = buffer.split('\n')
      buffer = lines.pop() // keep last incomplete line

      for (const line of lines) {
        if (!line.trim()) continue
        try {
          const ev = JSON.parse(line)
          const pkg = ev.Package ? ev.Package.replace(/^.*\/backend\//, '') : ''

          if (!packageResults.has(pkg) && pkg) {
            packageResults.set(pkg, { passed: 0, failed: 0, duration: 0 })
          }

          if (ev.Test && ev.Action === 'pass') {
            passedTests++
            totalTests++
            if (packageResults.has(pkg)) packageResults.get(pkg).passed++
          } else if (ev.Test && ev.Action === 'fail') {
            failedTests++
            totalTests++
            if (packageResults.has(pkg)) packageResults.get(pkg).failed++
            failures.push({ pkg, test: ev.Test })
          } else if (!ev.Test && (ev.Action === 'pass' || ev.Action === 'fail')) {
            if (packageResults.has(pkg)) {
              packageResults.get(pkg).duration = ev.Elapsed || 0
            }
          }
        } catch {
          // ignore non-json
        }
      }
    })

    proc.stderr.on('data', (chunk) => {
      process.stderr.write(chunk)
    })

    proc.on('close', (code) => {
      const duration = ((Date.now() - startTime) / 1000).toFixed(2)

      // Print per-package summary
      for (const [pkg, stats] of packageResults.entries()) {
        if (stats.passed === 0 && stats.failed === 0) continue
        const status = stats.failed === 0
          ? `${c.green}✔ PASS${c.reset}`
          : `${c.red}✖ FAIL${c.reset}`
        const timeStr = `${c.gray}(${stats.duration.toFixed(2)}s)${c.reset}`
        const countStr = `${stats.passed} passed` + (stats.failed > 0 ? `, ${c.red}${stats.failed} failed${c.reset}` : '')
        console.log(`  ${status}  ${c.bold}${pkg.padEnd(28)}${c.reset} ${countStr.padEnd(22)} ${timeStr}`)
      }

      if (failures.length > 0) {
        console.log(`\n  ${c.red}${c.bold}Failures:${c.reset}`)
        for (const f of failures) {
          console.log(`    ${c.red}✖ ${f.pkg} ❯ ${f.test}${c.reset}`)
        }
      }

      const success = code === 0 && failedTests === 0
      console.log(`\n  ${c.bold}Backend Result:${c.reset} ${success ? c.green + 'ALL TESTS PASSED' : c.red + 'SOME TESTS FAILED'} ${c.reset} ` +
        `${c.gray}[${passedTests}/${totalTests} tests passed in ${duration}s]${c.reset}`)

      resolve({
        name: 'Backend (Go)',
        passed: passedTests,
        failed: failedTests,
        total: totalTests,
        duration: `${duration}s`,
        success,
      })
    })
  })
}

// 2. Run Frontend Tests
async function runFrontendTests() {
  printHeader('2. Frontend Tests (Vitest & React Components)')

  return new Promise((resolve) => {
    const startTime = Date.now()
    const isWindows = process.platform === 'win32'
    const npmCmd = isWindows ? 'npm.cmd' : 'npm'

    const proc = spawn(npmCmd, ['test', '--prefix', 'frontend'], {
      cwd: rootDir,
      shell: isWindows,
      stdio: ['ignore', 'pipe', 'pipe'],
      env: { ...process.env, CI: 'true', FORCE_COLOR: '1' },
    })

    let stdout = ''
    let stderr = ''

    proc.stdout.on('data', (d) => {
      const str = d.toString()
      stdout += str
      // Stream passing suite lines directly for responsive output
      const lines = str.split('\n')
      for (const line of lines) {
        if (line.includes('✓') || line.includes('PASS') || line.includes('FAIL') || line.includes('❯')) {
          console.log('  ' + line.trim())
        }
      }
    })

    proc.stderr.on('data', (d) => {
      stderr += d.toString()
    })

    proc.on('close', (code) => {
      const duration = ((Date.now() - startTime) / 1000).toFixed(2)

      // Strip ANSI codes for regex matching
      const cleanStdout = stdout.replace(/\x1b\[[0-9;]*[a-zA-Z]/g, '')

      let passed = 0
      let failed = 0
      let total = 0

      const matchTests = cleanStdout.match(/Tests\s+(?:(\d+)\s+failed\s*\|\s*)?(\d+)\s+passed\s*\((\d+)\)/)
      if (matchTests) {
        failed = matchTests[1] ? parseInt(matchTests[1], 10) : 0
        passed = parseInt(matchTests[2], 10)
        total = parseInt(matchTests[3], 10)
      } else {
        const simplePass = cleanStdout.match(/Tests\s+.*(\d+)\s+passed/)
        if (simplePass) {
          passed = parseInt(simplePass[1], 10)
          total = passed
        }
      }

      const success = code === 0 && failed === 0
      console.log(`\n  ${c.bold}Frontend Result:${c.reset} ${success ? c.green + 'ALL TESTS PASSED' : c.red + 'SOME TESTS FAILED'} ${c.reset} ` +
        `${c.gray}[${passed}/${total || passed} tests passed in ${duration}s]${c.reset}`)

      resolve({
        name: 'Frontend (Vitest)',
        passed,
        failed,
        total: total || passed,
        duration: `${duration}s`,
        success,
      })
    })
  })
}

// 3. Run Node Scripts Tests
async function runScriptTests() {
  printHeader('3. Integration & Auth Setup Tests (Node:Test)')

  return new Promise((resolve) => {
    const startTime = Date.now()
    const proc = spawn('node', ['--test', '--test-reporter=spec', 'scripts/setup-auth.test.mjs'], {
      cwd: rootDir,
      stdio: ['ignore', 'pipe', 'pipe'],
      env: { ...process.env, FORCE_COLOR: '1' },
    })

    let stdout = ''
    proc.stdout.on('data', (d) => {
      const str = d.toString()
      stdout += str
      for (const line of str.split('\n')) {
        if (line.trim()) {
          console.log('  ' + line)
        }
      }
    })

    proc.stderr.on('data', (d) => {
      process.stderr.write(d)
    })

    proc.on('close', (code) => {
      const duration = ((Date.now() - startTime) / 1000).toFixed(2)

      let passed = 0
      let failed = 0
      let total = 0

      const passMatch = stdout.match(/ℹ pass (\d+)/)
      const failMatch = stdout.match(/ℹ fail (\d+)/)
      const totalMatch = stdout.match(/ℹ tests (\d+)/)

      if (passMatch) passed = parseInt(passMatch[1], 10)
      if (failMatch) failed = parseInt(failMatch[1], 10)
      if (totalMatch) total = parseInt(totalMatch[1], 10)

      const success = code === 0 && failed === 0
      console.log(`\n  ${c.bold}Scripts Result:${c.reset} ${success ? c.green + 'ALL TESTS PASSED' : c.red + 'SOME TESTS FAILED'} ${c.reset} ` +
        `${c.gray}[${passed}/${total || passed} tests passed in ${duration}s]${c.reset}`)

      resolve({
        name: 'Scripts (Node:Test)',
        passed,
        failed,
        total: total || passed,
        duration: `${duration}s`,
        success,
      })
    })
  })
}

// Main execution
async function main() {
  const args = process.argv.slice(2)
  const runOnlyBackend = args.includes('--backend') || args.includes('-b')
  const runOnlyFrontend = args.includes('--frontend') || args.includes('-f')
  const runOnlyScripts = args.includes('--scripts') || args.includes('-s')

  console.log(`${c.bold}${c.cyan}======================================================================${c.reset}`)
  console.log(`${c.bold}            PROJECT V: UNIFIED TEST SUITE RUNNER                      ${c.reset}`)
  console.log(`${c.bold}${c.cyan}======================================================================${c.reset}`)

  const suites = []
  const overallStart = Date.now()

  if (runOnlyBackend) {
    suites.push(await runBackendTests())
  } else if (runOnlyFrontend) {
    suites.push(await runFrontendTests())
  } else if (runOnlyScripts) {
    suites.push(await runScriptTests())
  } else {
    suites.push(await runBackendTests())
    suites.push(await runFrontendTests())
    suites.push(await runScriptTests())
  }

  const overallDuration = ((Date.now() - overallStart) / 1000).toFixed(2)

  // Overall Scorecard Table
  console.log('\n' + hr('═'))
  console.log(` ${c.bold}                     TEST EXECUTION SCORECARD                         ${c.reset}`)
  console.log(hr('═'))
  console.log(`  ${c.bold}${'Suite'.padEnd(24)} ${'Passed'.padEnd(10)} ${'Failed'.padEnd(10)} ${'Time'.padEnd(10)} ${'Status'}${c.reset}`)
  console.log(hr('─'))

  let totalPassed = 0
  let totalFailed = 0
  let totalTests = 0
  let allSuccess = true

  for (const s of suites) {
    totalPassed += s.passed
    totalFailed += s.failed
    totalTests += s.total
    if (!s.success) allSuccess = false

    const statusBadge = s.success
      ? `${c.green}✔ PASS${c.reset}`
      : `${c.red}✖ FAIL${c.reset}`
    console.log(`  ${c.cyan}${s.name.padEnd(24)}${c.reset} ${String(s.passed).padEnd(10)} ${String(s.failed).padEnd(10)} ${s.duration.padEnd(10)} ${statusBadge}`)
  }

  console.log(hr('─'))
  const totalStatusBadge = allSuccess
    ? ` ${c.bgGreen}${c.bold} ALL SUITES PASSED ${c.reset} `
    : ` ${c.bgRed}${c.bold} SOME SUITES FAILED ${c.reset} `

  console.log(`  ${c.bold}${'TOTAL'.padEnd(24)} ${String(totalPassed).padEnd(10)} ${String(totalFailed).padEnd(10)} ${overallDuration + 's'}${c.reset.padEnd(4)} ${totalStatusBadge}`)
  console.log(hr('═'))
  console.log(`  Total test assertions executed: ${c.bold}${c.green}${totalPassed}${c.reset} passed, ${totalFailed > 0 ? c.red : c.green}${totalFailed}${c.reset} failed (${totalTests} total)\n`)

  if (!allSuccess) {
    process.exit(1)
  }
}

main().catch((err) => {
  console.error('\nTest runner encountered an error:', err)
  process.exit(1)
})
