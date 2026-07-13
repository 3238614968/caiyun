param(
  [string]$ProjectName = $(if ($env:COMPOSE_PROJECT_NAME) { $env:COMPOSE_PROJECT_NAME } else { 'caiyun-e2e' }),
  [switch]$KeepStack
)

$ErrorActionPreference = 'Stop'
$RootDir = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$LocalDir = Join-Path $RootDir '.local'
$EnvFile = Join-Path $LocalDir 'e2e.env'

New-Item -ItemType Directory -Force $LocalDir | Out-Null

if (-not (Test-Path $EnvFile)) {
@'
MYSQL_ROOT_PASSWORD=caiyun_root_e2e_change_me
MYSQL_USER=caiyun_app
MYSQL_PASSWORD=caiyun_app_e2e_change_me
REDIS_PASSWORD=caiyun_redis_e2e_change_me
JWT_SECRET=0123456789abcdef0123456789abcdef
DATA_ENCRYPTION_KEYS=v1=0123456789abcdef0123456789abcdef
DATA_ENCRYPTION_CURRENT_VERSION=v1
TRUSTED_PROXIES=none
WORKER_MONITOR_TOKEN=caiyun_worker_e2e_token
GRAFANA_ADMIN_PASSWORD=caiyun_grafana_e2e_change_me
TASK_QUEUE_BACKEND=streams
ALLOWED_ORIGINS=http://frontend:8080,http://localhost,http://127.0.0.1
'@ | Set-Content -Encoding UTF8 $EnvFile
}

$composeArgs = @(
  '--project-name', $ProjectName,
  '--env-file', $EnvFile,
  '-f', (Join-Path $RootDir 'docker-compose.yml'),
  '-f', (Join-Path $RootDir 'docker-compose.e2e.yml')
)

try {
  docker compose @composeArgs up --build --abort-on-container-exit --exit-code-from e2e-runner e2e-runner
} finally {
  if (-not $KeepStack) {
    docker compose @composeArgs down -v --remove-orphans
  }
}
