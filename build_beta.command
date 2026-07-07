#!/bin/bash
# build_beta.command — NexusDesktop macOS Beta 构建
# 自动派生下一个 beta 版本号（x.y.z → x.y.(z+1)-beta），产物输出至 release/

set -euo pipefail
cd "$(dirname "$0")"

echo "============================================"
echo "  NexusDesktop - Beta Build (macOS)"
echo "============================================"
echo

# ── 0. 确保 Go 在 PATH ────────────────────────────────────
if ! command -v go &>/dev/null; then
    for GO_DIR in \
        "${GOROOT:-}/bin" \
        "/usr/local/go/bin" \
        "$HOME/go-sdk/go/bin" \
        "$HOME/go/bin" \
        "$HOME/.local/go/bin" \
        "/opt/homebrew/bin" \
        "/usr/local/bin"
    do
        if [ -x "$GO_DIR/go" ]; then
            export PATH="$GO_DIR:$PATH"
            break
        fi
    done
fi

if ! command -v go &>/dev/null; then
    echo "[ERROR] 找不到 go 命令。"
    echo "        请安装 Go 1.24+：https://go.dev/dl/"
    echo "        或将 Go/bin 路径加入系统 PATH。"
    read -rp "按回车退出..."
    exit 1
fi

# ── 1. 读取当前版本，自动派生 beta 版本号 ────────────────
CURRENT_VERSION="$(tr -d '[:space:]' < VERSION)"
echo "Current VERSION : $CURRENT_VERSION"

# 取 x.y.z 部分（去掉已有的 -xxx 后缀）
BASE_VERSION="${CURRENT_VERSION%%-*}"
MAJOR="${BASE_VERSION%%.*}"
REST="${BASE_VERSION#*.}"
MINOR="${REST%%.*}"
PATCH="${REST#*.}"
NEXT_PATCH=$(( PATCH + 1 ))
NEXT_VERSION="${MAJOR}.${MINOR}.${NEXT_PATCH}-beta"

echo "Next beta version: $NEXT_VERSION"
echo

# ── 2. 检测 Xcode CLI ───────────────────────────────────
if ! command -v clang &>/dev/null; then
    echo "[WARN] 未找到 clang，尝试安装 Xcode Command Line Tools..."
    xcode-select --install 2>/dev/null || true
    echo "       安装完成后请重新运行此脚本。"
    read -rp "按回车退出..."
    exit 1
fi

# ── 3. 检测 Python 3 ────────────────────────────────────
PYTHON=""
if command -v python3 &>/dev/null; then
    PYTHON=python3
elif command -v python &>/dev/null && python --version 2>&1 | grep -q "Python 3"; then
    PYTHON=python
else
    echo "[ERROR] 未找到 Python 3，请先安装：https://www.python.org/downloads/"
    read -rp "按回车退出..."
    exit 1
fi

# ── 4. 调用 Python 构建脚本 ──────────────────────────────
echo "[1/2] Building NexusDesktop (version: $NEXT_VERSION)..."
echo "      (首次构建需下载依赖，约需 1-5 分钟，请耐心等待...)"
echo

if ! "$PYTHON" scripts/build_desktop.py --version "$NEXT_VERSION"; then
    echo
    echo "[FAILED] Build failed! See output above for details."
    read -rp "按回车退出..."
    exit 1
fi

# ── 5. 显示产物路径 ──────────────────────────────────────
echo
echo "[2/2] Build successful!"
echo
ARTIFACT=$(ls release/NexusDesktop-darwin-universal*.dmg 2>/dev/null | head -1 \
        || ls release/NexusDesktop-darwin-*.dmg 2>/dev/null | head -1 \
        || true)
if [ -n "$ARTIFACT" ]; then
    echo "Output: $(pwd)/$ARTIFACT"
    open -R "$ARTIFACT" 2>/dev/null || true
else
    echo "Output: $(pwd)/release/"
fi

echo
read -rp "按回车退出..."
