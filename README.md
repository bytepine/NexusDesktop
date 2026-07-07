# NexusDesktop

**Language / 语言**: [简体中文](#nexusdesktop) · [English](#nexusdesktop-1)

---

## NexusDesktop

NexusDesktop 是一个**独立的本地 MCP 中转程序**，无需安装 IDE 插件即可使用：双击运行，在系统托盘常驻，AI 客户端通过 MCP HTTP 协议连接，程序自动发现本地 Unreal Engine 实例并经 WebSocket 转发工具调用。

与 IDE 插件方案对比：

| 接入方式 | 端点 | 适用 |
|----------|------|------|
| **NexusDesktop**（本程序） | `http://127.0.0.1:6700/stream` | 无需 IDE 插件；任意 AI 客户端；双击启动 |
| nexus-vscode | `http://127.0.0.1:6900/stream` | VSCode / Cursor 扩展 |
| nexus-rider | `http://127.0.0.1:6800/stream` | JetBrains Rider 插件 |
| 直连 UE | `http://127.0.0.1:45000/stream` | 需手动指定 UE 端口 |

---

## 依赖

| 组件 | 要求 |
|------|------|
| **NexusDesktop** | 直接下载 `.exe` / `.dmg`，无需 Go / Node / 运行时 |
| **NexusLink**（UE 插件） | [NexusLink Releases](https://github.com/bytepine/NexusLink/releases)；UE 4.26+ |
| **Windows** | Windows 10 / 11（amd64） |
| **macOS** | macOS 12+（Monterey）；Intel / Apple Silicon 通用 |

---

## 下载

从 [Releases](https://github.com/bytepine/NexusDesktop/releases) 下载最新版本：

- **Windows**：`NexusDesktop-windows-amd64.exe`（免安装，双击运行）
- **macOS**：`NexusDesktop-darwin-universal.dmg`（Universal Binary，支持 Intel + Apple Silicon）

---

## 使用

### 1. UE 前置条件

1. 从 [NexusLink Releases](https://github.com/bytepine/NexusLink/releases) 下载 `nexus-mcp-unreal-*.zip`，解压到 `Plugins/Developer/NexusLink`
2. UE：**Edit → Plugins → Developer → NexusLink** — 启用插件
3. UE：**Edit → Editor Preferences → Plugins → NexusLink** — 勾选 **启用 MCP 服务器**

### 2. 启动 NexusDesktop

**Windows**：双击 `NexusDesktop.exe`，程序进入系统托盘。

**macOS**：打开 `NexusDesktop-darwin-universal.dmg`，将 `NexusDesktop.app` 拖入 `Applications`，双击启动。程序不会出现在 Dock，仅在菜单栏（系统托盘）常驻。

托盘菜单功能：

| 菜单项 | 说明 |
|--------|------|
| 状态行 | 显示当前 UE 连接状态（项目名 / 未连接） |
| 选择 UE 实例 | 切换到指定 UE 实例 |
| ✓ 启用中转服务器 | 启停 MCP HTTP 监听（默认 `:6700`） |
| 复制 MCP 客户端配置 | 复制 JSON 到剪贴板 |
| 设置… | 打开设置窗口 |
| 打开日志目录 | 打开 `%APPDATA%/NexusDesktop/logs/` |
| 开机自启 | 切换开机自启 |
| 退出 | 退出程序 |

### 3. 配置 AI 客户端

**Cursor**（`~/.cursor/mcp.json`）：

```json
{
  "mcpServers": {
    "nexus-unreal": {
      "url": "http://127.0.0.1:6700/stream"
    }
  }
}
```

**CodeBuddy / Windsurf**：

```json
"Nexus": {
  "url": "http://127.0.0.1:6700/stream",
  "transportType": "streamable-http"
}
```

### 4. 设置窗口

双击托盘图标或点击「设置…」打开配置界面：

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| 启用中转服务器 | 开 | 总开关 |
| MCP HTTP 端口 | 6700 | AI 客户端连接端口 |
| UE 扫描起始端口 | 45000 | UE 实例扫描范围 |
| UE 扫描结束端口 | 45100 | UE 实例扫描范围 |
| 扫描间隔（秒） | 5 | 定时重新发现间隔 |

关闭窗口仅隐藏回托盘，不退出程序。

---

## 架构

```
AI 客户端 ──POST /stream──► MCP HTTP Server (:6700)
                                    │
                             Dispatcher (JSON-RPC 2.0)
                                    │
                          UnrealManager (发现 + WS)
                                    │
                     ◄──── WebSocket JSON-RPC ──────► UE NexusLink
```

---

## 本地构建

### 依赖

- Go 1.24+
- GCC / MinGW-w64（Windows）或 Xcode CLI（macOS）— Fyne 需要 CGO

### Windows

```powershell
$env:CGO_ENABLED = "1"
go build -ldflags "-H=windowsgui -s -w" -o NexusDesktop.exe ./cmd/nexusdesktop/
```

### macOS

一键构建 Universal Binary DMG（arm64 + amd64）：

```bash
python3 scripts/build_desktop.py --build-type develop
# 或 release 包
python3 scripts/build_desktop.py --build-type release --arch universal
```

手动构建单架构：

```bash
# 当前架构
CGO_ENABLED=1 go build -ldflags "-s -w" -o NexusDesktop ./cmd/nexusdesktop/

# Universal Binary（需 Go 支持交叉编译 + lipo）
CGO_ENABLED=1 GOARCH=arm64 go build -o NexusDesktop-arm64 ./cmd/nexusdesktop/
CGO_ENABLED=1 GOARCH=amd64 go build -o NexusDesktop-amd64 ./cmd/nexusdesktop/
lipo -create -output NexusDesktop NexusDesktop-arm64 NexusDesktop-amd64
```

> **注意（Windows）**：GCC 16+ (binutils 2.46+) 产生 BigOBJ 格式，Go CGO 暂不支持。推荐使用 GCC 14.x（如 [w64devkit v1.23.0](https://github.com/skeeto/w64devkit/releases/tag/v1.23.0)）。

---

## 变更记录

见 [CHANGELOG.md](CHANGELOG.md)。

---

## License

[MIT](LICENSE) © byteyang

---

## NexusDesktop

NexusDesktop is a **standalone local MCP proxy** — no IDE plugin needed. Just run it; it lives in the system tray, letting AI clients connect via MCP HTTP while it auto-discovers local UE instances and forwards tool calls over WebSocket.

See the Chinese section above for full documentation. Quick start:

1. Enable NexusLink MCP server in UE Editor Preferences
2. **Windows**: double-click `NexusDesktop.exe` — it sits in the system tray
   **macOS**: open `NexusDesktop-darwin-universal.dmg`, drag the app to Applications, then launch — it lives in the menu bar only (no Dock icon)
3. The proxy starts automatically; point your AI client to `http://127.0.0.1:6700/stream`
