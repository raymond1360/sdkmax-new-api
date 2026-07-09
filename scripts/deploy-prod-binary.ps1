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
  $Command = $Command -replace "`r`n", "`n"
  $Command | ssh -p $script:SshPort "$script:SshUser@$script:SshHost" "bash -s"
  if ($LASTEXITCODE -ne 0) {
    throw "Remote command failed with exit code $LASTEXITCODE"
  }
}

Read-DotEnv $EnvFile

$branch = if ($env:DEPLOY_BRANCH) { $env:DEPLOY_BRANCH } else { "sdkmax-prod" }
$remote = Require-Env "DEPLOY_REMOTE"
$healthUrl = if ($env:DEPLOY_HEALTH_URL) { $env:DEPLOY_HEALTH_URL } else { "https://api.sdkmax.com/api/status" }
$healthTimeout = if ($env:DEPLOY_HEALTH_TIMEOUT_SECONDS) { [int]$env:DEPLOY_HEALTH_TIMEOUT_SECONDS } else { 120 }
$healthInterval = if ($env:DEPLOY_HEALTH_INTERVAL_SECONDS) { [int]$env:DEPLOY_HEALTH_INTERVAL_SECONDS } else { 3 }
$legacyContainer = Require-Env "DEPLOY_LEGACY_CONTAINER_NAME"

$script:SshHost = Require-Env "DEPLOY_SSH_HOST"
$script:SshUser = Require-Env "DEPLOY_SSH_USER"
$script:SshPort = if ($env:DEPLOY_SSH_PORT) { $env:DEPLOY_SSH_PORT } else { "22" }
$appDir = Require-Env "DEPLOY_REMOTE_APP_DIR"
$dataDir = Require-Env "DEPLOY_REMOTE_DATA_DIR"
$backupDir = Require-Env "DEPLOY_REMOTE_BACKUP_DIR"
$dbBackupCmd = Require-Env "DEPLOY_REMOTE_DB_BACKUP_CMD"
$goExe = if ($env:DEPLOY_GO_EXE) { $env:DEPLOY_GO_EXE } else { "D:\sdkmax\tools\go\bin\go.exe" }

$currentBranch = (git branch --show-current).Trim()
if ($currentBranch -ne $branch) {
  throw "Current branch is '$currentBranch'. Switch to '$branch' before production deploy."
}

$workingTreeStatus = git status --porcelain
if (($workingTreeStatus -join "`n").Trim()) {
  throw "Working tree is not clean. Commit or stash changes before deploy."
}

$localCommit = (git rev-parse HEAD).Trim()
$shortCommit = (git rev-parse --short HEAD).Trim()
Write-Host "Deploying binary build for $branch at $localCommit"

git push $remote "${branch}:${branch}"

Push-Location "web/default"
try {
  cmd /c npm run build
  if ($LASTEXITCODE -ne 0) { throw "Frontend build failed." }
} finally {
  Pop-Location
}

if (!(Test-Path $goExe)) {
  throw "Go executable not found: $goExe"
}

New-Item -ItemType Directory -Force -Path ".test" | Out-Null
$binaryPath = ".test\new-api-linux-amd64-$shortCommit"
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
& $goExe build -trimpath -ldflags="-s -w" -o $binaryPath .
if ($LASTEXITCODE -ne 0) { throw "Go build failed." }

$remoteBinary = "$appDir/.deploy/new-api-$shortCommit"
ssh -p $script:SshPort "$script:SshUser@$script:SshHost" "mkdir -p '$appDir/.deploy' '$appDir/logs'"
if ($LASTEXITCODE -ne 0) { throw "Failed to create remote deploy directories." }
scp -P $script:SshPort $binaryPath "$script:SshUser@$script:SshHost:$remoteBinary"
if ($LASTEXITCODE -ne 0) { throw "Failed to upload binary." }

