@echo off
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"
set "ADMIN="
set "POOL="
if exist "hackme.env" (
  for /f "usebackq tokens=1,* delims==" %%A in ("hackme.env") do (
    if /I "%%~A"=="HACKME_ADMIN_TOKEN" set "ADMIN=%%~B"
    if /I "%%~A"=="HACKME_POOL_COORDINATOR_TOKEN" set "POOL=%%~B"
  )
)
set "ADMIN=!ADMIN:"=!"
set "POOL=!POOL:"=!"
if "!ADMIN!"=="" (
  echo ERR:no_admin — run setup_hackme_miner.bat
  exit /b 1
)
echo ADMIN_SET=!ADMIN:~0,4!...
echo POOL_SET=!POOL:~0,4!...
tasklist /FI "IMAGENAME eq hackme.exe" 2>nul | findstr hackme
tasklist /FI "IMAGENAME eq workerpoh.exe" 2>nul | findstr workerpoh
curl -fsS -m 5 -H "X-Hackme-Admin-Token: !ADMIN!" http://127.0.0.1:8080/api/worker/metrics 2>nul
echo.
curl -fsS -m 5 -H "X-Hackme-Admin-Token: !ADMIN!" http://127.0.0.1:8080/api/status 2>nul
echo.
