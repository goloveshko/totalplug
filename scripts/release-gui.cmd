@echo off
chcp 65001 >nul
cd /d "%~dp0"
powershell -NoProfile -ExecutionPolicy Bypass -File "build.ps1" release-gui %*
echo %cmdcmdline% | find /i "%~0" >nul && pause