[CmdletBinding()]
param(
    [string]$Version = "ci",
    [string]$ReleaseDir = ""
)

$ErrorActionPreference = "Stop"
$root = Resolve-Path (Join-Path $PSScriptRoot "..")
if (-not $ReleaseDir) {
    $ReleaseDir = Join-Path (Join-Path $root "release") ("{0}-linux-amd64" -f $Version)
} elseif (-not [System.IO.Path]::IsPathRooted($ReleaseDir)) {
    $ReleaseDir = Join-Path $root $ReleaseDir
}
$ReleaseDir = (Resolve-Path $ReleaseDir).Path

function Assert-True {
    param(
        [Parameter(Mandatory = $true)][bool]$Condition,
        [Parameter(Mandatory = $true)][string]$Message
    )
    if (-not $Condition) {
        throw $Message
    }
}

function Assert-Exists {
    param([Parameter(Mandatory = $true)][string]$Path)
    Assert-True (Test-Path $Path) "Expected path does not exist: $Path"
}

function Assert-Checksum {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][string[]]$ChecksumLines
    )
    $name = [System.IO.Path]::GetFileName($FilePath)
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $FilePath).Hash.ToLowerInvariant()
    $expected = "{0} *{1}" -f $hash, $name
    Assert-True ($ChecksumLines -contains $expected) "Checksum entry mismatch for $name"
}

function Assert-ElfAmd64 {
    param([Parameter(Mandatory = $true)][string]$FilePath)
    $bytes = [System.IO.File]::ReadAllBytes($FilePath)
    Assert-True ($bytes.Length -ge 20) "$FilePath is too short to be an ELF file"
    Assert-True ($bytes[0] -eq 0x7f -and $bytes[1] -eq 0x45 -and $bytes[2] -eq 0x4c -and $bytes[3] -eq 0x46) "$FilePath does not start with ELF magic"
    Assert-True ($bytes[4] -eq 2) "$FilePath is not a 64-bit ELF"
    $machine = [BitConverter]::ToUInt16($bytes, 18)
    Assert-True ($machine -eq 62) "$FilePath is not amd64 ELF (machine=$machine)"
}

function Get-TarEntries {
    param([Parameter(Mandatory = $true)][string]$ArchivePath)
    $tarCmd = Get-Command tar -ErrorAction SilentlyContinue
    Assert-True ($null -ne $tarCmd) "tar command not found"
    $entries = & $tarCmd.Source -tf $ArchivePath
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to inspect archive: $ArchivePath"
    }
    return @($entries)
}

function Resolve-ReleaseArtifactName {
    param(
        [Parameter(Mandatory = $true)][string]$Prefix,
        [Parameter(Mandatory = $true)][string]$Suffix
    )
    $preferred = "{0}-{1}{2}" -f $Prefix, $Version, $Suffix
    if (Test-Path (Join-Path $ReleaseDir $preferred)) {
        return $preferred
    }

    $matches = Get-ChildItem -LiteralPath $ReleaseDir -File |
        Where-Object { $_.Name -like "$Prefix-*${Suffix}" } |
        Sort-Object Name
    Assert-True ($matches.Count -eq 1) "Unable to uniquely resolve artifact for prefix=$Prefix suffix=$Suffix under $ReleaseDir"
    return $matches[0].Name
}

$frontendArchive = Resolve-ReleaseArtifactName -Prefix 'caiyun-frontend' -Suffix '.tar.gz'
$migrationsArchive = Resolve-ReleaseArtifactName -Prefix 'caiyun-migrations' -Suffix '.tar.gz'
$monitoringArchive = Resolve-ReleaseArtifactName -Prefix 'caiyun-monitoring' -Suffix '.tar.gz'
$calendarArchive = Resolve-ReleaseArtifactName -Prefix 'caiyun-calendar' -Suffix '.tar.gz'
$sbomFile = Resolve-ReleaseArtifactName -Prefix 'caiyun-sbom' -Suffix '.json'

$expectedFiles = @(
    'caiyun-linux',
    $frontendArchive,
    $migrationsArchive,
    $monitoringArchive,
    $calendarArchive,
    $sbomFile,
    'deploy-linux.sh',
    'rollback-linux.sh',
    'health-check.sh',
    'import-calendar.sh',
    'archive-history.sh',
    'rotate-encryption.sh',
    'nginx-server.conf',
    'SHA256SUMS'
)

foreach ($name in $expectedFiles) {
    Assert-Exists (Join-Path $ReleaseDir $name)
}

$checksumPath = Join-Path $ReleaseDir 'SHA256SUMS'
$checksumLines = Get-Content -LiteralPath $checksumPath
foreach ($name in $expectedFiles | Where-Object { $_ -ne 'SHA256SUMS' }) {
    Assert-Checksum -FilePath (Join-Path $ReleaseDir $name) -ChecksumLines $checksumLines
}

Assert-ElfAmd64 (Join-Path $ReleaseDir 'caiyun-linux')

$frontendEntries = Get-TarEntries (Join-Path $ReleaseDir $frontendArchive)
Assert-True ($frontendEntries -contains './index.html') 'frontend archive missing ./index.html'
Assert-True (($frontendEntries | Where-Object { $_ -like './assets/*' }).Count -gt 0) 'frontend archive missing assets/*'

$migrationEntries = Get-TarEntries (Join-Path $ReleaseDir $migrationsArchive)
Assert-True (($migrationEntries | Where-Object { $_ -like 'migrations/*' }).Count -gt 0) 'migrations archive missing migrations/*'

$monitoringEntries = Get-TarEntries (Join-Path $ReleaseDir $monitoringArchive)
Assert-True (($monitoringEntries | Where-Object { $_ -like 'monitoring/*' }).Count -gt 0) 'monitoring archive missing monitoring/*'

$calendarEntries = Get-TarEntries (Join-Path $ReleaseDir $calendarArchive)
Assert-True (($calendarEntries | Where-Object { $_ -like 'calendar/*' }).Count -gt 0) 'calendar archive missing calendar/*'

$sbomRaw = Get-Content -LiteralPath (Join-Path $ReleaseDir $sbomFile) -Raw | ConvertFrom-Json
Assert-True ($sbomRaw.bomFormat -eq 'CycloneDX') 'SBOM bomFormat must be CycloneDX'
Assert-True ($sbomRaw.components.Count -ge 12) 'SBOM components unexpectedly small'

Write-Host "release smoke test passed: $ReleaseDir"
