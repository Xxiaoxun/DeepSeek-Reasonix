@echo off
setlocal EnableDelayedExpansion

title Reasonix Desktop Build

REM Trap any unexpected exit so the window doesn't vanish.
if not defined BUILD_BAT_REENTRY (
    set "BUILD_BAT_REENTRY=1"
    cmd /c ""%~f0""
    echo.
    echo [build-reasonix.bat finished, exit code: %errorlevel%]
    pause
    exit /b %errorlevel%
)

cd /d "%~dp0desktop"
if errorlevel 1 (
    echo [ERROR] Cannot cd to "%~dp0desktop"
    pause
    exit /b 1
)

REM Force go to skip VCS stamping. The repo sits inside an SVN working copy
REM and contains a nested Git repo, so go build errors out with
REM multiple VCS detected otherwise -- wails build calls go internally.
set "GOFLAGS=-buildvcs=false"

REM --- Probe go.exe; if not in PATH, prepend the first known install dir.
REM     Flat goto chain (NOT nested inside `if errorlevel`) because cmd
REM     mis-parses `if exist "<path with spaces>" set "PATH=...;%PATH%"`
REM     when written inside a parenthesized block.
where go >nul 2>&1
if not errorlevel 1 goto :go_ready
if exist "C:\Go\bin\go.exe" (
    set "PATH=C:\Go\bin;!PATH!"
    goto :go_ready
)
if exist "C:\Program Files\Go\bin\go.exe" (
    set "PATH=C:\Program Files\Go\bin;!PATH!"
    goto :go_ready
)
:go_ready

REM --- Probe wails.exe in GOPATH/bin (go install drops it there).
where wails >nul 2>&1
if not errorlevel 1 goto :wails_ready
if exist "%USERPROFILE%\go\bin\wails.exe" (
    set "PATH=%USERPROFILE%\go\bin;!PATH!"
    goto :wails_ready
)
:wails_ready

REM --- Probe pnpm in the standard npm-global / pnpm-standalone locations.
where pnpm >nul 2>&1
if not errorlevel 1 goto :pnpm_ready
if exist "%APPDATA%\npm\pnpm.cmd" (
    set "PATH=%APPDATA%\npm;!PATH!"
    goto :pnpm_ready
)
if exist "%LOCALAPPDATA%\pnpm\pnpm.exe" (
    set "PATH=%LOCALAPPDATA%\pnpm;!PATH!"
    goto :pnpm_ready
)
:pnpm_ready

echo.
echo ============================================================
echo  Reasonix Desktop Builder
echo ============================================================
echo.

where go >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Go not found in PATH. Please install Go first.
    echo         https://go.dev/dl/
    pause
    exit /b 1
)
for /f "delims=" %%G in ('where go') do echo   using go:    %%G

where wails >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Wails CLI not found.
    echo         Install with: go install github.com/wailsapp/wails/v2/cmd/wails@latest
    pause
    exit /b 1
)
for /f "delims=" %%G in ('where wails') do echo   using wails: %%G

where pnpm >nul 2>&1
if errorlevel 1 (
    echo [ERROR] pnpm not found.
    echo         Install with: npm install -g pnpm
    pause
    exit /b 1
)
for /f "delims=" %%G in ('where pnpm') do echo   using pnpm:  %%G

set "BUILD_START=%TIME%"

echo.
echo [1/3] Frontend dependencies...
cd frontend
call pnpm install >nul 2>&1
if errorlevel 1 (
    echo   WARNING: pnpm install had issues (non-fatal, continuing)
)
cd ..

echo [2/3] Building desktop binary (GOFLAGS=%GOFLAGS%)...
wails build 2>&1
if errorlevel 1 (
    echo.
    echo [ERROR] Build failed! See log above.
    pause
    exit /b 1
)

set "BUILD_END=%TIME%"

echo [3/3] Checking output...
set "EXE=build\bin\reasonix-desktop.exe"
if exist "%EXE%" (
    for %%A in ("%EXE%") do (
        echo.
        echo ============================================================
        echo  BUILD SUCCESS
        echo ============================================================
        echo  Output:  %CD%\%EXE%
        echo  Size:    %%~zA bytes
        echo  Time:    %%~tA
        echo  Started: %BUILD_START%
        echo  Ended:   %BUILD_END%
        echo ============================================================
    )
) else (
    echo [ERROR] Output not found: %EXE%
    pause
    exit /b 1
)

echo.
set /p RUN_REPLACE="Run replace-reasonix.bat now to install? [Y/N]: "
if /i "!RUN_REPLACE!"=="Y" (
    echo.
    call "%~dp0replace-reasonix.bat"
) else (
    echo Skipped install. Run replace-reasonix.bat manually when ready.
    pause
)

endlocal