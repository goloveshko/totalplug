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
# .NET's File APIs resolve a relative path against the process current directory,
# which Set-Location does NOT update, so it stays wherever powershell was launched
# (e.g. scripts/) and a bare "wails.json" below reads the wrong folder. Sync it to
# the project root so the relative paths this script relies on mean the same thing
# to cmdlets and to [System.IO.File] alike.
[Environment]::CurrentDirectory = (Get-Location).Path

$binDir = "bin"
$cliPath = "$binDir/totalplug-cli.exe"
$guiPath = "$binDir/totalplug.exe"
# Wails always compiles into build/bin ("-o" only renames the file inside it), so
# the GUI artifact is hard linked into bin/ to keep bin/ the only folder a user
# needs.
$wailsDir = "build/bin"
$wailsExe = "$wailsDir/totalplug.exe"
$wailsConfig = "wails.json"
# Read/written through .NET because Windows PowerShell decodes a BOM-less UTF-8
# file as ANSI (mangling the © in the copyright) and writes a BOM back out.
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)

# Published builds are stamped from the latest reachable tag so the About panel,
# the release artifact name and the GitHub release all agree. Before the first tag
# exists `git describe` fails (and writes to stderr, which ErrorActionPreference
# would escalate), so the query runs in a quiet scope and the version comes from the
# value tracked in wails.json instead: an untagged tree reads as its upcoming
# release. The price of that is that such a build can report "update available" once
# the release it is guessing at actually gets published.
function Get-ConfigFallbackVersion {
    if (Test-Path $wailsConfig) {
        $content = [System.IO.File]::ReadAllText($wailsConfig)
        if ($content -match '"productVersion"\s*:\s*"([^"]+)"') {
            return $matches[1]
        }
    }
    return "1.0.0"
}

function Get-BuildVersion {
    $previous = $ErrorActionPreference
    $ErrorActionPreference = "SilentlyContinue"
    $describe = & git describe --tags --dirty 2>$null
    $exitCode = $LASTEXITCODE
    $ErrorActionPreference = $previous

    if ($exitCode -eq 0 -and $describe) {
        return ($describe -replace '^v', '')
    }
    return (Get-ConfigFallbackVersion)
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

function Publish-GuiArtifact {
    if (!(Test-Path $wailsExe)) {
        Write-Host "`n❌ Wails left no $wailsExe behind — nothing to publish." -ForegroundColor Red
        exit 1
    }
    Ensure-BinDir

    # A hard link instead of a 13 MB copy: bin/totalplug.exe and the Wails artifact
    # become one file, so the two can never disagree, and the name survives a
    # forgotten window — unlinking a running image is allowed, writing over it is
    # not, so a copy over the GUI that is still open is what used to fail here.
    # Always recreated, because Wails drops a fresh file at the artifact path and a
    # link left standing would keep pointing at the one it replaced.
    if (Test-Path $guiPath) { Remove-Item $guiPath -Force }
    try {
        New-Item -ItemType HardLink -Path $guiPath -Target (Resolve-Path $wailsExe) -ErrorAction Stop | Out-Null
    } catch {
        Write-Host "`n❌ Could not link $guiPath to $wailsExe — a hard link needs both on the same volume." -ForegroundColor Red
        Write-Host "   $($_.Exception.Message)" -ForegroundColor Red
        exit 1
    }

    Write-Host "`n✔ GUI build ready!" -ForegroundColor Green
    Show-BinarySize $guiPath
}

function Invoke-Build {
    Write-Host "======================================" -ForegroundColor Cyan
    Write-Host "      Building TotalPlug (Dev CLI)    " -ForegroundColor Cyan
    Write-Host "======================================" -ForegroundColor Cyan

    Ensure-BinDir

    Write-Host "Compiling development CLI binary (version $buildVersion)..." -ForegroundColor Yellow
    go build -ldflags="$versionLdflags" -o $cliPath ./cmd/totalplug

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

    $env:CGO_ENABLED = "0"

    Write-Host "Compiling optimized release binary (-trimpath -ldflags='-s -w')..." -ForegroundColor Yellow
    go build -trimpath -ldflags="-s -w $versionLdflags" -o $cliPath ./cmd/totalplug

    if ($LASTEXITCODE -eq 0) {
        Write-Host "`n✔ Release CLI build ready!" -ForegroundColor Green
        Show-BinarySize

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

# Wails has no flag for an alternate config file, and the VERSIONINFO resource it
# embeds is rendered from wails.json -> info.productVersion, so that is the only
# place the stamped version can go. Invoke-GuiBuild writes it, builds, and puts the
# file back whichever way the build ends, so the tracked value stays the pre-tag
# fallback Get-ConfigFallbackVersion reads rather than a second number someone has
# to keep in step with the tag.
function Set-WailsProductVersion {
    param([string]$Version)

    $content = [System.IO.File]::ReadAllText($wailsConfig)
    $pattern = '("productVersion"\s*:\s*")[^"]*(")'
    if ($content -notmatch $pattern) {
        Write-Host "❌ No productVersion field to stamp in $wailsConfig" -ForegroundColor Red
        exit 1
    }

    # A VERSIONINFO resource takes only dotted numbers, so a describe string such as
    # 1.0.0-1-gdeadbeef-dirty is reduced to its numeric head for the PE block. The
    # ldflags copy in $versionLdflags keeps the full string, so the About panel and
    # the file properties deliberately differ on an off-tag build.
    $cleanVersion = ($Version -split '-')[0]

    $updated = $content -replace $pattern, ('${1}' + $cleanVersion + '${2}')
    [System.IO.File]::WriteAllText($wailsConfig, $updated, $utf8NoBom)
}

# Neither GUI profile passes wails' "-clean": it deletes build/bin as its very first
# step, and a handle on that directory (IDE/Defender watcher, a TC or Explorer tab,
# a shell cwd) makes the build fail AFTER the previous exe is already gone. File
# deletes still work, so the release profile clears the old binary by name instead.
function Invoke-GuiBuild {
    param(
        [string]$Kind,
        [string[]]$WailsArgs
    )

    Test-WailsCli

    Write-Host "======================================" -ForegroundColor Cyan
    Write-Host ("     Building TotalPlug (GUI, {0})" -f $Kind.PadRight(7)) -ForegroundColor Cyan
    Write-Host "======================================" -ForegroundColor Cyan

    if ($Kind -eq "release" -and (Test-Path $wailsExe)) {
        Remove-Item $wailsExe -Force -ErrorAction SilentlyContinue
    }

    Write-Host "Building $guiPath (version $buildVersion, $Kind profile)..." -ForegroundColor Yellow
    $originalConfig = [System.IO.File]::ReadAllText($wailsConfig)
    $allArgs = $WailsArgs + $ArgsList
    try {
        Set-WailsProductVersion $buildVersion
        wails build @allArgs
    } finally {
        [System.IO.File]::WriteAllText($wailsConfig, $originalConfig, $utf8NoBom)
    }

    if ($LASTEXITCODE -ne 0) {
        Write-Host "`n❌ Wails $Kind build failed!" -ForegroundColor Red
        exit $LASTEXITCODE
    }
    Publish-GuiArtifact
}

function Invoke-Gui {
    Invoke-GuiBuild "dev" @("-platform", "windows/amd64", "-ldflags=$versionLdflags")
}

function Invoke-ReleaseGui {
    # -trimpath only: wails' production build already passes "-s -w" to the compiler
    # and merges it with our -ldflags, so the stamped version survives stripping.
    Invoke-GuiBuild "release" @("-platform", "windows/amd64", "-trimpath", "-ldflags=$versionLdflags")
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
