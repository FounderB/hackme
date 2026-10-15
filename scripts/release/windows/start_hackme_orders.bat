@echo off
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"
title HackMe Orders (B2B)

rem Install root (strip trailing backslash for clean joins)
set "APP_DIR=%~dp0"
if "%APP_DIR:~-1%"=="\" set "APP_DIR=%APP_DIR:~0,-1%"

set "ENV_FILE=%APP_DIR%\.env.orders.windows"
set "LOG_DIR=%APP_DIR%\logs\orders"
set "DATA_DIR=%APP_DIR%\data\orders"
set "BASE_URL=http://127.0.0.1:8080"

echo.
echo HackMe Orders -- preparing...
if not exist "%LOG_DIR%" mkdir "%LOG_DIR%" >nul 2>&1
if not exist "%DATA_DIR%" mkdir "%DATA_DIR%" >nul 2>&1

if not exist "%APP_DIR%\hackme.exe" (
  echo ERROR: hackme.exe not found in:
  echo   %APP_DIR%
  echo Reinstall from https://hackme.tech/downloads.html
  pause
  exit /b 1
)

rem Toolchains are optional and slow on first run -- never block the node.
rem Set HACKME_INSTALL_TOOLCHAINS=1 to install Dig toolchains in the background.
if /I "%HACKME_INSTALL_TOOLCHAINS%"=="1" (
  if /I not "%SKIP_TOOLCHAINS%"=="1" (
    if exist "%APP_DIR%\install_from_code_toolchains.bat" (
      echo Dig toolchains: installing in background ^(log: %LOG_DIR%\toolchains.log^)...
      start "hackme-orders-toolchains" /b cmd /c "call \"%APP_DIR%\install_from_code_toolchains.bat\" > \"%LOG_DIR%\toolchains.log\" 2>&1"
    )
  )
)
if exist "%APP_DIR%\.hackme-toolchains.env" (
  for /f "usebackq tokens=1,* delims==" %%A in ("%APP_DIR%\.hackme-toolchains.env") do (
    if /I "%%A"=="PATH" set "PATH=%%B;%PATH%"
  )
)

if not exist "%ENV_FILE%" (
  echo Generating Orders admin token...
  set "GEN_TOKEN="
  for /f "usebackq delims=" %%i in (`powershell -NoProfile -Command "$b=New-Object byte[] 24; [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b); ($b|ForEach-Object ToString x2) -join ''"`) do set "GEN_TOKEN=%%i"
  if "!GEN_TOKEN!"=="" (
    echo ERROR: could not generate HACKME_ADMIN_TOKEN
    pause
    exit /b 1
  )
  (
    echo HACKME_BIND_ADDR=127.0.0.1:8080
    echo HACKME_ADMIN_TOKEN=!GEN_TOKEN!
    echo HACKME_REQUIRE_ADMIN_TOKEN=1
    echo HACKME_DESKTOP_MODE=1
    echo HACKME_DESKTOP_EXPOSE_ADMIN_TOKEN=1
    echo HACKME_INTEGRATOR_SELF_REGISTER=1
    echo DESKTOP_PROFILE=orders
    echo WORKER_AUTOSTART=0
    echo HACKME_WORKER_WATCHDOG=0
    echo HACKME_DESKTOP_GPU_POOL=0
    echo HACKME_DATA_DIR=!DATA_DIR!
    echo HACKME_WORKING_DIR=!APP_DIR!
    echo HACKME_PUBLIC_AUTHORITY_BASE=https://hackme.tech
    echo HACKME_CANONICAL_CHAIN_URL=https://hackme.tech
  ) > "%ENV_FILE%"
  echo Created %ENV_FILE%
)

rem Load Orders env into this process (wins over hackme.env inside the binary)
for /f "usebackq tokens=1,* delims==" %%A in ("%ENV_FILE%") do (
  set "_k=%%~A"
  if defined _k if not "!_k:~0,1!"=="#" (
    set "%%~A=%%~B"
  )
)

if "!HACKME_ADMIN_TOKEN!"=="" (
  echo ERROR: HACKME_ADMIN_TOKEN missing in %ENV_FILE%
  echo Delete that file and re-run to regenerate.
  pause
  exit /b 1
)

rem Ensure Orders isolation even if env file is from an older install
if "!HACKME_DATA_DIR!"=="" set "HACKME_DATA_DIR=%DATA_DIR%"
if "!HACKME_WORKING_DIR!"=="" set "HACKME_WORKING_DIR=%APP_DIR%"
if "!HACKME_BIND_ADDR!"=="" set "HACKME_BIND_ADDR=127.0.0.1:8080"
set "DESKTOP_PROFILE=orders"
set "WORKER_AUTOSTART=0"
set "HACKME_WORKER_WATCHDOG=0"
set "HACKME_DESKTOP_GPU_POOL=0"
set "HACKME_DESKTOP_MODE=1"
set "HACKME_REQUIRE_ADMIN_TOKEN=1"
set "HACKME_DESKTOP_EXPOSE_ADMIN_TOKEN=1"
set "HACKME_INTEGRATOR_SELF_REGISTER=1"
set "HACKME_PUBLIC_AUTHORITY_BASE=https://hackme.tech"
set "HACKME_CANONICAL_CHAIN_URL=https://hackme.tech"

echo.
echo Starting HackMe Orders ^(local node for Dig / Hunt^)...
echo Install:   %APP_DIR%
echo Dashboard: %BASE_URL%/#orders
echo Data:      !HACKME_DATA_DIR!
if exist "%APP_DIR%\hackme-fuzzing.exe" (
  echo CLI:       hackme-fuzzing.exe wizard --pack secrets --package audit
) else (
  echo CLI:       download hackme-fuzzing from https://hackme.tech/downloads.html#fuzzing-client
)
echo.
echo If Windows asks for permission ^(UAC^), click Yes.
echo Keep this window open while placing orders. Close when finished.
echo.
echo Launching hackme.exe ...

rem Open browser after bind (same pattern as Miner -- no nested cmd /c log redirect)
start "hackme-orders-browser" cmd /c "timeout /t 4 /nobreak >nul && start http://127.0.0.1:8080/#orders"

cd /d "%APP_DIR%"
hackme.exe
set "EC=!ERRORLEVEL!"
if not "!EC!"=="0" (
  echo.
  echo Node exited with code !EC!.
  echo If port 8080 is busy: close HackMe Miner first, then retry Orders.
  echo If admin-token errors: delete "%ENV_FILE%" and re-run this launcher.
  echo If UAC was denied: run Orders again and click Yes.
  pause
)
exit /b !EC!
