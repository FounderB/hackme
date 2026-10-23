@echo off
REM W1 wrapper - run update_hackme_miner.ps1 from install dir or zip.
setlocal EnableExtensions
set "DIR=%~dp0"
if "%DIR:~-1%"=="\" set "DIR=%DIR:~0,-1%"
powershell -NoProfile -ExecutionPolicy Bypass -File "%DIR%\update_hackme_miner.ps1" %*
set "EC=%ERRORLEVEL%"
exit /b %EC%
