@echo off
setlocal EnableExtensions
cd /d "%~dp0"
title HackMe Install

rem Prefer PowerShell launcher (newest Setup by mtime, clear missing-file UI).
if exist "%~dp0Install-HackMe.ps1" (
  powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0Install-HackMe.ps1"
  exit /b %ERRORLEVEL%
)

rem Fallback when Install-HackMe.ps1 is absent: dir /b yields zero rows if no match
rem (unlike for %%F in (glob), which can leave a literal * path).
set "SETUP="
for /f "delims=" %%F in ('dir /b /a-d /o-d "%~dp0HackMe-Setup-*.exe" 2^>nul') do (
  if not defined SETUP set "SETUP=%~dp0%%F"
)
if not defined SETUP (
  echo HackMe-Setup-*.exe not found in this folder.
  echo Download from https://hackme.tech/downloads.html
  pause
  exit /b 1
)
if not exist "%SETUP%" (
  echo HackMe-Setup-*.exe not found in this folder.
  echo Download from https://hackme.tech/downloads.html
  pause
  exit /b 1
)

echo Starting HackMe installer...
echo If nothing appears: right-click the .exe -^> Run as administrator
echo Or use portable ZIP: hackme_*_windows.zip
echo.

powershell -NoProfile -ExecutionPolicy Bypass -Command "try { Unblock-File -LiteralPath '%SETUP%' -ErrorAction SilentlyContinue } catch {}" 2>nul
start "" "%SETUP%"
timeout /t 3 /nobreak >nul
echo If the setup window did not open, press any key for portable ZIP instructions.
pause >nul
exit /b 0
