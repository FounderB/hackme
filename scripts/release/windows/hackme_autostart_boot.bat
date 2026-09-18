@echo off
rem Optional Windows autostart: local node (public pool follower).
cd /d "%~dp0"
if not exist "hackme.exe" (
  echo [autostart] hackme.exe missing in %CD%
  exit /b 1
)

if exist "write_hackme_env.ps1" (
  powershell -NoProfile -ExecutionPolicy Bypass -File "write_hackme_env.ps1" >nul 2>&1
)

if exist "hackme.env" (
  for /f "usebackq tokens=1,* delims==" %%A in ("hackme.env") do (
    if not "%%A"=="" if not "%%~A"=="#" set "%%A=%%B"
  )
)
if exist ".env.desktop.windows" (
  for /f "usebackq tokens=1,* delims==" %%A in (".env.desktop.windows") do (
    if not "%%A"=="" if not "%%~A"=="#" set "%%A=%%B"
  )
)

if "%HACKME_ADMIN_TOKEN%"=="" (
  echo [autostart] HACKME_ADMIN_TOKEN empty — run start_hackme_miner.bat once
  exit /b 1
)

set "HACKME_BIND_ADDR=127.0.0.1:8080"
set "HACKME_DESKTOP_MODE=1"
start /min "" hackme.exe
exit /b 0
