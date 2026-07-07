@echo off
setlocal enabledelayedexpansion
chcp 65001 >nul 2>&1

echo ============================================
echo   NexusDesktop - Build (Windows)
echo ============================================
echo.

cd /d "%~dp0"

:: ── 1. 读取当前版本 ──────────────────────────────────────
set /p VERSION=<VERSION
set VERSION=%VERSION: =%
echo Version: %VERSION%
echo.

:: ── 2. 确保 GCC 14 在 PATH（Fyne/CGO 必须）────────────────
::    优先使用环境变量 W64DEVKIT，否则检查常见安装位置
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
:: PATH 中已有 gcc 则直接继续
where gcc >nul 2>&1
if %ERRORLEVEL% equ 0 goto build

echo [WARN] 未找到 GCC！Fyne 需要 CGO。
echo        请安装 w64devkit v1.23.0 (GCC 14)：
echo        https://github.com/skeeto/w64devkit/releases/tag/v1.23.0
echo        解压后将 bin 路径写入环境变量 W64DEVKIT，或加入系统 PATH。
echo.
pause
exit /b 1

:gcc_check
set PATH=%GCC_BIN%;%PATH%
echo [GCC] %GCC_BIN%
echo.

:build
:: ── 3. 调用 Python 打包脚本 ──────────────────────────────
echo [1/2] Building NexusDesktop (version: %VERSION%)...
python scripts\build_desktop.py --version %VERSION%
if %ERRORLEVEL% neq 0 (
    echo.
    echo [FAILED] Build failed! See output above for details.
    pause
    exit /b 1
)

:: ── 4. 显示产物路径 ──────────────────────────────────────
echo.
echo [2/2] Build successful!
echo.
echo Output: %cd%\release\NexusDesktop-windows-amd64.exe
echo.
pause
