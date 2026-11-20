$repo = Split-Path -Parent $PSScriptRoot
node (Join-Path $PSScriptRoot "test-all.mjs") @args
exit $LASTEXITCODE
