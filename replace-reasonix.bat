@echo off
setlocal EnableDelayedExpansion

echo.
echo ============================================================
echo  Reasonix Desktop Replace Script
echo ============================================================
echo.

set "SCRIPT_DIR=%~dp0"
set "SRC=%SCRIPT_DIR%desktop\build\bin\reasonix-desktop.exe"
set "DST=C:\Users\admin\AppData\Local\Programs\Reasonix\reasonix-desktop.exe"
set "DST_DIR=C:\Users\admin\AppData\Local\Programs\Reasonix"

echo [0/6] Checking source...
if not exist "%SRC%" (
    echo   ERROR - Source not found
    echo   Please build desktop first
    goto :fail
)
echo   OK

echo [1/6] Stopping Reasonix...
taskkill /F /IM reasonix-desktop.exe >nul 2>&1
taskkill /F /IM reasonix.exe >nul 2>&1
timeout /t 2 /nobreak >nul
echo   Done

echo [2/6] Cleaning locks...
set "EBVIEW=C:\Users\admin\AppData\Roaming\reasonix-desktop.exe\EBWebView"
if exist "%EBVIEW%\SingletonLock" del /F /Q "%EBVIEW%\SingletonLock" >nul 2>&1
if exist "%EBVIEW%\SingletonSocket" del /F /Q "%EBVIEW%\SingletonSocket" >nul 2>&1
if exist "%EBVIEW%\SingletonCookie" del /F /Q "%EBVIEW%\SingletonCookie" >nul 2>&1
echo   Done

echo [3/6] Ensuring directory...
if not exist "%DST_DIR%" (
    mkdir "%DST_DIR%"
    echo   Created
) else (
    echo   Exists
)

echo [4/6] Backing up...
if exist "%DST%" (
    copy /Y "%DST%" "%DST%.bak" >nul 2>&1
    echo   Done
) else (
    echo   Skipped
)

echo [5/6] Replacing...
copy /Y "%SRC%" "%DST%" >nul 2>&1
if errorlevel 1 goto :fail
echo   Done

echo [6/6] Verifying...
if not exist "%DST%" goto :fail
echo   Done

echo.
echo ============================================================
echo  SUCCESS
echo ============================================================
goto :end

:fail
echo.
echo ============================================================
echo  FAILED
echo ============================================================

:end
pause
endlocal
