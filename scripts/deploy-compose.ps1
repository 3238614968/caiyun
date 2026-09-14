param(
    [switch]$Production,
    [switch]$Local
)

$bash = Get-Command bash -ErrorAction SilentlyContinue
if (-not $bash) {
    throw "bash is required. Install Git for Windows or run scripts/deploy-compose.sh on Linux/macOS."
}

$arguments = @("scripts/deploy-compose.sh")
if ($Production) {
    $arguments += "--production"
} elseif ($Local) {
    $arguments += "--local"
}

Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    & $bash.Source @arguments
    exit $LASTEXITCODE
} finally {
    Pop-Location
}
