#!/bin/bash
# build_release.command — NexusDesktop macOS Release 构建
# 双击即可在终端运行，产物输出至 release/

set -euo pipefail
cd "$(dirname "$0")"

echo "============================================"
echo "  NexusDesktop - Release Build (macOS)"
echo "  日志级别：INFO（debug 日志不输出）"
echo "  产物：NexusDesktop-darwin-universal.dmg"
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

echo "[Go] $(go version)"
echo

# ── 1. 读取当前版本 ──────────────────────────────────────
VERSION="$(tr -d '[:space:]' < VERSION)"
echo "Version: $VERSION"
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

# ── 4. 调用 Python 构建脚本（release 模式）───────────────
echo "[1/2] Building NexusDesktop release (version: $VERSION)..."
echo "      (首次构建需下载依赖，约需 1-5 分钟，请耐心等待...)"
echo

if ! "$PYTHON" scripts/build_desktop.py --version "$VERSION" --build-type release; then
    echo
    echo "[FAILED] Build failed! See output above for details."
    read -rp "按回车退出..."
    exit 1
fi

# ── 5. 显示产物路径 ──────────────────────────────────────
echo
echo "[2/2] Release build successful!"
echo
ARTIFACT=$(ls release/NexusDesktop-darwin-universal.dmg 2>/dev/null \
        || ls release/NexusDesktop-darwin-universal*.dmg 2>/dev/null | head -1 \
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
