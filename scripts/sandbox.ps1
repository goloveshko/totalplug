# -NoRuntime keeps the sandbox without a WebView2 runtime, which is the only way
# to reproduce the first run on a machine that never had one.
param([switch]$NoRuntime)

$ErrorActionPreference = "Stop"

# ASCII-only text in this file: Windows PowerShell 5.1 reads a BOM-less UTF-8
# script through the ANSI codepage, and an en dash (E2 80 93) decodes into bytes
# that include a curly double quote - the string breaks and the next word is run
# as a command.
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host " TotalPlug - Clean Environment Test Runner (Windows Sandbox) " -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan

# 1. Check if Windows Sandbox executable exists in System32 (does not require admin rights)
$sandboxExe = Join-Path $env:SystemRoot "System32\WindowsSandbox.exe"

if (-not (Test-Path $sandboxExe)) {
    Write-Host "`n[ERROR] Windows Sandbox is not enabled or installed on this system!" -ForegroundColor Red
    Write-Host "To enable it:" -ForegroundColor Yellow
    Write-Host "  1. Run elevated PowerShell as Administrator:"
    Write-Host "     Enable-WindowsOptionalFeature -FeatureName 'Containers-DisposableClientVM' -Online" -ForegroundColor White
    Write-Host "  2. Or open 'optionalfeatures.exe' -> check 'Windows Sandbox' -> Reboot.`n" -ForegroundColor White
    exit 1
}

# 2. Build fresh release binaries - the same profiles a GitHub release ships,
#    not the dev ones, so the sandbox sees what a user downloads.
$projectRoot = (Resolve-Path "$PSScriptRoot\..").Path

Write-Host "`n[BUILD] Compiling fresh release binaries..." -ForegroundColor Cyan
foreach ($wrapper in @("release.cmd", "release-gui.cmd")) {
    # Through the .cmd wrapper rather than `& build.ps1` in this process: build.ps1
    # reports a failed build with `exit`, and inside a running script that would
    # take the sandbox run down with it instead of setting $LASTEXITCODE.
    & cmd.exe /c "call `"$PSScriptRoot\$wrapper`""
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[ERROR] $wrapper failed (exit $LASTEXITCODE). Aborting Sandbox launch." -ForegroundColor Red
        exit 1
    }
}

# 3. Stage the binaries in a folder of their own, under bin\ (which is ignored).
# Not build\bin: that directory keeps every earlier artifact and whatever a
# previous sandbox run wrote back into it (the mapping is writable), so pointing
# the sandbox at it would test a machine that is anything but clean. Stale
# executables cannot ride along here because each run overwrites them, while a
# Total Commander fixture left next to them survives between runs on purpose -
# this is the folder to keep the test plugins in, all under one roof.
$stagingDir = Join-Path $projectRoot "bin\totalplug-sandbox"
if (-not (Test-Path $stagingDir)) {
    New-Item -ItemType Directory -Path $stagingDir | Out-Null
}

foreach ($exe in @("totalplug.exe", "totalplug-cli.exe")) {
    $source = Join-Path $projectRoot "bin\$exe"
    if (-not (Test-Path $source)) {
        Write-Host "[ERROR] Built executable not found at: $source" -ForegroundColor Red
        exit 1
    }
    Copy-Item -Path $source -Destination $stagingDir -Force
}

# 4. Stage the WebView2 runtime for the sandbox to install at startup.
# The sandbox image is a disposable Windows 10 with no WebView2 runtime, so a GUI
# run begins with Wails' own "Missing Requirements" dialog and that dialog's
# bootstrapper downloads the runtime inside the sandbox - slowly, and from
# scratch on every single run, because the next run starts from a fresh image.
# The Evergreen Standalone Installer is the same runtime as one offline download
# that stays on the host between runs.
#
# The three links below are the Standalone buttons on Microsoft's own download
# page (developer.microsoft.com/microsoft-edge/webview2), read from its source.
# Sizes are the current Content-Length: x64 204 MB, x86 181 MB, ARM64 207 MB.
$installerLinkIds = @{ "X64" = "2124701"; "X86" = "2099617"; "ARM64" = "2099616" }
$hostArch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64" -or $env:PROCESSOR_ARCHITEW6432 -eq "ARM64") {
    "ARM64"
} elseif ([Environment]::Is64BitOperatingSystem) {
    "X64"
} else {
    "X86"
}
# The sandbox runs the architecture of the host, so the installer has to match it.
$installerName = "MicrosoftEdgeWebView2RuntimeInstaller$hostArch.exe"
$installerPath = Join-Path $stagingDir $installerName
$stageInstaller = $false

if ($NoRuntime) {
    Write-Host "`n[RUNTIME] -NoRuntime: leaving the sandbox without WebView2, the GUI will open Wails' prompt." -ForegroundColor Yellow
} elseif (Test-Path $installerPath) {
    $installerMb = [math]::Round((Get-Item $installerPath).Length / 1MB)
    Write-Host "`n[RUNTIME] Offline installer staged from the previous run ($installerMb MB): $installerName" -ForegroundColor DarkCyan
    $stageInstaller = $true
} else {
    $url = "https://go.microsoft.com/fwlink/?linkid=" + $installerLinkIds[$hostArch]
    Write-Host "`n[RUNTIME] Downloading the Evergreen Standalone Installer $hostArch (about 200 MB) - once, then it stays here." -ForegroundColor Cyan
    # A missing curl.exe would throw and take the whole run down over a
    # convenience; the sandbox works without the staged runtime, only slower.
    try {
        & curl.exe --fail --location --silent --show-error --progress-bar --output $installerPath $url
    } catch {
        $LASTEXITCODE = 1
    }

    # A partial file would be staged and installed next run without anyone noticing.
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path $installerPath)) {
        Remove-Item -Path $installerPath -Force -ErrorAction SilentlyContinue
        Write-Host "[RUNTIME] Download failed (exit $LASTEXITCODE). Continuing without it." -ForegroundColor Yellow
        Write-Host "          The sandbox will install the runtime online, as before." -ForegroundColor Yellow
        Write-Host "          To retry by hand: $url" -ForegroundColor DarkCyan
        Write-Host "          Drop the file at: $installerPath" -ForegroundColor DarkCyan
    } else {
        $stageInstaller = $true
    }
}

