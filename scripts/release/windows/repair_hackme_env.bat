@echo off
rem Re-apply fair Windows pool env (NVIDIA cooldown 0, no forced 28s sleep).
setlocal EnableExtensions
cd /d "%~dp0"
set "HACKME_DIR=%~dp0"
if "%HACKME_DIR:~-1%"=="\" set "HACKME_DIR=%HACKME_DIR:~0,-1%"
powershell -NoProfile -ExecutionPolicy Bypass -File "%HACKME_DIR%\write_hackme_env.ps1" -InstallDir "%HACKME_DIR%" -RepairOnly -NonInteractive
if errorlevel 1 (
  echo ERROR: could not repair hackme.env
  pause
  exit /b 1
)
echo.
echo Done. Restart: start_hackme_miner.bat
pause
exit /b 0
