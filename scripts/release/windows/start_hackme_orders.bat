@echo off
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"
title HackMe Orders (B2B)

set "APP_DIR=%~dp0"
set "ENV_FILE=%APP_DIR%.env.orders.windows"
set "PID_FILE=%APP_DIR%logs\orders\node.pid"
set "LOG_FILE=%APP_DIR%logs\orders\node.log"

if not exist "%APP_DIR%logs\orders" mkdir "%APP_DIR%logs\orders" >nul 2>&1

rem Dig/from_code toolchains (Zig/TinyGo/Node/Rust) — portable under toolchains\
if /I not "%SKIP_TOOLCHAINS%"=="1" (
  if exist "%APP_DIR%install_from_code_toolchains.bat" (
    call "%APP_DIR%install_from_code_toolchains.bat" >nul 2>&1
  ) else if exist "%APP_DIR%install_from_code_toolchains.ps1" (
    powershell -NoProfile -ExecutionPolicy Bypass -File "%APP_DIR%install_from_code_toolchains.ps1" -AppDir "%APP_DIR%" >nul 2>&1
  )
)
if exist "%APP_DIR%.hackme-toolchains.env" (
  for /f "usebackq tokens=1,* delims==" %%A in ("%APP_DIR%.hackme-toolchains.env") do (
    if /I "%%A"=="PATH" set "PATH=%%B;%PATH%"
  )
)

if not exist "%APP_DIR%hackme.exe" (
  echo ERROR: hackme.exe not found in this folder.
  pause
  exit /b 1
)

if not exist "%ENV_FILE%" (
  for /f "usebackq delims=" %%i in (`powershell -NoProfile -Command "$b=New-Object byte[] 24; [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b); ($b|ForEach-Object ToString x2) -join ''"`) do set "GEN_TOKEN=%%i"
  > "%ENV_FILE%" (
    echo HACKME_BIND_ADDR=127.0.0.1:8080
    echo HACKME_ADMIN_TOKEN=!GEN_TOKEN!
    echo HACKME_REQUIRE_ADMIN_TOKEN=1
    echo HACKME_DESKTOP_MODE=1
    echo HACKME_DESKTOP_EXPOSE_ADMIN_TOKEN=1
    echo DESKTOP_PROFILE=orders
    echo HACKME_PUBLIC_AUTHORITY_BASE=https://hackme.tech
    echo HACKME_CANONICAL_CHAIN_URL=https://hackme.tech
  )
  echo Created %ENV_FILE% with generated admin token.
)

for /f "usebackq tokens=1,* delims==" %%A in ("%ENV_FILE%") do (
  set "_k=%%~A"
  if defined _k if not "!_k:~0,1!"=="#" (
    set "%%~A=%%~B"
  )
)
if "!HACKME_ADMIN_TOKEN!"=="" (
  echo ERROR: HACKME_ADMIN_TOKEN missing in %ENV_FILE%
  echo Delete that file and re-run to regenerate a token.
  pause
  exit /b 1
)

echo Starting HackMe Orders (local node for Dig / Hunt)...
echo Dashboard: http://127.0.0.1:8080/#orders
echo CLI: hackme-fuzzing.exe wizard --pack secrets --package audit
echo.

start "hackme-orders-browser" cmd /c "timeout /t 2 /nobreak >nul && start http://127.0.0.1:8080/#orders"
start "hackme-orders-node" /b cmd /c "hackme.exe >> \"%LOG_FILE%\" 2>&1"
echo %RANDOM% > "%PID_FILE%"

timeout /t 2 /nobreak >nul
curl -s http://127.0.0.1:8080/api/status >nul 2>&1
if errorlevel 1 (
  echo WARN: Node is still starting. Check %LOG_FILE%
) else (
  echo Node is up — create Dig/Hunt orders in the dashboard or via CLI.
)

echo.
echo Keep this window open while placing orders. Close when finished.
pause
exit /b 0
