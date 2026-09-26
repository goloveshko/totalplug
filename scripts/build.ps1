# scripts/build.ps1 - Unified task runner for TotalPlug
param (
    [Parameter(Position = 0)]
    [Alias("c", "cmd")]
    [ValidateSet("build", "b", "release", "rel", "gui", "g", "release-gui", "relg", "run", "r", "lint", "l", "test", "t", "fmt")]
    [string]$Action = "build",

    [Parameter(Position = 1, ValueFromRemainingArguments = $true)]
    [string[]]$ArgsList
)

$ErrorActionPreference = "Stop"
Set-Location "$PSScriptRoot\.."

$binDir = "bin"
$cliPath = "$binDir/totalplug-cli.exe"
$guiPath = "$binDir/totalplug.exe"
# Wails always compiles into build/bin ("-o" only renames the file inside it), so
# the GUI artifact is copied next to the CLI one to keep bin/ the only folder a
# user needs.
$wailsDir = "build/bin"
$wailsExe = "$wailsDir/totalplug.exe"

# Published builds are stamped from the latest reachable tag so the About panel,
# the release artifact name and the GitHub release all agree. Before the first
# tag exists `git describe` fails (and writes to stderr, which ErrorActionPreference
# would escalate), so the query runs in a quiet scope and the version stays the
# literal "dev": an unstamped build has no comparable version and must not claim one.
function Get-BuildVersion {
    $previous = $ErrorActionPreference
    $ErrorActionPreference = "SilentlyContinue"
    $describe = & git describe --tags --dirty 2>$null
    $exitCode = $LASTEXITCODE
    $ErrorActionPreference = $previous

    if ($exitCode -ne 0 -or -not $describe) { return "dev" }
    return ($describe -replace '^v', '')
}

$buildVersion = Get-BuildVersion
$versionLdflags = "-X totalplug/internal/buildinfo.Version=$buildVersion"

function Ensure-BinDir {
    if (!(Test-Path $binDir)) { New-Item -ItemType Directory -Path $binDir | Out-Null }
}

function Show-BinarySize {
    param([string]$Path = $cliPath)
    if (Test-Path $Path) {
        $sizeMB = [math]::Round((Get-Item $Path).Length / 1MB, 2)
        Write-Host "Binary size: $sizeMB MB ($Path)" -ForegroundColor Gray
    }
}

function Copy-GuiArtifact {
    if (!(Test-Path $wailsExe)) {
        Write-Host "`n❌ Wails left no $wailsExe behind — nothing to publish." -ForegroundColor Red
        exit 1
    }
    Ensure-BinDir
    Copy-Item $wailsExe $guiPath -Force
    Write-Host "`n✔ GUI build ready!" -ForegroundColor Green
    Show-BinarySize $guiPath
}

function Invoke-Build {
    Write-Host "======================================" -ForegroundColor Cyan
    Write-Host "      Building TotalPlug (Dev CLI)    " -ForegroundColor Cyan
    Write-Host "======================================" -ForegroundColor Cyan

    Ensure-BinDir

    Write-Host "Compiling development CLI binary (version $buildVersion)..." -ForegroundColor Yellow
    go build -ldflags="-X totalplug/internal/buildinfo.Version=$buildVersion" -o $cliPath ./cmd/totalplug

    if ($LASTEXITCODE -eq 0) {
        Write-Host "`n✔ CLI build successful!" -ForegroundColor Green
        Show-BinarySize
    } else {
        Write-Host "`n❌ Build failed!" -ForegroundColor Red
        exit $LASTEXITCODE
    }
}

function Invoke-Release {
    Write-Host "======================================" -ForegroundColor Cyan
    Write-Host "    Building TotalPlug (Release CLI)  " -ForegroundColor Cyan
    Write-Host "======================================" -ForegroundColor Cyan

    Ensure-BinDir

    # Pure Go release build (no CGO, stripped symbols, trimmed paths)
    $env:CGO_ENABLED = "0"

    Write-Host "Compiling optimized release binary (-trimpath -ldflags='-s -w')..." -ForegroundColor Yellow
    go build -trimpath -ldflags="-s -w $versionLdflags" -o $cliPath ./cmd/totalplug

    if ($LASTEXITCODE -eq 0) {
        Write-Host "`n✔ Release CLI build ready!" -ForegroundColor Green
        Show-BinarySize

        # Optional UPX compression if available
        if (Get-Command upx -ErrorAction SilentlyContinue) {
            Write-Host "`nCompressing binary with UPX..." -ForegroundColor Yellow
            upx --best --lzma $cliPath
            Show-BinarySize
        }
    } else {
        Write-Host "`n❌ Release build failed!" -ForegroundColor Red
        exit $LASTEXITCODE
    }
}

