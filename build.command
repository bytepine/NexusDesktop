#!/bin/bash
# build.command — NexusDesktop macOS 一键构建
# 双击即可在终端运行，产物输出至 release/

set -euo pipefail

# 切换到脚本所在目录（双击 .command 时 cwd 为 ~）
cd "$(dirname "$0")"

echo "============================================"
echo "  NexusDesktop - Build (macOS)"
echo "============================================"
echo

# ── 1. 读取当前版本 ──────────────────────────────────────
VERSION="$(cat VERSION | tr -d '[:space:]')"
echo "Version: $VERSION"
echo

# ── 2. 检测 Python 3 ────────────────────────────────────
if command -v python3 &>/dev/null; then
    PYTHON=python3
elif command -v python &>/dev/null && python --version 2>&1 | grep -q "Python 3"; then
    PYTHON=python
else
    echo "[ERROR] 未找到 Python 3，请先安装：https://www.python.org/downloads/"
    read -p "按回车退出..."
    exit 1
fi

# ── 3. 检测 Go ───────────────────────────────────────────
if ! command -v go &>/dev/null; then
    echo "[ERROR] 未找到 go 命令，请先安装 Go 1.24+：https://go.dev/dl/"
    read -p "按回车退出..."
    exit 1
fi

# ── 4. 检测 Xcode CLI（CGO 必须）────────────────────────
if ! command -v gcc &>/dev/null; then
    echo "[WARN] 未找到 gcc，尝试安装 Xcode CLI..."
    xcode-select --install || true
fi

# ── 5. 调用 Python 构建脚本 ──────────────────────────────
echo "[1/2] Building NexusDesktop (version: $VERSION)..."
"$PYTHON" scripts/build_desktop.py --version "$VERSION"

echo
echo "[2/2] Build successful!"
echo

# 找到产物并展示路径
ARTIFACT=$(ls release/NexusDesktop-darwin-*.app.zip 2>/dev/null | head -1 || true)
if [ -n "$ARTIFACT" ]; then
    echo "Output: $(pwd)/$ARTIFACT"
    # 在 Finder 中高亮文件
    open -R "$ARTIFACT" 2>/dev/null || true
else
    echo "Output: $(pwd)/release/"
fi

echo
read -p "按回车退出..."
