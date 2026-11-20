$ErrorActionPreference = 'Stop'
node (Join-Path $PSScriptRoot "scripts\test-all.mjs") @args
exit $LASTEXITCODE
