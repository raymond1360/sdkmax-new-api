param(
  [string]$EnvFile = ".env"
)

$ErrorActionPreference = "Stop"

function Read-DotEnv {
  param([string]$Path)
  if (!(Test-Path $Path)) {
    throw "Missing $Path. Copy .env.sdkmax.example to .env and fill deployment settings."
  }

  Get-Content $Path | ForEach-Object {
    $line = $_.Trim()
    if ($line -eq "" -or $line.StartsWith("#")) { return }
    $idx = $line.IndexOf("=")
    if ($idx -lt 1) { return }
    $name = $line.Substring(0, $idx).Trim()
    $value = $line.Substring($idx + 1).Trim().Trim('"').Trim("'")
    [Environment]::SetEnvironmentVariable($name, $value, "Process")
  }
}

function Require-Env {
  param([string]$Name)
  $value = [Environment]::GetEnvironmentVariable($Name, "Process")
  if ([string]::IsNullOrWhiteSpace($value)) {
    throw "Missing required env var: $Name"
  }
  return $value
}

function Get-GitRemoteUrl {
  param([string]$Name)
  $url = git remote get-url $Name 2>$null
  if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($url)) {
    throw "Git remote '$Name' is not configured. Add a private deploy remote first."
  }
  return $url.Trim()
}

function Run-Remote {
  param([string]$Command)
  ssh -p $script:SshPort "$script:SshUser@$script:SshHost" $Command
}

Read-DotEnv $EnvFile

$branch = if ($env:DEPLOY_BRANCH) { $env:DEPLOY_BRANCH } else { "sdkmax-prod" }
$remote = Require-Env "DEPLOY_REMOTE"
$remoteNameOnServer = if ($env:DEPLOY_REMOTE_NAME_ON_SERVER) { $env:DEPLOY_REMOTE_NAME_ON_SERVER } else { "origin" }
$healthUrl = if ($env:DEPLOY_HEALTH_URL) { $env:DEPLOY_HEALTH_URL } else { "https://api.sdkmax.com/api/status" }
$composeFile = if ($env:DEPLOY_COMPOSE_FILE) { $env:DEPLOY_COMPOSE_FILE } else { "docker-compose.prod.yml" }
$service = if ($env:DEPLOY_COMPOSE_SERVICE) { $env:DEPLOY_COMPOSE_SERVICE } else { "new-api" }
$healthTimeout = if ($env:DEPLOY_HEALTH_TIMEOUT_SECONDS) { [int]$env:DEPLOY_HEALTH_TIMEOUT_SECONDS } else { 90 }
$healthInterval = if ($env:DEPLOY_HEALTH_INTERVAL_SECONDS) { [int]$env:DEPLOY_HEALTH_INTERVAL_SECONDS } else { 3 }
$legacyContainer = if ($env:DEPLOY_LEGACY_CONTAINER_NAME) { $env:DEPLOY_LEGACY_CONTAINER_NAME } else { "" }

$script:SshHost = Require-Env "DEPLOY_SSH_HOST"
$script:SshUser = Require-Env "DEPLOY_SSH_USER"
$script:SshPort = if ($env:DEPLOY_SSH_PORT) { $env:DEPLOY_SSH_PORT } else { "22" }
$appDir = Require-Env "DEPLOY_REMOTE_APP_DIR"
$dataDir = Require-Env "DEPLOY_REMOTE_DATA_DIR"
$backupDir = Require-Env "DEPLOY_REMOTE_BACKUP_DIR"
$dbBackupCmd = Require-Env "DEPLOY_REMOTE_DB_BACKUP_CMD"

$currentBranch = (git branch --show-current).Trim()
if ($currentBranch -ne $branch) {
  throw "Current branch is '$currentBranch'. Switch to '$branch' before production deploy."
}

$remoteUrl = Get-GitRemoteUrl $remote
if ($remote -eq "origin" -or $remoteUrl -match "github\.com[:/]QuantumNous/new-api(\.git)?$") {
  throw "Refusing to deploy through '$remote' ($remoteUrl). Configure DEPLOY_REMOTE as a private SDKMAX deploy remote."
}

