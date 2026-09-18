@echo off
rem Legacy name — forwards to the keep-alive watchdog (do not use one-shot autostart).
cd /d "%~dp0"
if exist "%~dp0run_watchdog_hidden.vbs" (
  wscript //B //Nologo "%~dp0run_watchdog_hidden.vbs"
) else (
  call "%~dp0watchdog_pool_worker.bat"
)