# 5. Generate the sandbox startup script.
# The .wsb format takes a single LogonCommand, so the runtime install and the
# folder that opens afterwards are sequenced in a batch file instead.
# ASCII only, and written without a BOM: cmd.exe misreads a batch file that
# starts with UTF-8 marker bytes.
$startupPath = Join-Path $stagingDir "sandbox-startup.cmd"
$installBlock = ""
if ($stageInstaller) {
    # /silent /install is Microsoft's offline workflow; run from this elevated
    # sandbox user it is a per-machine install. The window's title is the only
    # progress the run shows, and the install takes about a minute.
    #
    # The install itself is silent and the sandbox dies with its console, so the
    # proof goes to a file: Microsoft's documented detection points are these two
    # pv values, and the mapped folder carries the log back to the host. A missing
    # key prints "unable to find the specified registry key", which is the answer
    # to "did it install" as much as a version number is.
    $installBlock = @"
title TotalPlug sandbox - installing the WebView2 runtime (once)
start /wait "" "%~dp0$installerName" /silent /install
(
  echo [%date% %time%] WebView2 runtime after the offline install:
  reg query "HKLM\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" /v pv
  reg query "HKCU\Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" /v pv
) > "%~dp0webview2-install.log" 2>&1
type "%~dp0webview2-install.log"
echo.
echo Keeping this window for 15 seconds; the log stays in the TotalPlug folder.
timeout /t 15 /nobreak >nul

"@
}

$startupContent = @"
@echo off
rem Generated by scripts/sandbox.ps1 on every run - change the script, not this file.
cd /d "%~dp0"
$($installBlock)explorer.exe "C:\Users\WDAGUtilityAccount\Desktop\TotalPlug"

"@
[System.IO.File]::WriteAllText($startupPath, $startupContent, (New-Object System.Text.UTF8Encoding($false)))

# 6. Generate dynamic WSB configuration
$wsbContent = @"
<!-- ============================================================================ -->
<!-- TotalPlug - Windows Sandbox Configuration                                -->
<!-- Generated dynamically by scripts/sandbox.ps1                              -->
<!-- ============================================================================ -->
<Configuration>
  <!-- Virtual GPU Acceleration (Default | Disable) -->
  <!-- Leave this on Default: the scan never touches the GPU, so Disable buys no
       speed, only software compositing. Uncomment the alternative when a run
       dies early with "The connection to the sandbox was lost. Error code:
       0x80072746" - the vGPU driver is the usual suspect for that one. -->
  <VGpu>Default</VGpu>
  <!-- <VGpu>Disable</VGpu> -->

  <!-- Network Access (Default | Disable) -->
  <Networking>Default</Networking>

  <!-- Audio Input / Microphone (Default | Enable | Disable) -->
  <AudioInput>Default</AudioInput>

  <!-- Video Input / Webcam (Default | Enable | Disable) -->
  <VideoInput>Disable</VideoInput>

  <!-- Enhanced Security Isolation (Default | Enable | Disable) -->
  <ProtectedClient>Disable</ProtectedClient>

  <!-- Printer Redirection (Default | Enable | Disable) -->
  <PrinterRedirection>Disable</PrinterRedirection>

  <!-- Shared Clipboard (Default | Disable) -->
  <ClipboardRedirection>Default</ClipboardRedirection>

  <!-- Memory Limit in MB (Uncomment to set fixed RAM limit, e.g. 4096 for 4 GB) -->
  <!-- <MemoryInMB>4096</MemoryInMB> -->

  <!-- Host Folders Shared with Sandbox -->
  <MappedFolders>
    <MappedFolder>
      <HostFolder>$stagingDir</HostFolder>
      <SandboxFolder>C:\Users\WDAGUtilityAccount\Desktop\TotalPlug</SandboxFolder>
      <ReadOnly>false</ReadOnly>
    </MappedFolder>
  </MappedFolders>

  <!-- Startup Command: the staged runtime install, if any, then the folder -->
  <LogonCommand>
    <Command>cmd.exe /c "C:\Users\WDAGUtilityAccount\Desktop\TotalPlug\sandbox-startup.cmd"</Command>
  </LogonCommand>
</Configuration>
"@

# Written next to the binaries it points at, so the whole experiment is one
# folder: the config, the startup script, the build, the runtime installer, the
# Total Commander fixture and whatever data folder the app creates in there.
$wsbPath = Join-Path $stagingDir "TotalPlug.wsb"
[System.IO.File]::WriteAllText($wsbPath, $wsbContent, [System.Text.Encoding]::UTF8)

Write-Host "[SANDBOX] Launching Windows Sandbox..." -ForegroundColor Green
Start-Process -FilePath $wsbPath