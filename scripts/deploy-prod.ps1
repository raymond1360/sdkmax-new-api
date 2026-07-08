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

function Run-Remote {
  param([string]$Command)
  ssh -p $script:SshPort "$script:SshUser@$script:SshHost" $Command
}

Read-DotEnv $EnvFile

$branch = if ($env:DEPLOY_BRANCH) { $env:DEPLOY_BRANCH } else { "sdkmax-prod" }
$remote = if ($env:DEPLOY_REMOTE) { $env:DEPLOY_REMOTE } else { "origin" }
$healthUrl = if ($env:DEPLOY_HEALTH_URL) { $env:DEPLOY_HEALTH_URL } else { "https://api.sdkmax.com/api/status" }
$composeFile = if ($env:DEPLOY_COMPOSE_FILE) { $env:DEPLOY_COMPOSE_FILE } else { "docker-compose.yml" }
$service = if ($env:DEPLOY_COMPOSE_SERVICE) { $env:DEPLOY_COMPOSE_SERVICE } else { "new-api" }

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

if ((git status --porcelain).Trim()) {
  throw "Working tree is not clean. Commit or stash changes before deploy."
}

$localCommit = (git rev-parse HEAD).Trim()
Write-Host "Deploying $branch at $localCommit"

git push $remote "${branch}:${branch}"

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$remoteScript = @"
set -euo pipefail
cd '$appDir'
PREV_COMMIT=\$(git rev-parse HEAD)
mkdir -p '$backupDir/$timestamp'
echo "\$PREV_COMMIT" > '$backupDir/$timestamp/previous_commit.txt'
($dbBackupCmd) > '$backupDir/$timestamp/db.sql'
if [ -d '$dataDir' ]; then tar -czf '$backupDir/$timestamp/data.tar.gz' -C '$dataDir' .; fi
git fetch origin '$branch'
git checkout '$branch'
git reset --hard 'origin/$branch'
docker compose -f '$composeFile' build
docker compose -f '$composeFile' up -d
sleep 8
if curl -fsS '$healthUrl' >/dev/null; then
  echo 'Health check passed.'
else
  echo 'Health check failed. Rolling back to previous commit.'
  git reset --hard "\$PREV_COMMIT"
  docker compose -f '$composeFile' build
  docker compose -f '$composeFile' up -d
  exit 1
fi
"@

Run-Remote $remoteScript
Write-Host "Deployment finished successfully."
