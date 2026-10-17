# dev.ps1 - local development launcher
#
# Serves the frontend straight from web/ (templates are re-parsed per request,
# static files are read from disk), so editing CSS / JS / HTML only needs a
# browser refresh. It also watches files and reloads the page for you.
#
# Usage (from the repo root):
#   powershell -ExecutionPolicy Bypass -File scripts/dev.ps1
#   powershell -ExecutionPolicy Bypass -File scripts/dev.ps1 -Port 18091 -NoBuild
#
# What it does:
#   1. builds libra.exe once (-NoBuild skips this)
#   2. starts the server with LIBRA_WEB_DIR=web and LIBRA_DEV_STAMP=<stamp file>
#   3. watches the tree:
#        - web/static, web/templates changed -> bump the stamp; open pages
#          poll /__dev/reload and reload themselves within a second
#        - internal/, main.go, go.mod changed -> rebuild and restart
#   4. Ctrl+C stops everything
#
# Development only. Production leaves LIBRA_WEB_DIR unset and serves go:embed.

param(
  [int]$Port = 18091,
  [string]$DataDir = ".gotmp/demo-run",
  [switch]$NoBuild,
  [string]$Goproxy = ""
)

$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

function Say($msg, $color = "Cyan") { Write-Host "[dev] $msg" -ForegroundColor $color }

# ---------------------------------------------------------------- environment
$env:GOCACHE = Join-Path $root ".gocache"
$env:GOTMPDIR = Join-Path $root ".gotmp"
$env:GOFLAGS = "-mod=mod"
if ($Goproxy) { $env:GOPROXY = $Goproxy }
$env:LIBRA_ADDR = ":$Port"
$env:LIBRA_DATA_DIR = (Join-Path $root $DataDir)
$env:LIBRA_LIBRARY_ROOT = (Join-Path $root "testdata")
$env:LIBRA_SCAN_ON_START = "false"
$env:LIBRA_WEB_DIR = (Join-Path $root "web")

New-Item -ItemType Directory -Force -Path $env:GOCACHE, $env:GOTMPDIR, $env:LIBRA_DATA_DIR, $env:LIBRA_LIBRARY_ROOT | Out-Null

$exe = Join-Path $env:GOTMPDIR "libra.exe"
$stampFile = Join-Path $env:GOTMPDIR "dev.stamp"
$env:LIBRA_DEV_STAMP = $stampFile
$outLog = Join-Path $env:GOTMPDIR "dev.out.log"
$errLog = Join-Path $env:GOTMPDIR "dev.err.log"

function Set-Stamp {
  $ms = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
  Set-Content -LiteralPath $stampFile -Value $ms -NoNewline
}

# ---------------------------------------------------------------- build
function Build {
  Say "building..."
  & go build -o $exe . | Out-Host
  if ($LASTEXITCODE -ne 0) { Say "build failed, keeping the previous binary" "Red"; return $false }
  Say "build ok" "Green"
  return $true
}

if (-not $NoBuild -or -not (Test-Path $exe)) {
  if (-not (Build)) { exit 1 }
}

# ---------------------------------------------------------------- server process
$script:proc = $null

function Start-Server {
  Set-Stamp
  $script:proc = Start-Process -FilePath $exe -WorkingDirectory $env:LIBRA_DATA_DIR `
    -RedirectStandardOutput $outLog -RedirectStandardError $errLog -PassThru
  Say "listening on http://127.0.0.1:$Port  (pid $($script:proc.Id))" "Green"
}

function Stop-Server {
  if ($script:proc -and -not $script:proc.HasExited) {
    $script:proc | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 400
  }
}

Start-Server

# ---------------------------------------------------------------- watcher
$frontDirs = @((Join-Path $root "web/static"), (Join-Path $root "web/templates"))
$backPaths = @((Join-Path $root "internal"), (Join-Path $root "go.mod"), (Join-Path $root "go.sum"), (Join-Path $root "main.go"))

function Snapshot($paths) {
  $parts = New-Object System.Collections.Generic.List[string]
  foreach ($p in $paths) {
    if (Test-Path -LiteralPath $p -PathType Leaf) {
      $parts.Add("$p=$((Get-Item -LiteralPath $p).LastWriteTimeUtc.Ticks)")
      continue
    }
    if (-not (Test-Path -LiteralPath $p)) { continue }
    Get-ChildItem -LiteralPath $p -Recurse -File -ErrorAction SilentlyContinue | ForEach-Object {
      $parts.Add("$($_.FullName)=$($_.LastWriteTimeUtc.Ticks)")
    }
  }
  return ($parts | Sort-Object) -join "|"
}

$frontSnap = Snapshot $frontDirs
$backSnap = Snapshot $backPaths

try {
  while ($true) {
    Start-Sleep -Milliseconds 700

    if ($script:proc.HasExited) {
      Say "server exited; tail of $errLog" "Red"
      Get-Content $errLog -Tail 20 -ErrorAction SilentlyContinue | Out-Host
      break
    }

    $f2 = Snapshot $frontDirs
    if ($f2 -ne $frontSnap) {
      $frontSnap = $f2
      Set-Stamp
      Say "frontend changed -> page will reload itself" "Yellow"
    }

    $b2 = Snapshot $backPaths
    if ($b2 -ne $backSnap) {
      $backSnap = $b2
      Start-Sleep -Milliseconds 400
      Say "backend changed -> rebuild" "Yellow"
      Stop-Server
      if (Build) {
        Start-Server
        $frontSnap = Snapshot $frontDirs
        $backSnap = Snapshot $backPaths
      } else {
        break
      }
    }
  }
} finally {
  Stop-Server
  Say "stopped"
}
