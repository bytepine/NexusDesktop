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
        - Windows 产物：NexusDesktop-dev.exe（不打包 zip）

    release
        - 日志级别 info（debug 日志不输出）
        - Windows 隐藏控制台窗口（-H=windowsgui）
        - -s -w 裁剪符号，减小体积
        - Windows 产物：NexusDesktop-windows-amd64-v<ver>-setup.exe（安装包，需 Inno Setup 7）
                       + NexusDesktop-windows-amd64-v<ver>-update.zip（更新包，内含 exe）
        - macOS 产物：NexusDesktop-darwin-arm64.dmg（安装包，仅 Apple Silicon）
                     + NexusDesktop-darwin-arm64-v<ver>-update.zip（更新包，内含 .app）

平台要求：
    Windows : GCC 14.x（如 w64devkit v1.23.0）。
              GCC 16+(binutils 2.46+) 产生 BigOBJ 对象文件，Go CGO 暂不支持；
              推荐将 w64devkit/bin 加入 PATH，或通过环境变量 W64DEVKIT 指定根目录。
              release 安装包另需 Inno Setup 7（ISCC.exe）；CI 会自动安装。
              本机：https://jrsoftware.org/isdl.php 或 winget install JRSoftware.InnoSetup.7
    macOS   : Xcode Command Line Tools（xcode-select --install）
    Linux   : build-essential（sudo apt install build-essential）
