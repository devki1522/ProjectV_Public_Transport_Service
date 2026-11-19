$repo = Split-Path -Parent $PSScriptRoot

$report = [IO.Path]::GetTempFileName()
try {
    Write-Host 'Running frontend tests'
    npm --silent --prefix (Join-Path $repo frontend) test -- --reporter=verbose --reporter=json "--outputFile.json=$report"
    $frontendExit = $LASTEXITCODE
    $frontend = Get-Content -LiteralPath $report -Raw | ConvertFrom-Json
} finally {
    Remove-Item -LiteralPath $report
}

Write-Host 'Running backend tests'
$backend = @(go -C (Join-Path $repo backend) test -json -v -count=1 ./... | ForEach-Object {
    $event = ConvertFrom-Json -InputObject $_
    if ($event.Output) {
        Write-Host $event.Output.TrimEnd("`r", "`n")
    }
    if ($event.Test -and $event.Action -in 'pass', 'fail', 'skip') {
        $event
    }
})
$backendExit = $LASTEXITCODE

Write-Output ''
Write-Output 'Frontend:'
if ($null -ne $frontend.numPassedTests) {
    Write-Output "  Passed: $($frontend.numPassedTests)  Failed: $($frontend.numFailedTests)"
} else {
    Write-Output "  Test runner did not produce counts (exit $frontendExit)."
}

Write-Output 'Backend:'
if ($backend.Count -gt 0) {
    $passed = @($backend | Where-Object Action -eq pass).Count
    $failed = @($backend | Where-Object Action -eq fail).Count
    Write-Output "  Passed: $passed  Failed: $failed"
    if ($backendExit -ne 0 -and $failed -eq 0) {
        Write-Output "  Test runner exited with code $backendExit."
    }
} else {
    Write-Output "  Test runner did not produce counts (exit $backendExit)."
}

if ($frontendExit -ne 0 -or $backendExit -ne 0 -or $null -eq $frontend.numPassedTests -or $backend.Count -eq 0) { exit 1 }
