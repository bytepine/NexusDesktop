"""
build_desktop.py — NexusDesktop 跨平台构建脚本

用法:
    python scripts/build_desktop.py [--version <版本号>] [--output <输出目录>]

说明:
    1. 读取 VERSION 文件确定版本号（--version 可覆盖）
    2. 检测当前平台并设置对应构建参数
    3. go build，通过 -X main.appVersion 注入版本号
    4. Windows 产物：NexusDesktop-windows-amd64.exe
       macOS   产物：NexusDesktop-darwin-<arch>（可选 fyne package 为 .app）
       Linux   产物：NexusDesktop-linux-amd64
    5. 复制到 --output 目录（默认 release/）

平台要求：
    Windows : GCC 14.x（如 w64devkit v1.23.0）。
              GCC 16+(binutils 2.46+) 产生 BigOBJ 对象文件，Go CGO 暂不支持；
              推荐将 w64devkit/bin 加入 PATH，或通过环境变量 W64DEVKIT 指定根目录。
    macOS   : Xcode Command Line Tools（xcode-select --install）
    Linux   : build-essential（sudo apt install build-essential）
"""

from __future__ import annotations

import argparse
import os
import platform
import shutil
import subprocess
import sys

if sys.stdout.encoding and sys.stdout.encoding.lower() != "utf-8":
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
if sys.stderr.encoding and sys.stderr.encoding.lower() != "utf-8":
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")


def repo_root() -> str:
    return os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def read_version(root: str) -> str:
    vpath = os.path.join(root, "VERSION")
    if not os.path.isfile(vpath):
        raise FileNotFoundError(f"找不到 VERSION 文件: {vpath}")
    return open(vpath, encoding="utf-8").read().strip()


def find_go() -> str:
    """返回 go 可执行路径；优先 GOROOT，其次 PATH，最后探测常见安装目录。"""
    exe_name = "go.exe" if sys.platform == "win32" else "go"

    goroot = os.environ.get("GOROOT")
    if goroot:
        exe = os.path.join(goroot, "bin", exe_name)
        if os.path.isfile(exe):
            return exe

    found = shutil.which("go")
    if found:
        return found

    if sys.platform == "win32":
        home = os.environ.get("USERPROFILE", "")
        local = os.environ.get("LOCALAPPDATA", "")
        candidates = [
            r"C:\tools\go\bin",
            r"C:\Go\bin",
            r"C:\Program Files\Go\bin",
            os.path.join(home, "go", "bin"),
            os.path.join(local, "Programs", "Go", "bin"),
        ]
    else:
        home = os.path.expanduser("~")
        candidates = [
            "/usr/local/go/bin",
            "/usr/local/bin",
            os.path.join(home, "go", "bin"),
            os.path.join(home, ".local", "go", "bin"),
        ]

    for d in candidates:
        exe = os.path.join(d, exe_name)
        if os.path.isfile(exe):
            # 将该目录加入 PATH，后续子进程也能找到
            os.environ["PATH"] = d + os.pathsep + os.environ.get("PATH", "")
            return exe

    raise FileNotFoundError(
        "找不到 go 命令。请安装 Go 1.24+：https://go.dev/dl/\n"
        "或设置 GOROOT 环境变量指向 Go 安装目录。"
    )


def _prepend_w64devkit(env: dict) -> None:
    """Windows：检测 w64devkit v1.x（GCC 14）并将其 bin 目录加到 PATH 最前。"""
    if sys.platform != "win32":
        return

    # 环境变量 W64DEVKIT 指向 w64devkit 根目录（含 bin/gcc.exe 的上级）
    w64root = os.environ.get("W64DEVKIT", "")
    candidates = []
    if w64root:
        candidates.append(os.path.join(w64root, "bin"))
    # 常见默认安装位置
    for d in [
        r"C:\tools\w64devkit-old\w64devkit\bin",
        r"C:\w64devkit\w64devkit\bin",
        r"C:\tools\w64devkit\bin",
        r"C:\w64devkit\bin",
    ]:
        candidates.append(d)

    for d in candidates:
        gcc_path = os.path.join(d, "gcc.exe")
        if os.path.isfile(gcc_path):
            env["PATH"] = d + os.pathsep + env.get("PATH", "")
            print(f"[build] 使用 GCC: {gcc_path}")
            return

    # 已在 PATH 中有 gcc 则直接使用
    if shutil.which("gcc"):
        print("[build] 使用 PATH 中已有的 gcc")
        return

    print(
        "[WARN] 未找到 GCC！Fyne 需要 CGO。\n"
        "       建议安装 w64devkit v1.23.0（GCC 14）：\n"
        "       https://github.com/skeeto/w64devkit/releases/tag/v1.23.0\n"
        "       解压后将 bin 目录路径设置到环境变量 W64DEVKIT，\n"
        "       或将 w64devkit/bin 加入系统 PATH。",
        file=sys.stderr,
    )


