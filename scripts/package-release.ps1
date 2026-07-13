[CmdletBinding()]
param(
    [string]$Version = "",
    [string]$OutputDir = "",
    [string]$BuildId = ""
)

$ErrorActionPreference = "Stop"
$root = Resolve-Path (Join-Path $PSScriptRoot "..")

if (-not $Version) {
    try {
        $Version = (git -C $root describe --tags --always --dirty 2>$null).Trim()
    } catch {}
}
if (-not $Version) {
    $Version = "dev"
}
if (-not $BuildId -and -not [string]::IsNullOrWhiteSpace($env:BUILD_ID)) {
    $BuildId = $env:BUILD_ID.Trim()
}
$releaseSuffix = if ($BuildId) { "{0}-{1}" -f $Version, $BuildId } else { $Version }

if (-not $OutputDir) {
    $OutputDir = Join-Path (Join-Path $root "release") ("{0}-linux-amd64" -f $releaseSuffix)
} elseif (-not [System.IO.Path]::IsPathRooted($OutputDir)) {
    $OutputDir = Join-Path $root $OutputDir
}

$parentDir = Split-Path -Parent $OutputDir
if (-not [string]::IsNullOrWhiteSpace($parentDir)) {
    New-Item -ItemType Directory -Force -Path $parentDir | Out-Null
}

$caiyunBin = Join-Path $root "backend\caiyun-linux"
$frontendDist = Join-Path $root "frontend\dist"
$nginxConf = Join-Path $root "nginx-server.conf"
$migrationsDir = Join-Path $root "backend\migrations"
$monitoringDir = Join-Path $root "deploy\monitoring"
$calendarDir = Join-Path $root "deploy\calendar"

foreach ($path in @($caiyunBin, $frontendDist, $nginxConf, $migrationsDir, $monitoringDir, $calendarDir)) {
    if (-not (Test-Path $path)) {
        throw "Missing required path: $path"
    }
}

$tarCmd = Get-Command tar -ErrorAction SilentlyContinue
if (-not $tarCmd) {
    throw "tar command not found; Windows 10/11 built-in tar.exe is required"
}

function New-TarGz {
    param(
        [Parameter(Mandatory = $true)][string]$ArchivePath,
        [Parameter(Mandatory = $true)][string]$SourceDir
    )
    if (Test-Path $ArchivePath) {
        Remove-Item -LiteralPath $ArchivePath -Force
    }
    & $tarCmd.Source -czf $ArchivePath -C $SourceDir .
    if ($LASTEXITCODE -ne 0) {
        throw "tar failed for $ArchivePath"
    }
}

