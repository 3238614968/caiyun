[CmdletBinding()]
param(
    [switch]$SkipInstall,
    [switch]$WithRedisIntegration,
    [string]$RedisAddr = "127.0.0.1:6379",
    [int]$RedisDB = 15,
    [switch]$SkipE2E,
    [switch]$WithAudit
)

$ErrorActionPreference = "Stop"
$root = Resolve-Path (Join-Path $PSScriptRoot "..")

function Invoke-Step {
    param(
        [string]$Name,
        [scriptblock]$Script
    )
    Write-Host "`n==> $Name" -ForegroundColor Cyan
    & $Script
}

function Use-Location {
    param(
        [string]$Path,
        [scriptblock]$Script
    )
    Push-Location $Path
    try {
        & $Script
    }
    finally {
        Pop-Location
    }
}

$backend = Join-Path $root "backend"
$frontend = Join-Path $root "frontend"

# Local developer machines may not have the exact Go version from go.mod installed.
# Keep CI strict with setup-go, but allow local toolchain auto-selection here.
$env:GOTOOLCHAIN = "auto"

Invoke-Step "Backend unit tests" {
    Use-Location $backend { go test ./... }
}

Invoke-Step "Backend coverage build" {
    Use-Location $backend { go test ./... -cover }
}

Invoke-Step "Backend vet" {
    Use-Location $backend { go vet ./... }
}

if ($WithRedisIntegration) {
    Invoke-Step "Backend Redis queue integration" {
        Use-Location $backend {
            $env:CAIYUN_REDIS_INTEGRATION = "1"
            $env:CAIYUN_TEST_REDIS_ADDR = $RedisAddr
            $env:CAIYUN_TEST_REDIS_DB = [string]$RedisDB
            go test ./internal/queue -run RedisIntegration -count=1
        }
    }
}

if (-not $SkipInstall) {
    Invoke-Step "Frontend npm ci" {
        Use-Location $frontend { npm ci }
    }
}

Invoke-Step "Frontend typecheck" {
    Use-Location $frontend { npm run typecheck }
}

Invoke-Step "Frontend lint" {
    Use-Location $frontend { npm run lint -- --quiet }
}

Invoke-Step "Frontend unit tests" {
    Use-Location $frontend { npm run test:unit }
}

Invoke-Step "Frontend build" {
    Use-Location $frontend { npm run build }
}

Invoke-Step "Frontend bundle budget" {
    Use-Location $frontend { npm run bundle:budget }
}

if (-not $SkipE2E) {
    Invoke-Step "Frontend E2E" {
        Use-Location $frontend { npm run e2e }
    }
}

if ($WithAudit) {
    Invoke-Step "Frontend npm audit" {
        Use-Location $frontend { npm audit --audit-level=high }
    }
}
else {
    Write-Host "`n==> Frontend npm audit skipped. Pass -WithAudit to send dependency metadata to the npm registry." -ForegroundColor Yellow
}

Write-Host "`nAll selected checks passed." -ForegroundColor Green
