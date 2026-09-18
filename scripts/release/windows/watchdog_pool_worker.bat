@echo off
rem Keeps the public pool worker alive: restarts via node API if workerpoh exits (GPU reset, crash, thermal pause).
rem Designed to run hidden (see run_watchdog_hidden.vbs). All status goes to logs\watchdog_worker.log — not a blank console.
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"

if not exist "logs" mkdir "logs" >nul 2>&1
set "LOG=logs\watchdog_worker.log"

call :log "watchdog start dir=%CD%"

rem Prefer in-process HACKME_WORKER_WATCHDOG inside hackme.exe (avoids double-start + token races).
set "INTERNAL_WD="
if exist "hackme.env" (
  for /f "usebackq tokens=1,* delims==" %%A in ("hackme.env") do (
    if /I "%%~A"=="HACKME_WORKER_WATCHDOG" set "INTERNAL_WD=%%~B"
  )
)
if /I "!INTERNAL_WD!"=="1" (
  call :log "internal HACKME_WORKER_WATCHDOG=1 — external bat watchdog idle"
  exit /b 0
)
if /I "!INTERNAL_WD!"=="true" (
  call :log "internal HACKME_WORKER_WATCHDOG=true — external bat watchdog idle"
  exit /b 0
)

set "ADMIN_TOKEN="
set "POOL_TOKEN="
set "WORKER_ID="
set "GPU_BACKEND=auto"
set "WORKER_BATCH=16777216"

if not exist "hackme.env" (
  call :log "hackme.env missing — running write_hackme_env.ps1"
  if exist "write_hackme_env.ps1" if exist "pool.miner.token" (
    powershell -NoProfile -ExecutionPolicy Bypass -File "write_hackme_env.ps1" -InstallDir "%CD%" -RepairOnly -NonInteractive >> "%LOG%" 2>&1
  )
)

if exist "hackme.env" call :load_env

if "!POOL_TOKEN!"=="" if exist "pool.miner.token" (
  for /f "usebackq delims=" %%T in ("pool.miner.token") do set "POOL_TOKEN=%%T"
)

call :trim ADMIN_TOKEN
call :trim POOL_TOKEN
call :trim WORKER_ID
call :trim GPU_BACKEND
call :trim WORKER_BATCH

if "!ADMIN_TOKEN!"=="" (
  call :log "ERROR: HACKME_ADMIN_TOKEN empty — repairing hackme.env"
  if exist "write_hackme_env.ps1" if exist "pool.miner.token" (
    powershell -NoProfile -ExecutionPolicy Bypass -File "write_hackme_env.ps1" -InstallDir "%CD%" -RepairOnly -NonInteractive >> "%LOG%" 2>&1
    call :load_env
    call :trim ADMIN_TOKEN
  )
)

if "!ADMIN_TOKEN!"=="" (
  call :log "FATAL: no admin token after repair — run setup_hackme_miner.bat"
  exit /b 1
)
if "!POOL_TOKEN!"=="" (
  call :log "FATAL: no pool token"
  exit /b 1
)
if /I "!POOL_TOKEN!"=="REPLACE_WITH_POOL_TOKEN" (
  call :log "FATAL: pool token is placeholder"
  exit /b 1
)

if "!WORKER_ID!"=="" (
  for /f "usebackq delims=" %%H in (`powershell -NoProfile -Command "(hostname).ToLower() -replace '[^a-z0-9-]','-' "`) do set "WORKER_ID=worker-%%H"
)

if /I "!GPU_BACKEND!"=="auto" (
  if exist "workerpoh-cuda.exe" (
    set "GPU_BACKEND=cuda"
  ) else if exist "workerpoh-opencl.exe" (
    set "GPU_BACKEND=opencl"
  )
)
if "!WORKER_BATCH!"=="" set "WORKER_BATCH=16777216"

call :log "config worker_id=!WORKER_ID! gpu=!GPU_BACKEND! batch=!WORKER_BATCH! admin_ok=1"

set /a N=0
:waitnode
curl -fsS -o nul -H "X-Hackme-Admin-Token: !ADMIN_TOKEN!" http://127.0.0.1:8080/api/status 2>nul
if !ERRORLEVEL! EQU 0 (
  call :log "node ready after !N! probes"
  goto watchloop
)
set /a N+=1
if !N! GEQ 90 (
  call :log "FATAL: node not ready after 90 probes (admin token mismatch or node down)"
  exit /b 1
)
if !N! EQU 1 call :log "waiting for node on 127.0.0.1:8080 ..."
if !N! EQU 15 call :log "still waiting (!N!/90) — repair hackme.env if Mining API returns 401"
timeout /t 2 /nobreak >nul
goto waitnode

:watchloop
call :ensure_worker
timeout /t 45 /nobreak >nul
goto watchloop

:ensure_worker
set "RUNNING=0"
curl -fsS -H "X-Hackme-Admin-Token: !ADMIN_TOKEN!" http://127.0.0.1:8080/api/worker/status 2>nul | findstr /I "\"running\":true" >nul && set "RUNNING=1"
if "!RUNNING!"=="1" exit /b 0
tasklist 2>nul | findstr /I "workerpoh.exe workerpoh-opencl.exe workerpoh-cuda.exe" >nul && exit /b 0
call :log "restarting pool worker !WORKER_ID! backend=!GPU_BACKEND!"
curl -fsS -X POST -H "Content-Type: application/json" -H "X-Hackme-Admin-Token: !ADMIN_TOKEN!" ^
  -d "{\"coord_url\":\"https://hackme.tech/pool/coordinator\",\"worker_id\":\"!WORKER_ID!\",\"batch_size\":!WORKER_BATCH!,\"gpu_backend\":\"!GPU_BACKEND!\"}" ^
  http://127.0.0.1:8080/api/worker/start >> "%LOG%" 2>&1
echo.>> "%LOG%"
exit /b 0

:load_env
for /f "usebackq tokens=1,* delims==" %%A in ("hackme.env") do (
  set "_k=%%~A"
  if defined _k if not "!_k:~0,1!"=="#" (
    if /I "!_k!"=="HACKME_ADMIN_TOKEN" set "ADMIN_TOKEN=%%~B"
    if /I "!_k!"=="HACKME_POOL_COORDINATOR_TOKEN" set "POOL_TOKEN=%%~B"
    if /I "!_k!"=="WORKER_ID" set "WORKER_ID=%%~B"
    if /I "!_k!"=="HACKME_GPU_BACKEND" set "GPU_BACKEND=%%~B"
    if /I "!_k!"=="HACKME_WORKER_BATCH_SIZE" set "WORKER_BATCH=%%~B"
  )
)
exit /b 0

:trim
set "_v=!%~1!"
set "_v=!_v:"=!"
for /f "tokens=* delims= " %%T in ("!_v!") do set "_v=%%T"
set "%~1=!_v!"
exit /b 0

:log
set "_msg=%~1"
>>"%LOG%" echo [%date% %time%] !_msg!
exit /b 0