"""

from __future__ import annotations

import argparse
import json
import os
import platform
import shutil
import subprocess
import sys
import zipfile

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


def _version_quad(version: str) -> tuple[int, int, int, int]:
    """把 VERSION / tag 解析为 Windows FileVersion 四元组。"""
    v = version.strip()
    if not v or v.lower() == "dev":
        return (0, 0, 0, 0)
    if v[:1] in "vV":
        v = v[1:]
    for sep in "-+":
        i = v.find(sep)
        if i >= 0:
            v = v[:i]
            break
    parts: list[int] = []
    for s in v.split("."):
        if s.isdigit():
            parts.append(int(s))
        else:
            break
        if len(parts) == 4:
            break
    while len(parts) < 4:
        parts.append(0)
    return (parts[0], parts[1], parts[2], parts[3])


def _embed_windows_resources(root: str, go: str, env: dict, version: str) -> str | None:
    """
    在 cmd/nexusdesktop/ 生成 resource.syso：嵌入图标 + FileVersion。
    rsrc 不支持 VERSIONINFO，改用 goversioninfo。失败时构建继续但无资源。
    """
    icon_png = os.path.join(root, "assets", "icon.png")
    icon_ico = os.path.join(root, "assets", "icon.ico")
    cmd_dir = os.path.join(root, "cmd", "nexusdesktop")
    syso_path = os.path.join(cmd_dir, "resource.syso")
    vi_path = os.path.join(cmd_dir, "versioninfo.json")

    if not os.path.isfile(icon_png):
        print("[WARN] assets/icon.png 不存在，跳过图标/版本嵌入", file=sys.stderr)
        return None

    if not _create_ico(icon_png, icon_ico):
        return None
    print(f"[icon] ICO 已生成: {icon_ico}")

    print("[icon] 安装 goversioninfo...")
    subprocess.run(
        [go, "install", "github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest"],
        env=env, check=False,
    )

    gobin = subprocess.run(
        [go, "env", "GOBIN"], env=env, capture_output=True, text=True
    ).stdout.strip()
    gopath = subprocess.run(
        [go, "env", "GOPATH"], env=env, capture_output=True, text=True
    ).stdout.strip()
    gvi = ""
    lookup: list[str] = []
    if gobin:
        lookup.append(os.path.join(gobin, "goversioninfo.exe"))
        lookup.append(os.path.join(gobin, "goversioninfo"))
    for gp in gopath.split(os.pathsep):
        lookup.append(os.path.join(gp, "bin", "goversioninfo.exe"))
        lookup.append(os.path.join(gp, "bin", "goversioninfo"))
    which = shutil.which("goversioninfo")
    if which:
        lookup.append(which)
    for cand in lookup:
        if cand and os.path.isfile(cand):
            gvi = cand
            break
    if not gvi:
        print("[WARN] goversioninfo 未找到，跳过图标/版本嵌入", file=sys.stderr)
        return None

    maj, mino, pat, bld = _version_quad(version)
    ver_str = f"{maj}.{mino}.{pat}.{bld}"
    vi = {
        "FixedFileInfo": {
            "FileVersion": {"Major": maj, "Minor": mino, "Patch": pat, "Build": bld},
            "ProductVersion": {"Major": maj, "Minor": mino, "Patch": pat, "Build": bld},
            "FileFlagsMask": "3f",
            "FileFlags ": "00",
            "FileOS": "040004",
            "FileType": "01",
            "FileSubType": "00",
        },
        "StringFileInfo": {
            "CompanyName": "byteyang",
            "FileDescription": "NexusDesktop",
            "FileVersion": ver_str,
            "InternalName": "NexusDesktop",
            "LegalCopyright": "Copyright byteyang",
            "OriginalFilename": "NexusDesktop.exe",
            "ProductName": "NexusDesktop",
            "ProductVersion": ver_str,
        },
        "VarFileInfo": {
            "Translation": {"LangID": "0409", "CharsetID": "04B0"}
        },
        "IconPath": os.path.abspath(icon_ico).replace("\\", "/"),
        "ManifestPath": "",
    }
    with open(vi_path, "w", encoding="utf-8") as f:
        json.dump(vi, f, indent=2)

    try:
        result = subprocess.run(
            [gvi, "-64", "-o", syso_path],
            cwd=cmd_dir,
            env=env,
        )
    finally:
        if os.path.isfile(vi_path):
            os.remove(vi_path)

    if result.returncode != 0 or not os.path.isfile(syso_path):
        print("[WARN] goversioninfo 执行失败，跳过图标/版本嵌入", file=sys.stderr)
        return None

    print(f"[icon] resource.syso 已生成（FileVersion {ver_str}）: {syso_path}")
    return syso_path


def _zip_windows_update(exe_path: str, output_dir: str, version: str) -> str:
    """把 NexusDesktop.exe 打进版本化 zip 更新包。"""
    zip_path = os.path.join(output_dir, f"NexusDesktop-windows-amd64-v{version}-update.zip")
    if os.path.isfile(zip_path):
        os.remove(zip_path)
    with zipfile.ZipFile(zip_path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        zf.write(exe_path, "NexusDesktop.exe")
    size_mb = os.path.getsize(zip_path) / (1024 * 1024)
    print(f"[update] 更新包：{size_mb:.1f} MB → {zip_path}")
    return zip_path


def find_iscc() -> str | None:
    """返回 Inno Setup 7 的 ISCC.exe；找不到则 None。"""
    candidates: list[str] = []
    for d in [
        r"C:\Program Files\Inno Setup 7\ISCC.exe",
        r"C:\Program Files (x86)\Inno Setup 7\ISCC.exe",
    ]:
        if os.path.isfile(d):
            candidates.append(d)
    found = shutil.which("ISCC") or shutil.which("iscc")
    if found:
        candidates.append(found)
    for exe in candidates:
        if "inno setup 7" in os.path.normpath(exe).lower():
            return exe
    return None


def _build_windows_installer(
    root: str,
    version: str,
    output_dir: str,
    exe_path: str,
) -> str | None:
    """
    用 Inno Setup 编译 Setup.exe。成功返回安装包路径；本机无 ISCC 时：
    CI 环境抛错，本地仅 WARN 并返回 None。
    """
    iscc = find_iscc()
    if not iscc:
        msg = (
            "未找到 Inno Setup 7（ISCC.exe）。release 安装包已跳过。\n"
            "       安装 Inno Setup 7：https://jrsoftware.org/isdl.php\n"
            "       或 winget install JRSoftware.InnoSetup.7"
        )
        if os.environ.get("CI"):
            raise RuntimeError(msg)
        print(f"[WARN] {msg}", file=sys.stderr)
        return None

    iss_path = os.path.join(root, "installer", "NexusDesktop.iss")
    if not os.path.isfile(iss_path):
        raise FileNotFoundError(f"找不到安装脚本: {iss_path}")

    def iss_path_arg(p: str) -> str:
        # 正斜杠避免 ISCC /D 把 \r \n 当转义
        return os.path.abspath(p).replace("\\", "/")

    base_name = f"NexusDesktop-windows-amd64-v{version}-setup"
    cmd = [
        iscc,
        f"/DMyAppVersion={version}",
        f"/DMyAppExePath={iss_path_arg(exe_path)}",
        f"/DMyOutputDir={iss_path_arg(output_dir)}",
        f"/DMyOutputBaseFilename={base_name}",
    ]
    icon_ico = os.path.join(root, "assets", "icon.ico")
    if os.path.isfile(icon_ico):
        cmd.append(f"/DMySetupIcon={iss_path_arg(icon_ico)}")
    cmd.append(iss_path)

    print(f"[setup] ISCC: {iscc}")
    result = subprocess.run(cmd, cwd=root)
    if result.returncode != 0:
        raise RuntimeError(f"Inno Setup 编译失败（返回码 {result.returncode}）")

    setup_path = os.path.join(output_dir, base_name + ".exe")
    if not os.path.isfile(setup_path):
        raise RuntimeError(f"ISCC 成功但未找到安装包: {setup_path}")
    setup_mb = os.path.getsize(setup_path) / (1024 * 1024)
    print(f"[setup] 安装包：{setup_mb:.1f} MB → {setup_path}")
    return setup_path


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


def build_desktop(
    version: str,
    output_dir: str,
    build_type: str = "develop",
    arch_target: str = "arm64",
) -> str:
    """
    build_type : "develop" | "release"
    arch_target: macOS 专用 — 仅 "arm64"（Apple Silicon）。其他平台忽略此参数。
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
        syso_path = _embed_windows_resources(root, go, env, version)
        try:
            _go_build_binary(go, env, root, "windows", arch, exe_path, ldflags)
        finally:
            if syso_path and os.path.isfile(syso_path):
                os.remove(syso_path)
                print(f"[icon] 已清理: {syso_path}")
        size_mb = os.path.getsize(exe_path) / (1024 * 1024)
        print(f"[build] 产物大小：{size_mb:.1f} MB → {exe_path}")
        if is_release:
            zip_path = _zip_windows_update(exe_path, output_dir, version)
            setup_path = _build_windows_installer(root, version, output_dir, exe_path)
            if setup_path:
                os.remove(exe_path)
                return setup_path
            print("[WARN] 未生成 Setup.exe，保留 exe 供本机调试", file=sys.stderr)
            return zip_path
        return exe_path

    elif system == "Darwin":
        suffix = "" if is_release else "-dev"
        ldflags = (
            f"{strip_flags}"
            f"-X main.appVersion={version} "
            f"-X {_LOG_PKG}.Level={log_level}"
        )

        # 仅 Apple Silicon；不再打 Intel / Universal
        if arch_target in ("auto", "arm64"):
            target_arch = "arm64"
        else:
            raise RuntimeError(
                f"不支持的 arch_target: {arch_target}（macOS 仅打包 arm64 / Apple Silicon）"
            )

        out_path = os.path.join(output_dir, f"NexusDesktop-darwin-{target_arch}{suffix}")
        _go_build_binary(go, env, root, "darwin", target_arch, out_path, ldflags)
        size_mb = os.path.getsize(out_path) / (1024 * 1024)
        print(f"[build] 产物大小：{size_mb:.1f} MB")

        app_path = _package_macos_app(
            root, out_path, version, output_dir, target_arch, is_release,
        )
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


