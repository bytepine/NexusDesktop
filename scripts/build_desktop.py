"""
build_desktop.py — NexusDesktop 跨平台构建脚本

用法:
    python scripts/build_desktop.py [--version <版本号>] [--output <目录>]
                                    [--build-type develop|release]

构建类型：
    develop（默认）
        - 日志级别 debug（所有日志可见）
        - Windows 保留控制台窗口（便于查看实时日志）
        - 不裁剪符号（方便 panic 堆栈阅读）
        - Windows 产物：NexusDesktop-windows-amd64-v<ver>-dev.zip（内含 NexusDesktop-dev.exe）

    release
        - 日志级别 info（debug 日志不输出）
        - Windows 隐藏控制台窗口（-H=windowsgui）
        - -s -w 裁剪符号，减小体积
        - Windows 产物：NexusDesktop-windows-amd64-v<ver>.zip（内含 NexusDesktop.exe）

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


_LOG_PKG = "github.com/bytepine/NexusDesktop/internal/log"


def _create_ico(png_path: str, ico_path: str) -> bool:
    """将 PNG 封装为 ICO（嵌入原始 PNG 数据，Windows Vista+ 原生支持）。纯 Python，无需 PIL。"""
    import struct
    try:
        with open(png_path, "rb") as f:
            png_data = f.read()
        # ICO 头 + 1 条目目录 + PNG 数据
        # 目录条目中 width/height=0 代表 256
        data_offset = 6 + 16
        header = struct.pack("<HHH", 0, 1, 1)
        dir_entry = struct.pack("<BBBBHHII", 0, 0, 0, 0, 1, 32, len(png_data), data_offset)
        with open(ico_path, "wb") as f:
            f.write(header + dir_entry + png_data)
        return True
    except Exception as e:
        print(f"[WARN] ICO 生成失败: {e}", file=sys.stderr)
        return False


def _embed_windows_icon(root: str, go: str, env: dict) -> str | None:
    """
    在 cmd/nexusdesktop/ 生成 resource.syso，将品牌图标嵌入 Windows exe。
    返回 syso 路径（成功）或 None（失败，构建继续但无自定义图标）。
    """
    icon_png = os.path.join(root, "assets", "icon.png")
    icon_ico = os.path.join(root, "assets", "icon.ico")
    syso_path = os.path.join(root, "cmd", "nexusdesktop", "resource.syso")

    if not os.path.isfile(icon_png):
        print("[WARN] assets/icon.png 不存在，跳过图标嵌入", file=sys.stderr)
        return None

    # 1. PNG → ICO
    if not _create_ico(icon_png, icon_ico):
        return None
    print(f"[icon] ICO 已生成: {icon_ico}")

    # 2. 安装 rsrc（Go 工具，生成 .syso Windows 资源文件）
    print("[icon] 安装 rsrc 工具...")
    subprocess.run(
        [go, "install", "github.com/akavel/rsrc@latest"],
        env=env, check=False,
    )

    # 3. 查找 rsrc.exe（go install 放到 GOPATH/bin 或 GOENV bin）
    gopath = subprocess.run(
        [go, "env", "GOPATH"], env=env, capture_output=True, text=True
    ).stdout.strip()
    rsrc_exe = os.path.join(gopath, "bin", "rsrc.exe")
    if not os.path.isfile(rsrc_exe):
        rsrc_exe = shutil.which("rsrc") or ""
    if not rsrc_exe or not os.path.isfile(rsrc_exe):
        print("[WARN] rsrc 未找到，跳过图标嵌入", file=sys.stderr)
        return None

    # 4. 生成 resource.syso
    result = subprocess.run(
        [rsrc_exe, "-ico", icon_ico, "-o", syso_path],
        env=env,
    )
    if result.returncode != 0 or not os.path.isfile(syso_path):
        print("[WARN] rsrc 执行失败，跳过图标嵌入", file=sys.stderr)
        return None

    print(f"[icon] resource.syso 已生成: {syso_path}")
    return syso_path


def _go_build_binary(
    go: str, env: dict, root: str,
    goos: str, goarch: str,
    out_path: str, ldflags: str,
) -> None:
    """编译单一 GOOS/GOARCH 二进制；失败抛 RuntimeError。"""
    build_env = env.copy()
    build_env["GOOS"] = goos
    build_env["GOARCH"] = goarch
    print(f"[build] go build {goos}/{goarch} → {out_path}")
    print(f"        CGO_ENABLED=1 ldflags={ldflags.strip()!r}")
    result = subprocess.run(
        [go, "build", "-ldflags", ldflags.strip(), "-o", out_path,
         "./cmd/nexusdesktop/"],
        cwd=root, env=build_env,
    )
    if result.returncode != 0:
        raise RuntimeError(f"go build {goos}/{goarch} 失败（返回码 {result.returncode}）")


def _lipo_merge(arm64_bin: str, amd64_bin: str, out_path: str) -> None:
    """用 lipo 将 arm64 + amd64 合并为 Universal Binary；失败抛 RuntimeError。"""
    if not shutil.which("lipo"):
        raise RuntimeError("未找到 lipo，请安装 Xcode Command Line Tools")
    result = subprocess.run(
        ["lipo", "-create", "-output", out_path, arm64_bin, amd64_bin]
    )
    if result.returncode != 0:
        raise RuntimeError("lipo 合并失败")
    size_mb = os.path.getsize(out_path) / (1024 * 1024)
    print(f"[build] Universal Binary：{size_mb:.1f} MB → {out_path}")


def build_desktop(
    version: str,
    output_dir: str,
    build_type: str = "develop",
    arch_target: str = "auto",
) -> str:
    """
    build_type : "develop" | "release"
    arch_target: macOS 专用 — "universal"（默认）| "arm64" | "amd64"
                 其他平台忽略此参数。
    """
    is_release = build_type == "release"
    log_level = "info" if is_release else "debug"
    strip_flags = "-s -w " if is_release else ""

    root = repo_root()
    go = find_go()

    env = os.environ.copy()
    env["CGO_ENABLED"] = "1"

    system = platform.system()
    machine = platform.machine().lower()
    os.makedirs(output_dir, exist_ok=True)

    if system == "Windows":
        import zipfile
        _prepend_w64devkit(env)
        arch = "amd64"
        # exe 固定名称（不含版本号）
        exe_name = "NexusDesktop.exe" if is_release else "NexusDesktop-dev.exe"
        win_flags = "-H=windowsgui " if is_release else ""
        ldflags = (
            f"{win_flags}{strip_flags}"
            f"-X main.appVersion={version} "
            f"-X {_LOG_PKG}.Level={log_level}"
        )
        exe_path = os.path.join(output_dir, exe_name)
        syso_path = _embed_windows_icon(root, go, env)
        try:
            _go_build_binary(go, env, root, "windows", arch, exe_path, ldflags)
        finally:
            if syso_path and os.path.isfile(syso_path):
                os.remove(syso_path)
                print(f"[icon] 已清理: {syso_path}")
        size_mb = os.path.getsize(exe_path) / (1024 * 1024)
        print(f"[build] 产物大小：{size_mb:.1f} MB")
        # 打包 zip：版本号写在 zip 文件名中，exe 保持固定名称
        suffix = "" if is_release else "-dev"
        zip_name = f"NexusDesktop-windows-{arch}-v{version}{suffix}.zip"
        zip_path = os.path.join(output_dir, zip_name)
        with zipfile.ZipFile(zip_path, "w", zipfile.ZIP_DEFLATED) as zf:
            zf.write(exe_path, exe_name)
        os.remove(exe_path)
        zip_mb = os.path.getsize(zip_path) / (1024 * 1024)
        print(f"[build] ZIP 产物：{zip_mb:.1f} MB → {zip_path}")
        return zip_path

    elif system == "Darwin":
        suffix = "" if is_release else "-dev"
        ldflags = (
            f"{strip_flags}"
            f"-X main.appVersion={version} "
            f"-X {_LOG_PKG}.Level={log_level}"
        )

        # 决定目标架构
        if arch_target in ("auto", "universal"):
            target_arch = "universal"
        elif arch_target in ("arm64", "amd64"):
            target_arch = arch_target
        else:
            raise RuntimeError(f"不支持的 arch_target: {arch_target}")

        if target_arch == "universal":
            # 分别编译 arm64 / amd64，再 lipo 合并
            arm64_tmp = os.path.join(output_dir, f"_nexusdesktop-arm64{suffix}")
            amd64_tmp = os.path.join(output_dir, f"_nexusdesktop-amd64{suffix}")
            _go_build_binary(go, env, root, "darwin", "arm64", arm64_tmp, ldflags)
            _go_build_binary(go, env, root, "darwin", "amd64", amd64_tmp, ldflags)
            out_path = os.path.join(output_dir, f"NexusDesktop-darwin-universal{suffix}")
            _lipo_merge(arm64_tmp, amd64_tmp, out_path)
            os.remove(arm64_tmp)
            os.remove(amd64_tmp)
        else:
            out_path = os.path.join(output_dir, f"NexusDesktop-darwin-{target_arch}{suffix}")
            _go_build_binary(go, env, root, "darwin", target_arch, out_path, ldflags)
            size_mb = os.path.getsize(out_path) / (1024 * 1024)
            print(f"[build] 产物大小：{size_mb:.1f} MB")

        app_path = _package_macos_app(root, out_path, version, output_dir, target_arch)
        return app_path if app_path else out_path

    elif system == "Linux":
        arch = "arm64" if "aarch" in machine else "amd64"
        suffix = "" if is_release else "-dev"
        out_name = f"NexusDesktop-linux-{arch}{suffix}"
        ldflags = (
            f"{strip_flags}"
            f"-X main.appVersion={version} "
            f"-X {_LOG_PKG}.Level={log_level}"
        )
        out_path = os.path.join(output_dir, out_name)
        _go_build_binary(go, env, root, "linux", arch, out_path, ldflags)
        size_mb = os.path.getsize(out_path) / (1024 * 1024)
        print(f"[build] 产物大小：{size_mb:.1f} MB")
        return out_path

    else:
        raise RuntimeError(f"不支持的平台: {system}")


def _create_icns(png_path: str, icns_path: str) -> bool:
    """
    用 macOS 内置 sips + iconutil 将 PNG 转为 .icns。
    需要 Xcode CLI；失败时返回 False，构建继续。
    """
    if not shutil.which("sips") or not shutil.which("iconutil"):
        print("[WARN] sips/iconutil 未找到，跳过 .icns 生成", file=sys.stderr)
        return False

    iconset_dir = icns_path.replace(".icns", ".iconset")
    os.makedirs(iconset_dir, exist_ok=True)

    # macOS 标准图标尺寸：(尺寸, 文件名)
    sizes = [
        (16,   "icon_16x16.png"),
        (32,   "icon_16x16@2x.png"),
        (32,   "icon_32x32.png"),
        (64,   "icon_32x32@2x.png"),
        (128,  "icon_128x128.png"),
        (256,  "icon_128x128@2x.png"),
        (256,  "icon_256x256.png"),
        (512,  "icon_256x256@2x.png"),
        (512,  "icon_512x512.png"),
        (1024, "icon_512x512@2x.png"),
    ]
    for size, name in sizes:
        result = subprocess.run(
            ["sips", "-z", str(size), str(size), png_path, "--out",
             os.path.join(iconset_dir, name)],
            capture_output=True,
        )
        if result.returncode != 0:
            print(f"[WARN] sips 缩放 {size}x{size} 失败", file=sys.stderr)
            shutil.rmtree(iconset_dir, ignore_errors=True)
            return False

    result = subprocess.run(
        ["iconutil", "-c", "icns", iconset_dir, "-o", icns_path],
        capture_output=True,
    )
    shutil.rmtree(iconset_dir, ignore_errors=True)

    if result.returncode != 0 or not os.path.isfile(icns_path):
        print("[WARN] iconutil 失败，跳过 .icns 生成", file=sys.stderr)
        return False

    print(f"[icon] .icns 已生成: {icns_path}")
    return True


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

    # PNG → .icns，放入 Resources
    icon_png = os.path.join(root, "assets", "icon.png")
    icon_name = "AppIcon"
    icns_path = os.path.join(res_dir, f"{icon_name}.icns")
    has_icon = os.path.isfile(icon_png) and _create_icns(icon_png, icns_path)
    icon_plist_entry = (
        f"  <key>CFBundleIconFile</key><string>{icon_name}</string>\n"
        if has_icon else ""
    )

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
{icon_plist_entry}  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
"""
    with open(os.path.join(app_dir, "Contents", "Info.plist"), "w", encoding="utf-8") as f:
        f.write(plist)

    # 打 DMG（含 Applications 快捷方式，支持拖拽安装）
    dmg_name = f"NexusDesktop-darwin-{arch}.dmg"
    dmg_path = os.path.join(output_dir, dmg_name)

    # staging 目录：.app + /Applications 软链
    staging_dir = os.path.join(output_dir, "_dmg_staging")
    if os.path.exists(staging_dir):
        shutil.rmtree(staging_dir)
    os.makedirs(staging_dir)
    shutil.copytree(app_dir, os.path.join(staging_dir, "NexusDesktop.app"),
                    symlinks=True)
    os.symlink("/Applications", os.path.join(staging_dir, "Applications"))

    # 删除旧 DMG（hdiutil -ov 覆盖有时有 bug，先手动删）
    if os.path.exists(dmg_path):
        os.remove(dmg_path)

    result = subprocess.run(
        [
            "hdiutil", "create",
            "-volname", "NexusDesktop",
            "-srcfolder", staging_dir,
            "-ov",
            "-format", "UDZO",
            dmg_path,
        ],
    )
    shutil.rmtree(staging_dir)
    shutil.rmtree(app_dir)
    os.remove(binary)

    if result.returncode != 0:
        print("[WARN] hdiutil 失败，跳过 DMG 封装", file=sys.stderr)
        return None

    print(f"[build] DMG 封装完成：{dmg_path}")
    return dmg_path


def main() -> int:
    parser = argparse.ArgumentParser(description="构建 NexusDesktop 跨平台二进制")
    parser.add_argument("--version", default=None, help="版本号，默认读取 VERSION 文件")
    parser.add_argument("--output", default=None, help="输出目录，默认 <repo>/release/")
    parser.add_argument(
        "--build-type",
        default="develop",
        choices=["develop", "release"],
        help="构建类型：develop（默认，debug 日志）或 release（info 日志，裁剪符号）",
    )
    parser.add_argument(
        "--arch",
        default="universal",
        choices=["universal", "arm64", "amd64"],
        help="macOS 目标架构：universal（默认，arm64+amd64）| arm64 | amd64",
    )
    args = parser.parse_args()

    root = repo_root()
    output_dir = args.output or os.path.join(root, "release")

    try:
        version = args.version or read_version(root)
        path = build_desktop(version, output_dir, args.build_type, args.arch)
    except Exception as e:
        print(f"[ERROR] {e}", file=sys.stderr)
        return 1

    print(f"[OK] {path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
