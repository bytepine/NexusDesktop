@echo off
setlocal enabledelayedexpansion
chcp 65001 >nul 2>&1

echo ============================================
echo   NexusDesktop - Beta Build (Windows)
echo ============================================
echo.

cd /d "%~dp0"

:: ── 1. 读取当前版本，自动派生 beta 版本号 ────────────────
set /p CURRENT_VERSION=<VERSION
set CURRENT_VERSION=%CURRENT_VERSION: =%
echo Current VERSION : %CURRENT_VERSION%

for /f "tokens=1,2,3 delims=." %%a in ("%CURRENT_VERSION%") do (
    set MAJOR=%%a
    set MINOR=%%b
    set PATCH=%%c
)
for /f "tokens=1 delims=-" %%x in ("%PATCH%") do set PATCH=%%x

set /a NEXT_PATCH=%PATCH%+1
set NEXT_VERSION=%MAJOR%.%MINOR%.%NEXT_PATCH%-beta

echo Next beta version: %NEXT_VERSION%
echo.

:: ── 2. 确保 GCC 14 在 PATH ────────────────────────────────
if defined W64DEVKIT (
    set GCC_BIN=%W64DEVKIT%\bin
    goto gcc_check
)
for %%d in (
    "C:\tools\w64devkit-old\w64devkit\bin"
    "C:\w64devkit\w64devkit\bin"
    "C:\tools\w64devkit\bin"
    "C:\w64devkit\bin"
) do (
    if exist "%%~d\gcc.exe" (
        set GCC_BIN=%%~d
        goto gcc_check
    )
)
where gcc >nul 2>&1
if %ERRORLEVEL% equ 0 goto build

echo [WARN] 未找到 GCC！Fyne 需要 CGO。
echo        请安装 w64devkit v1.23.0 (GCC 14)：
echo        https://github.com/skeeto/w64devkit/releases/tag/v1.23.0
pause
exit /b 1

:gcc_check
set PATH=%GCC_BIN%;%PATH%
echo [GCC] %GCC_BIN%
echo.

:build
:: ── 3. 调用 Python 打包脚本 ──────────────────────────────
echo [1/2] Building NexusDesktop (version: %NEXT_VERSION%)...
python scripts\build_desktop.py --version %NEXT_VERSION%
if %ERRORLEVEL% neq 0 (
    echo.
    echo [FAILED] Build failed! See output above for details.
    pause
    exit /b 1
)

echo.
echo [2/2] Build successful!
echo.
echo Output: %cd%\release\NexusDesktop-windows-amd64.exe
echo.
pause