def _zip_macos_app(app_dir: str, zip_path: str) -> None:
    """把 .app 打进 zip 更新包；优先 ditto 以保留可执行位。"""
    if os.path.isfile(zip_path):
        os.remove(zip_path)
    if shutil.which("ditto"):
        result = subprocess.run(
            ["ditto", "-c", "-k", "--keepParent", app_dir, zip_path],
        )
        if result.returncode != 0 or not os.path.isfile(zip_path):
            raise RuntimeError("ditto 打包 .app zip 失败")
    else:
        parent = os.path.dirname(app_dir)
        with zipfile.ZipFile(zip_path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
            for dirpath, _, filenames in os.walk(app_dir):
                for name in filenames:
                    full = os.path.join(dirpath, name)
                    rel = os.path.relpath(full, parent).replace("\\", "/")
                    info = zipfile.ZipInfo(rel)
                    st = os.stat(full)
                    info.external_attr = (st.st_mode & 0xFFFF) << 16
                    with open(full, "rb") as f:
                        zf.writestr(info, f.read())
    size_mb = os.path.getsize(zip_path) / (1024 * 1024)
    print(f"[update] 更新包：{size_mb:.1f} MB → {zip_path}")


def _package_macos_app(
    root: str, binary: str, version: str, output_dir: str, arch: str,
    is_release: bool = False,
) -> str | None:
    """将 macOS 二进制封装为 .app bundle；release 额外打 zip 更新包，并打 DMG。"""
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

    if is_release:
        zip_name = f"NexusDesktop-darwin-{arch}-v{version}-update.zip"
        _zip_macos_app(app_dir, os.path.join(output_dir, zip_name))

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
        default="arm64",
        choices=["arm64"],
        help="macOS 目标架构：仅 arm64（Apple Silicon）",
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
