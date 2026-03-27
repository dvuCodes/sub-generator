@echo off
setlocal
set SCRIPT_DIR=%~dp0

if exist "%SCRIPT_DIR%runtime\python.exe" (
  "%SCRIPT_DIR%runtime\python.exe" "%SCRIPT_DIR%service.py" %*
  exit /b %errorlevel%
)

python "%SCRIPT_DIR%service.py" %*
