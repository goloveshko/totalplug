# scripts/build.ps1 - Unified task runner for TotalPlug
param (
    [Parameter(Position = 0)]
    [Alias("c", "cmd")]
    [ValidateSet("build", "b", "release", "rel", "wails", "w", "run", "r", "lint", "l", "test", "t", "fmt")]
    [string]$Action = "build",

    [Parameter(Position = 1, ValueFromRemainingArguments = $true)]
    [string[]]$ArgsList
)

$ErrorActionPreference = "Stop"
Set-Location "$PSScriptRoot\.."

$binDir = "bin"
$binPath = "$binDir/totalplug.exe"

function Ensure-BinDir {
    if (!(Test-Path $binDir)) { New-Item -ItemType Directory -Path $binDir | Out-Null }
}

function Show-BinarySize {
    param([string]$Path = $binPath)
    if (Test-Path $Path) {
        $sizeMB = [math]::Round((Get-Item $Path).Length / 1MB, 2)
        Write-Host "Binary size: $sizeMB MB ($Path)" -ForegroundColor Gray
    }
}

function Invoke-Build {
    Write-Host "======================================" -ForegroundColor Cyan
    Write-Host "      Building TotalPlug (Dev CLI)    " -ForegroundColor Cyan
    Write-Host "======================================" -ForegroundColor Cyan

    Ensure-BinDir

    Write-Host "Compiling development CLI binary..." -ForegroundColor Yellow
    go build -o $binPath ./cmd/totalplug

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
    go build -trimpath -ldflags="-s -w" -o $binPath ./cmd/totalplug

    if ($LASTEXITCODE -eq 0) {
        Write-Host "`n✔ Release CLI build ready!" -ForegroundColor Green
        Show-BinarySize

        # Optional UPX compression if available
        if (Get-Command upx -ErrorAction SilentlyContinue) {
            Write-Host "`nCompressing binary with UPX..." -ForegroundColor Yellow
            upx --best --lzma $binPath
            Show-BinarySize
        }
    } else {
        Write-Host "`n❌ Release build failed!" -ForegroundColor Red
        exit $LASTEXITCODE
    }
}

function Invoke-Wails {
    Write-Host "======================================" -ForegroundColor Cyan
    Write-Host "    Building TotalPlug (Wails GUI)    " -ForegroundColor Cyan
    Write-Host "======================================" -ForegroundColor Cyan

    if (!(Get-Command wails -ErrorAction SilentlyContinue)) {
        Write-Host "❌ Wails CLI not found in PATH! Install via: go install github.com/wailsapp/wails/v2/cmd/wails@latest" -ForegroundColor Red
        exit 1
    }

    Write-Host "Compiling production Wails GUI application..." -ForegroundColor Yellow
    wails build -platform windows/amd64 -clean

    if ($LASTEXITCODE -eq 0) {
        Write-Host "`n✔ Wails GUI build ready!" -ForegroundColor Green
        Show-BinarySize "build/bin/totalplug.exe"
    } else {
        Write-Host "`n❌ Wails build failed!" -ForegroundColor Red
        exit $LASTEXITCODE
    }
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
    { $_ -in "build", "b" }     { Invoke-Build }
    { $_ -in "release", "rel" } { Invoke-Release }
    { $_ -in "wails", "w" }     { Invoke-Wails }
    { $_ -in "run",   "r" }     { Invoke-Run }
    { $_ -in "lint",  "l" }     { Invoke-Lint }
    { $_ -in "test",  "t" }     { Invoke-Test }
    { $_ -in "fmt" }            { Invoke-Fmt }
}