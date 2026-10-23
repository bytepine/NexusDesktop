# NexusDesktop

**Language / 语言**: 简体中文 · [English](README.en.md)

独立的本地 MCP **中转程序**：无需 IDE 插件，双击运行后在系统托盘（macOS 菜单栏）常驻，发现本机 Unreal Engine 实例并经 WebSocket 转发工具调用。能力由 UE 里的 **NexusLink** 提供。

四端端口与开关层数见 [NexusLink 使用指南](https://github.com/bytepine/NexusLink/blob/master/docs/usage-guide.md)。本程序是 **两层** 开关中的客户端层（UE 启用 MCP + 托盘启用中转）。本机不要与 Rider / VSCode 代理同时开。

---

## 依赖

| 组件 | 要求 |
|------|------|
| **NexusDesktop** | 下载 Setup.exe / `.dmg`，无需 Go / Node / 运行时 |
| **NexusLink** | [NexusLink Releases](https://github.com/bytepine/NexusLink/releases)；UE 4.26+ |
| **Windows** | Windows 10 / 11（amd64） |
| **macOS** | macOS 12+（Monterey）；仅 Apple Silicon（arm64） |

---

## 下载与安装

从 [Releases](https://github.com/bytepine/NexusDesktop/releases) 下载：

- **Windows**：`NexusDesktop-windows-amd64-v<版本号>-setup.exe` — 安装向导，可从「应用和功能」卸载
- **macOS**：`NexusDesktop-darwin-arm64.dmg`（仅 Apple Silicon）
- **不要下载 `*-update.zip`**：那是应用内更新包，不是安装包

### Windows 安装与卸载

安装范围仿 Python.org 安装器：

| 范围 | 权限 | 默认目录 |
|------|------|----------|
| 当前用户（默认） | 无需管理员 | `%LOCALAPPDATA%\Programs\NexusDesktop` |
| 全部用户 | 需 UAC | `C:\Program Files\NexusDesktop` |

- 已安装旧版时**原地升级**（沿用目录与范围，配置/日志保留）
- 若已通过应用内更新装上**更新**的版本，再运行旧 Setup 会被拒绝
- 在「当前用户」与「全部用户」之间切换：先卸载再安装
- **卸载**：Windows 设置 → 应用 → NexusDesktop；可选删除 `%APPDATA%\NexusDesktop`
- 配置/日志始终在 `%APPDATA%\NexusDesktop`，与安装范围无关

### 应用内更新

启动后自动检查，之后每 6 小时再查一次。托盘「检查更新」按版本号下载 GitHub Release 上的 **zip 更新包**（不是安装包），解压覆盖后重启。Windows 替换过程不弹出命令行。

设置里 **更新渠道**（默认「跟随当前版本」）：正式安装只收正式版；预发布安装会收更新的 beta 以及同主段正式版，也可改成「仅正式版」。选「包含预发布」时，正式安装可升到更高主段的 beta（不会把 `2.0.0` 降到 `2.0.0-beta.N`）。

| 平台 | 安装包 | 更新包 |
|------|--------|--------|
| Windows | `*-setup.exe` | `*-update.zip`（内含 `NexusDesktop.exe`） |
| macOS | `.dmg` | `*-update.zip`（内含 `NexusDesktop.app`） |

- **macOS**：须先把 `.app` 拖入 `Applications` 再更新；直接从 DMG 运行会失败
- 安装到 `Program Files` 时替换文件会请求一次管理员权限

---

## 使用

### 1. 先准备好 UE

安装并启用 NexusLink，勾选 **启用 MCP 服务器**（步骤见 [usage-guide §2](https://github.com/bytepine/NexusLink/blob/master/docs/usage-guide.md)）。

### 2. 启动

**Windows**：Setup 后从开始菜单启动，进入系统托盘。

**macOS**：打开 dmg，将 `NexusDesktop.app` 拖入 `Applications` 后启动。不出现在 Dock，仅菜单栏常驻。

| 托盘菜单 | 说明 |
|----------|------|
| 状态行 | 始终显示当前 UE 连接（项目名 / 未连接） |
| 选择 UE 实例 | 切换实例 |
| 扫描 UE 实例 | 主动扫端口 |
| 暂停 / 恢复 Agent 转发 | 远端调用在代理排队 |
| ✓ 启用中转服务器 | 启停 MCP HTTP（默认 `:6700`；**新安装默认关**） |
| MCP 客户端配置… | 选协议（Streamable HTTP / SSE）与客户端（Cursor / CodeBuddy）后复制一份 JSON；开 LAN 且多网卡时可选 IP；Bearer 仅本机 token |
| 复制身份验证 Token | 只复制本机共享 token |
| 检查更新 | 启动与每 6 小时自动检查；点击下载 zip 并重启 |
| 设置… | 打开设置窗口 |
| 打开日志目录 | 打开日志 |
| 开机自启 | 切换 |
| 退出 | 退出程序 |

唯一实例自动连接；多实例优先 `hostKind=Editor`（旧 UE 无该字段时改用 `netRole=Editor`）。读缓存与断线快照按 UE host/port 隔离，切换实例不会复用另一实例的数据。显式 `targetPort` 须为 1024–65535 的整数；非法值返回 `-32602`，不会转发到当前实例或返回缓存。

批量调用（含部分失败）会清理目标实例的读缓存，失败读取不会进入缓存。大响应临时文件相互隔离，仅清理超过 1 小时的文件。

兼容已发布 2.x 的工具调用格式和当前格式，请求原样转发。新版临时目录与旧 2.x 分开，避免旧进程清理新版结果。

### 3. AI 客户端

**Cursor**（`~/.cursor/mcp.json`）。Token 用托盘「复制身份验证 Token」或设置面板。可写多个：`Bearer <tok1>, <tok2>`。关闭「启用 MCP 身份验证」时可不带 `headers`。规则见 [usage-guide §1.1](https://github.com/bytepine/NexusLink/blob/master/docs/usage-guide.md#11-身份验证)。

```json
{
  "mcpServers": {
    "nexus-unreal": {
      "url": "http://127.0.0.1:6700/stream",
      "headers": {
        "Authorization": "Bearer <token>"
      }
    }
  }
}
```

### 4. 设置窗口

双击托盘图标或「设置…」。关闭窗口只藏回托盘，不退出。

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| 启用中转服务器 | 关 | 总开关；已有 `config.json` 不受影响 |
| MCP HTTP 端口 | 6700 | AI 客户端端口；保存后立即重启监听 |
| UE 扫描起始 / 结束端口 | 45000 / 45100 | 发现范围 |
| 扫描间隔（秒） | 5 | 定时发现 |
| 写入前确认 | 破坏性操作 | 关闭 / 破坏性（删除、重命名、停 PIE）/ 全部写操作 |
| 更新渠道 | 跟随当前版本 | 正式仅正式版；预发布含 beta（可改成仅正式版或强制含预发布） |
| 允许局域网接入 | 关 | MCP 绑 `0.0.0.0` |
| 启用 MCP 身份验证 | 开 | 关闭后 AI 连本中转无需 Bearer（同旧版）；连 UE 仍看对方身份验证 |
| 额外身份验证 Token | （空） | 点 + 逐条添加其他机器 token；本机 UE 自动读文件 |
| 远程 UE | （空） | 每行 `host:端口 [token...]` |
| 界面语言 | 跟随系统 | 简体中文 / English |

跨机与身份验证见 [usage-guide §1](https://github.com/bytepine/NexusLink/blob/master/docs/usage-guide.md)。

---

## 日志与断连排查

托盘「打开日志目录」中的 `nexusdesktop_<启动时间>_pid<进程ID>_<分段>_*.log` 记录启动版本、进程 ID、服务器启停原因、MCP 会话创建与握手、会话失效或淘汰、SSE 通断，以及工具调用开始、结束、耗时和错误码。每次启动新建文件，不追加或覆盖上次日志；同次启动超过 4 MiB 后写入新分段。Windows 日志目录为 `%APPDATA%\NexusDesktop\logs`。发布包的 INFO 级别即可看到这些记录，无需开发包；扫描与心跳的常规细节仍在 DEBUG。

会话通过 `session` 摘要关联，同一次服务器运行内的调用通过 `request` 编号关联；`missing_session` 表示客户端未携带会话 ID，`unknown_session` 表示 ID 在当前服务器中不存在。日志不记录 Bearer Token、完整会话 ID、工具参数或响应正文。

最多保留 50 个 MCP 会话；容量满时仅回收超过 30 分钟没有请求、也没有进行中调用或 SSE 连接的会话（`idle_timeout`）。正在初始化或使用的会话不会被新连接淘汰；无法回收时，新连接返回 503 和 `Retry-After`（`session_limit`）。客户端可通过携带会话 ID 的 `DELETE /stream` 主动结束会话（`client_delete`）。回收后的 ID 需要重新 `initialize`。

反馈断连时，请提供报错时间、AI 客户端名称与版本、客户端错误日志，以及对应启动时间和分段的 `nexusdesktop_*.log`；旧版本留下的 `nexusdesktop.log` 仍可用于排查之前的问题。服务器重启后原会话会失效，客户端需要重新 `initialize`，可关闭再开启该 MCP 连接。

## 本地构建

提交前运行 `go vet ./...` 与 `go test ./...`，其中包含当前 PascalCase 契约和本地 HTTP/WebSocket 多实例转发回归。无 CGO/OpenGL 的环境可用 `go vet -tags ci ./...` 与 `go test -tags ci ./...` 做无窗口检查；这不代替原生 GUI 构建。

- Go 1.25+
- GCC / MinGW-w64（Windows）或 Xcode CLI（macOS）— Fyne 需要 CGO

**Windows**：

```powershell
$env:CGO_ENABLED = "1"
go build -ldflags "-H=windowsgui -s -w" -o NexusDesktop.exe ./cmd/nexusdesktop/
```

或 `build.bat`。GCC 16+（binutils 2.46+）产生 BigOBJ，Go CGO 暂不支持；用 GCC 14.x（如 [w64devkit v1.23.0](https://github.com/skeeto/w64devkit/releases/tag/v1.23.0)）。release 安装包另需 [Inno Setup 7](https://jrsoftware.org/isdl.php)。

**macOS**：

```bash
python3 scripts/build_desktop.py --build-type develop
# release：python3 scripts/build_desktop.py --build-type release --arch arm64
```

或 `./build.command`。

功能变更写入 [CHANGELOG.md](CHANGELOG.md) `[Unreleased]`。tag 前缀：`nexus-desktop-v`。

---

## License

[MIT](LICENSE) © byteyang
