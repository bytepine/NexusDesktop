@echo off
setlocal enabledelayedexpansion
chcp 65001 >nul 2>&1

echo ============================================
echo   NexusDesktop - Build (Windows)
echo ============================================
echo.

cd /d "%~dp0"

:: ── 0. 确保 Go 在 PATH ────────────────────────────────────
where go >nul 2>&1
if %ERRORLEVEL% equ 0 goto go_ok

if defined GOROOT (
    if exist "%GOROOT%\bin\go.exe" (
        set "PATH=%GOROOT%\bin;%PATH%"
        goto go_ok
    )
)
if exist "C:\tools\go\bin\go.exe" (
    set "PATH=C:\tools\go\bin;%PATH%"
    goto go_ok
)
if exist "C:\Go\bin\go.exe" (
    set "PATH=C:\Go\bin;%PATH%"
    goto go_ok
)
if exist "C:\Program Files\Go\bin\go.exe" (
    set "PATH=C:\Program Files\Go\bin;%PATH%"
    goto go_ok
)
if exist "%USERPROFILE%\go\bin\go.exe" (
    set "PATH=%USERPROFILE%\go\bin;%PATH%"
    goto go_ok
)
if exist "%LOCALAPPDATA%\Programs\Go\bin\go.exe" (
    set "PATH=%LOCALAPPDATA%\Programs\Go\bin;%PATH%"
    goto go_ok
)

echo [ERROR] 找不到 go 命令。
echo         请安装 Go 1.24+：https://go.dev/dl/
echo         或将 Go\bin 路径加入系统 PATH。
pause
exit /b 1

:go_ok
echo [Go] %PATH:~0,0%%GOROOT% (from PATH)
for /f "tokens=*" %%v in ('go version 2^>nul') do echo [Go] %%v
echo.

:: ── 1. 读取当前版本 ──────────────────────────────────────
set /p VERSION=<VERSION
set "VERSION=%VERSION: =%"
echo Version: %VERSION%
echo.

:: ── 2. 确保 GCC 14 在 PATH（Fyne/CGO 必须）────────────────
where gcc >nul 2>&1
if %ERRORLEVEL% equ 0 goto gcc_ok

if defined W64DEVKIT (
    if exist "%W64DEVKIT%\bin\gcc.exe" (
        set "PATH=%W64DEVKIT%\bin;%PATH%"
        echo [GCC] %W64DEVKIT%\bin
        goto gcc_ok
    )
)
if exist "C:\tools\w64devkit-old\w64devkit\bin\gcc.exe" (
    set "PATH=C:\tools\w64devkit-old\w64devkit\bin;%PATH%"
    echo [GCC] C:\tools\w64devkit-old\w64devkit\bin
    goto gcc_ok
)
if exist "C:\w64devkit\w64devkit\bin\gcc.exe" (
    set "PATH=C:\w64devkit\w64devkit\bin;%PATH%"
    echo [GCC] C:\w64devkit\w64devkit\bin
    goto gcc_ok
)
if exist "C:\tools\w64devkit\bin\gcc.exe" (
    set "PATH=C:\tools\w64devkit\bin;%PATH%"
    echo [GCC] C:\tools\w64devkit\bin
    goto gcc_ok
)
if exist "C:\w64devkit\bin\gcc.exe" (
    set "PATH=C:\w64devkit\bin;%PATH%"
    echo [GCC] C:\w64devkit\bin
    goto gcc_ok
)

echo [WARN] 未找到 GCC！Fyne 需要 CGO。
echo        请安装 w64devkit v1.23.0 (GCC 14)：
echo        https://github.com/skeeto/w64devkit/releases/tag/v1.23.0
echo        解压后将 bin 路径写入环境变量 W64DEVKIT，或加入系统 PATH。
echo.
pause
exit /b 1

:gcc_ok
echo.

:: ── 3. 调用 Python 打包脚本 ──────────────────────────────
echo [1/2] Building NexusDesktop (version: %VERSION%)...
echo       (首次构建需下载依赖，约需 1-5 分钟，请耐心等待...)
echo.
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