$remoteScriptTemplate = @'
set -euo pipefail
APP_DIR='__APP_DIR__'
DATA_DIR='__DATA_DIR__'
BACKUP_DIR='__BACKUP_DIR__'
DB_BACKUP_CMD='__DB_BACKUP_CMD__'
LEGACY_CONTAINER='__LEGACY_CONTAINER__'
NEW_CONTAINER='sdkmax-new-api-__SHORT_COMMIT__'
SMOKE_CONTAINER='sdkmax-new-api-smoke-__SHORT_COMMIT__'
REMOTE_BINARY='__REMOTE_BINARY__'
HEALTH_URL='__HEALTH_URL__'
HEALTH_TIMEOUT=__HEALTH_TIMEOUT__
HEALTH_INTERVAL=__HEALTH_INTERVAL__

cd "$APP_DIR"
chmod 755 "$REMOTE_BINARY"
RUNTIME_IMAGE=$(docker inspect "$LEGACY_CONTAINER" --format '{{.Config.Image}}')
if [ -z "$RUNTIME_IMAGE" ]; then
  echo 'Unable to detect runtime image from legacy container.'
  exit 1
fi

timestamp=$(date +%Y%m%d-%H%M%S)
mkdir -p "$BACKUP_DIR/$timestamp"
echo '__LOCAL_COMMIT__' > "$BACKUP_DIR/$timestamp/deploy_commit.txt"
($DB_BACKUP_CMD) > "$BACKUP_DIR/$timestamp/db.sql"
if [ -d "$DATA_DIR" ]; then tar -czf "$BACKUP_DIR/$timestamp/data.tar.gz" -C "$DATA_DIR" .; fi

docker rm -f "$SMOKE_CONTAINER" >/dev/null 2>&1 || true
docker run -d --name "$SMOKE_CONTAINER" --network host --env-file "$APP_DIR/.env" -e PORT=3002 -v "$DATA_DIR:/data" -v "$REMOTE_BINARY:/new-api:ro" "$RUNTIME_IMAGE" >/dev/null
smoke_ok=0
for _ in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:3002/api/status >/dev/null; then smoke_ok=1; break; fi
  sleep 2
done
if [ "$smoke_ok" -ne 1 ]; then
  docker logs --tail 120 "$SMOKE_CONTAINER" || true
  docker rm -f "$SMOKE_CONTAINER" >/dev/null 2>&1 || true
  exit 1
fi
docker rm -f "$SMOKE_CONTAINER" >/dev/null

rollback() {
  echo 'Rolling back deployment.'
  docker rm -f "$NEW_CONTAINER" >/dev/null 2>&1 || true
  docker start "$LEGACY_CONTAINER" >/dev/null 2>&1 || true
}

docker rm -f "$NEW_CONTAINER" >/dev/null 2>&1 || true
docker stop "$LEGACY_CONTAINER" >/dev/null
if ! docker run -d --name "$NEW_CONTAINER" --restart always --network host --env-file "$APP_DIR/.env" -e PORT=3000 -v "$DATA_DIR:/data" -v "$APP_DIR/logs:/app/logs" -v "$REMOTE_BINARY:/new-api:ro" "$RUNTIME_IMAGE" --log-dir /app/logs >/dev/null; then
  rollback
  exit 1
fi

deadline=$((SECONDS + HEALTH_TIMEOUT))
until curl -fsS "$HEALTH_URL" >/dev/null; do
  if [ $SECONDS -ge $deadline ]; then
    docker logs --tail 120 "$NEW_CONTAINER" || true
    rollback
    exit 1
  fi
  sleep "$HEALTH_INTERVAL"
done

echo "Binary deployment finished: $NEW_CONTAINER"
docker ps
'@

$remoteScript = $remoteScriptTemplate.
  Replace("__APP_DIR__", $appDir).
  Replace("__DATA_DIR__", $dataDir).
  Replace("__BACKUP_DIR__", $backupDir).
  Replace("__DB_BACKUP_CMD__", $dbBackupCmd).
  Replace("__LEGACY_CONTAINER__", $legacyContainer).
  Replace("__SHORT_COMMIT__", $shortCommit).
  Replace("__REMOTE_BINARY__", $remoteBinary).
  Replace("__HEALTH_URL__", $healthUrl).
  Replace("__HEALTH_TIMEOUT__", [string]$healthTimeout).
  Replace("__HEALTH_INTERVAL__", [string]$healthInterval).
  Replace("__LOCAL_COMMIT__", $localCommit)

Run-Remote $remoteScript
Write-Host "Deployment finished successfully."