$workingTreeStatus = git status --porcelain
if (($workingTreeStatus -join "`n").Trim()) {
  throw "Working tree is not clean. Commit or stash changes before deploy."
}

$localCommit = (git rev-parse HEAD).Trim()
Write-Host "Deploying $branch at $localCommit"

git push $remote "${branch}:${branch}"

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$remoteScriptTemplate = @'
set -euo pipefail
cd '__APP_DIR__'
PREV_COMMIT=$(git rev-parse HEAD)
LEGACY_CONTAINER='__LEGACY_CONTAINER__'
legacy_exists() {
  [ -n "$LEGACY_CONTAINER" ] && docker ps -a --format '{{.Names}}' | grep -qx "$LEGACY_CONTAINER"
}
rollback_deploy() {
  echo 'Rolling back deployment.'
  git reset --hard "$PREV_COMMIT"
  docker compose -f '__COMPOSE_FILE__' down || true
  if legacy_exists; then
    docker start "$LEGACY_CONTAINER" || true
    echo "Legacy container restored: $LEGACY_CONTAINER"
  else
    docker compose -f '__COMPOSE_FILE__' build
    docker compose -f '__COMPOSE_FILE__' up -d
  fi
}
mkdir -p '__BACKUP_DIR__/__TIMESTAMP__'
echo "$PREV_COMMIT" > '__BACKUP_DIR__/__TIMESTAMP__/previous_commit.txt'
(__DB_BACKUP_CMD__) > '__BACKUP_DIR__/__TIMESTAMP__/db.sql'
if [ -d '__DATA_DIR__' ]; then tar -czf '__BACKUP_DIR__/__TIMESTAMP__/data.tar.gz' -C '__DATA_DIR__' .; fi
if [ -n "$(git status --porcelain)" ]; then
  echo 'Remote working tree has uncommitted changes. Refusing to reset.'
  git status --porcelain
  exit 1
fi
git fetch '__REMOTE_NAME_ON_SERVER__' '__BRANCH__'
git checkout '__BRANCH__'
git reset --hard '__REMOTE_NAME_ON_SERVER__/__BRANCH__'
docker compose -f '__COMPOSE_FILE__' build
if legacy_exists; then
  docker stop "$LEGACY_CONTAINER"
fi
if ! docker compose -f '__COMPOSE_FILE__' up -d; then
  rollback_deploy
  exit 1
fi
deadline=$((SECONDS + __HEALTH_TIMEOUT__))
until curl -fsS '__HEALTH_URL__' >/dev/null; do
  if [ $SECONDS -ge $deadline ]; then
    echo 'Health check failed. Rolling back to previous commit.'
    rollback_deploy
    exit 1
  fi
  sleep __HEALTH_INTERVAL__
done
echo 'Health check passed.'
if ! docker compose -f '__COMPOSE_FILE__' ps '__SERVICE__' >/dev/null; then
  echo 'Compose service check failed. Rolling back to previous commit.'
  rollback_deploy
  exit 1
fi
if legacy_exists; then
  echo "Legacy container remains stopped for manual rollback: $LEGACY_CONTAINER"
fi
'@

$remoteScript = $remoteScriptTemplate.
  Replace("__APP_DIR__", $appDir).
  Replace("__BACKUP_DIR__", $backupDir).
  Replace("__TIMESTAMP__", $timestamp).
  Replace("__DB_BACKUP_CMD__", $dbBackupCmd).
  Replace("__DATA_DIR__", $dataDir).
  Replace("__LEGACY_CONTAINER__", $legacyContainer).
  Replace("__REMOTE_NAME_ON_SERVER__", $remoteNameOnServer).
  Replace("__BRANCH__", $branch).
  Replace("__COMPOSE_FILE__", $composeFile).
  Replace("__HEALTH_TIMEOUT__", [string]$healthTimeout).
  Replace("__HEALTH_URL__", $healthUrl).
  Replace("__HEALTH_INTERVAL__", [string]$healthInterval).
  Replace("__SERVICE__", $service)

Run-Remote $remoteScript
Write-Host "Deployment finished successfully."