function Test-WailsCli {
    if (!(Get-Command wails -ErrorAction SilentlyContinue)) {
        Write-Host "❌ Wails CLI not found in PATH! Install via: go install github.com/wailsapp/wails/v2/cmd/wails@latest" -ForegroundColor Red
        exit 1
    }
}

# Neither GUI profile passes wails' "-clean": it deletes build/bin as its very first
# step, and a handle on that directory (IDE/Defender watcher, a TC or Explorer tab,
# a shell cwd) makes the build fail AFTER the previous exe is already gone. File
# deletes still work, so the release profile clears the old binary by name instead.
function Invoke-Gui {
    Test-WailsCli

    Write-Host "======================================" -ForegroundColor Cyan
    Write-Host "     Building TotalPlug (GUI, dev)    " -ForegroundColor Cyan
    Write-Host "======================================" -ForegroundColor Cyan

    Write-Host "Building $guiPath (version $buildVersion, dev profile)..." -ForegroundColor Yellow
    wails build -platform windows/amd64 -ldflags "$versionLdflags" @ArgsList

    if ($LASTEXITCODE -ne 0) {
        Write-Host "`n❌ Wails build failed!" -ForegroundColor Red
        exit $LASTEXITCODE
    }
    Copy-GuiArtifact
}

function Invoke-ReleaseGui {
    Test-WailsCli

    Write-Host "======================================" -ForegroundColor Cyan
    Write-Host "   Building TotalPlug (GUI, release)  " -ForegroundColor Cyan
    Write-Host "======================================" -ForegroundColor Cyan

    if (Test-Path $wailsExe) { Remove-Item $wailsExe -Force }

    # -trimpath only: wails' production build already passes "-s -w" to the compiler
    # and merges it with our -ldflags, so the stamped version survives stripping.
    Write-Host "Building $guiPath (version $buildVersion, release profile)..." -ForegroundColor Yellow
    wails build -platform windows/amd64 -trimpath -ldflags "$versionLdflags" @ArgsList

    if ($LASTEXITCODE -ne 0) {
        Write-Host "`n❌ Wails release build failed!" -ForegroundColor Red
        exit $LASTEXITCODE
    }
    Copy-GuiArtifact
}

function Invoke-Run {
    go run ./cmd/totalplug @ArgsList
}

function Invoke-Lint {
    Write-Host "======================================" -ForegroundColor Cyan
    Write-Host "       Running Go Code Analysis...    " -ForegroundColor Cyan
    Write-Host "======================================" -ForegroundColor Cyan

    Write-Host "`n1. Running 'go vet ./...'..." -ForegroundColor Yellow
    go vet ./...
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✔ go vet passed!" -ForegroundColor Green
    } else {
        Write-Host "❌ go vet found issues!" -ForegroundColor Red
        exit $LASTEXITCODE
    }

    Write-Host "`n2. Running 'golangci-lint'..." -ForegroundColor Yellow
    if (Get-Command golangci-lint -ErrorAction SilentlyContinue) {
        golangci-lint run ./...
        if ($LASTEXITCODE -eq 0) {
            Write-Host "✔ golangci-lint passed!" -ForegroundColor Green
        }
    } else {
        Write-Host "ℹ golangci-lint not found in PATH." -ForegroundColor DarkGray
        Write-Host "  Install via: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" -ForegroundColor DarkGray
    }
}

function Invoke-Test {
    Write-Host "======================================" -ForegroundColor Cyan
    Write-Host "       Running Tests (-v)...          " -ForegroundColor Cyan
    Write-Host "======================================" -ForegroundColor Cyan
    go test -v ./... @ArgsList
}

function Invoke-Fmt {
    Write-Host "Formatting Go source files..." -ForegroundColor Yellow
    go fmt ./...
    if (Get-Command goimports -ErrorAction SilentlyContinue) {
        goimports -w internal cmd
    }
    Write-Host "✔ Done!" -ForegroundColor Green
}

# Router
switch ($Action) {
    { $_ -in "build", "b" }                { Invoke-Build }
    { $_ -in "release", "rel" }            { Invoke-Release }
    { $_ -in "gui", "g" }                  { Invoke-Gui }
    { $_ -in "release-gui", "relg" }       { Invoke-ReleaseGui }
    { $_ -in "run",   "r" }                { Invoke-Run }
    { $_ -in "lint",  "l" }                { Invoke-Lint }
    { $_ -in "test",  "t" }                { Invoke-Test }
    { $_ -in "fmt" }                       { Invoke-Fmt }
}