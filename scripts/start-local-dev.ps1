param(
  [switch]$SkipFrontendBuild,
  [int]$Port = 3000
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path (Join-Path $scriptDir "..")
$testDir = Join-Path $repoRoot ".test"
$binaryPath = Join-Path $testDir "new-api-local-current.exe"
$stdoutLog = Join-Path $testDir "new-api-local-current.out.log"
$stderrLog = Join-Path $testDir "new-api-local-current.err.log"
$goBin = "D:\sdkmax\tools\go\bin"
$frontendDir = Join-Path $repoRoot "web\default"

function Write-Step {
  param([string]$Message)
  Write-Host ""
  Write-Host "==> $Message"
}

function Get-ProcessPath {
  param([int]$ProcessId)
  try {
    return (Get-Process -Id $ProcessId -ErrorAction Stop).Path
  } catch {
    return $null
  }
}

function Stop-LocalNewApiProcess {
  param([int]$ProcessId)

  $processPath = Get-ProcessPath -ProcessId $ProcessId
  if (-not $processPath) {
    return
  }

  $normalizedRepo = [System.IO.Path]::GetFullPath($repoRoot.Path)
  $normalizedPath = [System.IO.Path]::GetFullPath($processPath)
  $isRepoTestBinary = $normalizedPath.StartsWith((Join-Path $normalizedRepo ".test"), [System.StringComparison]::OrdinalIgnoreCase)
  $isNewApiLocalBinary = [System.IO.Path]::GetFileName($normalizedPath) -like "new-api-local*.exe"

  if ($isRepoTestBinary -and $isNewApiLocalBinary) {
    Write-Host "Stopping old local binary: $normalizedPath (PID $ProcessId)"
    Stop-Process -Id $ProcessId -Force
    return
  }

  throw "Port $Port is occupied by PID $ProcessId ($normalizedPath). Stop it manually or choose another port."
}

Set-Location $repoRoot
New-Item -ItemType Directory -Force -Path $testDir | Out-Null

Write-Step "SDKMAX local dev entry"
$branch = git rev-parse --abbrev-ref HEAD
$commit = git rev-parse --short HEAD
Write-Host "Branch: $branch"
Write-Host "Commit: $commit"

Write-Step "Clearing old local new-api binaries"
$listeners = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
foreach ($listener in $listeners) {
  Stop-LocalNewApiProcess -ProcessId $listener.OwningProcess
}

$repoTestRoot = [System.IO.Path]::GetFullPath($testDir)
Get-Process -ErrorAction SilentlyContinue |
  Where-Object {
    $_.Path -and
    ([System.IO.Path]::GetFullPath($_.Path)).StartsWith($repoTestRoot, [System.StringComparison]::OrdinalIgnoreCase) -and
    $_.ProcessName -like "new-api-local*"
  } |
  ForEach-Object {
    Write-Host "Stopping stale local binary: $($_.Path) (PID $($_.Id))"
    Stop-Process -Id $_.Id -Force
  }

Start-Sleep -Seconds 1

if (-not $SkipFrontendBuild) {
  Write-Step "Building default frontend"
  Set-Location $frontendDir
  & npm.cmd run build
  Set-Location $repoRoot
} else {
  Write-Step "Skipping frontend build"
}

Write-Step "Building fixed local backend binary"
if (Test-Path $goBin) {
  $env:PATH = "$goBin;$env:PATH"
}
& go version
& go build -o $binaryPath .

Write-Step "Starting localhost:$Port"
Start-Process `
  -FilePath $binaryPath `
  -WorkingDirectory $repoRoot `
  -RedirectStandardOutput $stdoutLog `
  -RedirectStandardError $stderrLog `
  -WindowStyle Hidden

$statusUrl = "http://localhost:$Port/api/status"
$pricingUrl = "http://localhost:$Port/pricing"
$deadline = (Get-Date).AddSeconds(30)
$lastError = $null

do {
  Start-Sleep -Seconds 1
  try {
    $response = Invoke-WebRequest -Uri $statusUrl -UseBasicParsing -TimeoutSec 3
    if ($response.StatusCode -eq 200) {
      $lastError = $null
      break
    }
  } catch {
    $lastError = $_.Exception.Message
  }
} while ((Get-Date) -lt $deadline)

if ($lastError) {
  Write-Host ""
  Write-Host "Startup failed. stderr tail:"
  if (Test-Path $stderrLog) {
    Get-Content $stderrLog -Tail 80
  }
  throw "Local service did not become healthy: $lastError"
}

$activeListener = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction Stop | Select-Object -First 1
$activePath = Get-ProcessPath -ProcessId $activeListener.OwningProcess
if ([System.IO.Path]::GetFullPath($activePath) -ne [System.IO.Path]::GetFullPath($binaryPath)) {
  throw "localhost:$Port is not running the fixed binary. Active process: $activePath"
}

$pricingResponse = Invoke-WebRequest -Uri $pricingUrl -UseBasicParsing -TimeoutSec 5
if ($pricingResponse.StatusCode -ne 200) {
  throw "Pricing page health check failed with status $($pricingResponse.StatusCode)"
}

Write-Host ""
Write-Host "SDKMAX local dev is ready."
Write-Host "Binary: $binaryPath"
Write-Host "Status: $statusUrl"
Write-Host "Pricing: $pricingUrl"
Write-Host "Logs: $stdoutLog"