def build_desktop(version: str, output_dir: str) -> str:
    root = repo_root()
    go = find_go()

    env = os.environ.copy()
    env["CGO_ENABLED"] = "1"

    system = platform.system()
    machine = platform.machine().lower()

    if system == "Windows":
        _prepend_w64devkit(env)
        arch = "amd64"
        out_name = f"NexusDesktop-windows-{arch}.exe"
        ldflags = f"-H=windowsgui -s -w -X main.appVersion={version}"
        goos, goarch = "windows", arch
    elif system == "Darwin":
        # Apple Silicon → arm64；Intel → amd64
        arch = "arm64" if "arm" in machine or "aarch" in machine else "amd64"
        out_name = f"NexusDesktop-darwin-{arch}"
        ldflags = f"-s -w -X main.appVersion={version}"
        goos, goarch = "darwin", arch
    elif system == "Linux":
        arch = "arm64" if "aarch" in machine else "amd64"
        out_name = f"NexusDesktop-linux-{arch}"
        ldflags = f"-s -w -X main.appVersion={version}"
        goos, goarch = "linux", arch
    else:
        raise RuntimeError(f"不支持的平台: {system}")

    os.makedirs(output_dir, exist_ok=True)
    out_path = os.path.join(output_dir, out_name)

    cmd = [
        go, "build",
        "-ldflags", ldflags,
        "-o", out_path,
        "./cmd/nexusdesktop/",
    ]

    build_env = env.copy()
    build_env["GOOS"] = goos
    build_env["GOARCH"] = goarch

    print(f"[build] go build v{version} → {out_path}")
    print(f"        GOOS={goos} GOARCH={goarch} CGO_ENABLED=1")

    result = subprocess.run(cmd, cwd=root, env=build_env)
    if result.returncode != 0:
        raise RuntimeError(f"go build 失败（返回码 {result.returncode}）")

    size_mb = os.path.getsize(out_path) / (1024 * 1024)
    print(f"[build] 产物大小：{size_mb:.1f} MB")

    # macOS：可选封装为 .app bundle
    if system == "Darwin":
        app_path = _package_macos_app(root, out_path, version, output_dir, arch)
        if app_path:
            return app_path

    return out_path


def _package_macos_app(
    root: str, binary: str, version: str, output_dir: str, arch: str
) -> str | None:
    """将 macOS 二进制封装为 .app bundle 并 zip。"""
    app_dir = os.path.join(output_dir, "NexusDesktop.app")
    macos_dir = os.path.join(app_dir, "Contents", "MacOS")
    res_dir = os.path.join(app_dir, "Contents", "Resources")
    os.makedirs(macos_dir, exist_ok=True)
    os.makedirs(res_dir, exist_ok=True)

    # 复制可执行文件
    shutil.copy2(binary, os.path.join(macos_dir, "NexusDesktop"))
    os.chmod(os.path.join(macos_dir, "NexusDesktop"), 0o755)

    # 写入 Info.plist
    plist = f"""<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key><string>NexusDesktop</string>
  <key>CFBundleIdentifier</key><string>com.bytepine.nexusdesktop</string>
  <key>CFBundleName</key><string>NexusDesktop</string>
  <key>CFBundleShortVersionString</key><string>{version}</string>
  <key>CFBundleVersion</key><string>{version}</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
"""
    with open(os.path.join(app_dir, "Contents", "Info.plist"), "w", encoding="utf-8") as f:
        f.write(plist)

    # 打 zip
    zip_name = f"NexusDesktop-darwin-{arch}.app.zip"
    zip_path = os.path.join(output_dir, zip_name)
    result = subprocess.run(
        ["zip", "-r", zip_path, "NexusDesktop.app"],
        cwd=output_dir,
    )
    if result.returncode != 0:
        print("[WARN] zip 失败，跳过 .app 封装", file=sys.stderr)
        return None

    shutil.rmtree(app_dir)
    os.remove(binary)
    print(f"[build] .app 封装完成：{zip_path}")
    return zip_path


def main() -> int:
    parser = argparse.ArgumentParser(description="构建 NexusDesktop 跨平台二进制")
    parser.add_argument("--version", default=None, help="版本号，默认读取 VERSION 文件")
    parser.add_argument("--output", default=None, help="输出目录，默认 <repo>/release/")
    args = parser.parse_args()

    root = repo_root()
    output_dir = args.output or os.path.join(root, "release")

    try:
        version = args.version or read_version(root)
        path = build_desktop(version, output_dir)
    except Exception as e:
        print(f"[ERROR] {e}", file=sys.stderr)
        return 1

    print(f"[OK] {path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