$stageDir = Join-Path ([System.IO.Path]::GetTempPath()) ("caiyun-release-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $stageDir | Out-Null
$finalized = $false

try {
    Copy-Item -LiteralPath $caiyunBin -Destination (Join-Path $stageDir "caiyun-linux") -Force
    Copy-Item -LiteralPath $nginxConf -Destination (Join-Path $stageDir "nginx-server.conf") -Force

    New-TarGz -ArchivePath (Join-Path $stageDir ("caiyun-frontend-{0}.tar.gz" -f $Version)) -SourceDir $frontendDist

    $archive = Join-Path $stageDir ("caiyun-migrations-{0}.tar.gz" -f $Version)
    & $tarCmd.Source -czf $archive -C (Join-Path $root "backend") "migrations"
    if ($LASTEXITCODE -ne 0) { throw "tar failed for $archive" }

    $archive = Join-Path $stageDir ("caiyun-monitoring-{0}.tar.gz" -f $Version)
    & $tarCmd.Source -czf $archive -C (Join-Path $root "deploy") "monitoring"
    if ($LASTEXITCODE -ne 0) { throw "tar failed for $archive" }

    $archive = Join-Path $stageDir ("caiyun-calendar-{0}.tar.gz" -f $Version)
    & $tarCmd.Source -czf $archive -C (Join-Path $root "deploy") "calendar"
    if ($LASTEXITCODE -ne 0) { throw "tar failed for $archive" }

    $copyFiles = @(
        "scripts\deploy-linux.sh",
        "scripts\rollback-linux.sh",
        "scripts\health-check.sh",
        "scripts\import-calendar.sh",
        "scripts\archive-history.sh",
        "scripts\rotate-encryption.sh"
    )
    foreach ($relative in $copyFiles) {
        $source = Join-Path $root $relative
        if (-not (Test-Path $source)) {
            throw "Missing file: $source"
        }
        Copy-Item -LiteralPath $source -Destination (Join-Path $stageDir ([System.IO.Path]::GetFileName($source))) -Force
    }

    $sbomPath = Join-Path $stageDir ("caiyun-sbom-{0}.json" -f $Version)
    $sbomItems = Get-ChildItem -LiteralPath $stageDir -File |
        Where-Object { $_.Name -ne "SHA256SUMS" -and $_.Name -ne "SHA256SUMS.sig" -and -not $_.Name.EndsWith('.sig') } |
        Sort-Object Name
    $components = foreach ($item in $sbomItems) {
        $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $item.FullName).Hash.ToLowerInvariant()
        @{
            type = "file"
            name = $item.Name
            hashes = @(@{ alg = "SHA-256"; content = $hash })
            properties = @(@{ name = "size"; value = [string]$item.Length })
        }
    }
    $sbom = @{
        bomFormat = "CycloneDX"
        specVersion = "1.5"
        version = 1
        metadata = @{
            timestamp = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
            tools = @(@{ vendor = "caiyun"; name = "package-release.ps1"; version = "fallback" })
            component = @{ type = "application"; name = "caiyun" }
        }
        components = $components
    }
    $sbom | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $sbomPath -Encoding UTF8

    $checksumPath = Join-Path $stageDir "SHA256SUMS"
    $checksumItems = Get-ChildItem -LiteralPath $stageDir -File |
        Where-Object { $_.Name -ne "SHA256SUMS" -and $_.Name -ne "SHA256SUMS.sig" -and -not $_.Name.EndsWith('.sig') } |
        Sort-Object Name
    $checksumLines = foreach ($item in $checksumItems) {
        $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $item.FullName).Hash.ToLowerInvariant()
        "{0} *{1}" -f $hash, $item.Name
    }
    Set-Content -LiteralPath $checksumPath -Encoding ASCII -Value $checksumLines

    $signKey = $env:RELEASE_SIGN_KEY
    $signKeyB64 = $env:RELEASE_SIGN_KEY_B64
    if ($signKey -or $signKeyB64) {
        $openssl = Get-Command openssl -ErrorAction SilentlyContinue
        if ($openssl) {
            $tmpKey = $null
            try {
                $keyPath = $signKey
                if ($signKeyB64) {
                    $tmpKey = Join-Path $env:TEMP ([System.Guid]::NewGuid().ToString("N") + ".pem")
                    [IO.File]::WriteAllBytes($tmpKey, [Convert]::FromBase64String($signKeyB64))
                    $keyPath = $tmpKey
                }
                & $openssl.Source dgst -sha256 -sign $keyPath -out (Join-Path $stageDir "SHA256SUMS.sig") $checksumPath
                if ($LASTEXITCODE -ne 0) {
                    throw "openssl sign failed"
                }
            }
            finally {
                if ($tmpKey -and (Test-Path $tmpKey)) {
                    Remove-Item -LiteralPath $tmpKey -Force
                }
            }
        }
        else {
            Write-Warning "RELEASE_SIGN_KEY/RELEASE_SIGN_KEY_B64 已设置，但本机未找到 openssl，已跳过签名"
        }
    }

    if (Test-Path $OutputDir) {
        try {
            Remove-Item -LiteralPath $OutputDir -Recurse -Force
        }
        catch {
            throw "无法清理已存在的发布目录 $OutputDir。请改用 -BuildId 或 -OutputDir 生成全新目录，或关闭占用该目录文件的进程。原始错误: $($_.Exception.Message)"
        }
    }

    Move-Item -LiteralPath $stageDir -Destination $OutputDir
    $finalized = $true

    Get-ChildItem -LiteralPath $OutputDir | Select-Object Name, Length, LastWriteTime
}
finally {
    if (-not $finalized -and (Test-Path $stageDir)) {
        Remove-Item -LiteralPath $stageDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
