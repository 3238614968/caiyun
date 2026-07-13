[CmdletBinding()]
param(
    [string]$Version = "dev",
    [string]$Commit = "unknown",
    [string]$BuildTime = "",
    [string]$OutputDir = "backend"
)

$ErrorActionPreference = "Stop"
$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$backend = Join-Path $root "backend"
$out = Join-Path $root $OutputDir
New-Item -ItemType Directory -Force -Path $out | Out-Null

if (-not $BuildTime) {
    $BuildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
}
try {
    if ($Version -eq "dev") {
        $gitVersion = git -C $root describe --tags --always --dirty 2>$null
        if ($gitVersion) { $Version = $gitVersion.Trim() }
    }
    if ($Commit -eq "unknown") {
        $gitCommit = git -C $root rev-parse --short HEAD 2>$null
        if ($gitCommit) { $Commit = $gitCommit.Trim() }
    }
} catch {}

$ldflags = "-s -w -X caiyun/internal/version.Version=$Version -X caiyun/internal/version.Commit=$Commit -X caiyun/internal/version.BuildTime=$BuildTime"
$binary = Join-Path $out "caiyun-linux"
$checksumPath = Join-Path $out "SHA256SUMS"

Push-Location $backend
try {
    $env:GOTOOLCHAIN = "auto"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"
    & go build -trimpath -ldflags $ldflags -o $binary ./cmd/caiyun
    if ($LASTEXITCODE -ne 0) { throw "go build caiyun-linux failed with exit code $LASTEXITCODE" }

    $item = Get-Item -LiteralPath $binary
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $item.FullName).Hash.ToLowerInvariant()
    Set-Content -Encoding ASCII -LiteralPath $checksumPath -Value "$hash  $($item.Name)"
}
finally {
    Pop-Location
}

Get-Item -LiteralPath $binary, $checksumPath | Select-Object FullName,Length,LastWriteTime
