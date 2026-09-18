@echo off
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"
set "ADMIN="
if exist "hackme.env" (
  for /f "usebackq tokens=1,* delims==" %%A in ("hackme.env") do (
    if /I "%%~A"=="HACKME_ADMIN_TOKEN" set "ADMIN=%%~B"
  )
)
set "ADMIN=!ADMIN:"=!"
if "!ADMIN!"=="" (echo ERR:no_admin & exit /b 1)
echo === processes ===
tasklist /FI "IMAGENAME eq hackme.exe" 2>nul | findstr hackme
tasklist /FI "IMAGENAME eq workerpoh.exe" 2>nul | findstr workerpoh
echo === status ===
curl -fsS -m 8 -H "X-Hackme-Admin-Token: !ADMIN!" http://127.0.0.1:8080/api/status
echo.
echo === worker metrics ===
curl -fsS -m 8 -H "X-Hackme-Admin-Token: !ADMIN!" http://127.0.0.1:8080/api/worker/metrics
echo.
echo === worker status ===
curl -fsS -m 8 -H "X-Hackme-Admin-Token: !ADMIN!" http://127.0.0.1:8080/api/worker/status
echo.
