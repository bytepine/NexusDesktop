# Changelog

All notable changes to NexusDesktop are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

## [1.0.6] - 2026-07-17

### Fixed
- fix(updater): 检查更新改走 GitHub `releases/latest` 重定向（带 User-Agent），不再调 REST API（无鉴权常 403 导致静默失败）；用 semver 判断 `latest > current`；托盘菜单显示当前版本，点击可手动复检

## [1.0.5] - 2026-07-17

### Added
- feat: 代理转发失败（断连/超时/`connect_unreal_instance` 失败）经新 `nexus/proxy_feedback` 上报给 UE，写入 `.nexus-feedback/`，使中转层错误也能被 AI 反馈系统记录；进程内缓冲 + 连上后自动补发；旧版 NexusLink（未实现该方法）静默降级，不影响正常使用

### Fixed
- MCP 客户端配置窗口：配置片段改用前景色展示（不再 `Disable` Entry），提升可读性与手动选中复制体验

## [1.0.4] - 2026-07-09

### Fixed
- 开机自启：校验注册表/LaunchAgent 中的 exe 路径是否仍存在；启动时若路径失效则自动重写为当前可执行文件（修复 exe 迁移后菜单仍显示已启用但实际无法自启）
- 切换 UE 实例偶发闪退：keepalive Ping 与 JSON-RPC 写共用同一把锁，消除 gorilla/websocket 并发写 panic
- 托盘刷新：`Refresh` / `SetUpdateState` 经 `fyne.Do` 切回 UI 线程，避免 Fyne ≥2.6 后台 goroutine 直接改托盘导致偶发崩溃
- 未处理 panic 写入 `logs/crash.log`；日志文件打开失败时写入 `logs/init-error.txt`（windowsgui 下 stdout 不可见）

## [1.0.3] - 2026-07-07

### Changed
- 托盘「复制 MCP 客户端配置」改为「MCP 客户端配置…」：点击后弹出配置窗口，展示 Streamable HTTP / SSE 两种配置片段，点击「复制」一键写入剪贴板（参考 NexusRider 设置面板交互）

## [1.0.2] - 2026-07-07

### Added
- 托盘菜单「检查更新」：启动后异步查询 GitHub 最新 Release，发现新版本时标签变为「[新版本] vX.X.X → 下载」，点击跳转到下载页面
- Windows 产物改为 ZIP 打包：zip 文件名含版本号（`NexusDesktop-windows-amd64-v<ver>.zip`），zip 内 exe 保持固定名称（`NexusDesktop.exe`）

## [1.0.1] - 2026-07-07

### Added
- 构建脚本 `build.bat` / `build_beta.bat` / `build_release.bat` / `build.command` 一键跨平台打包
- `build_desktop.py` 支持 `--build-type develop|release`，区分调试包与发布包
- 日志系统完善：新增 `Debug`/`Debugf`、日志级别过滤（develop=debug / release=info）、时间戳精确到毫秒
- MCP 层、WS 层、扫描层补充 Debug 级别日志
- 托盘菜单新增「扫描 UE 实例」按钮，可主动触发一次端口扫描

### Fixed
- 打开菜单 1–2 秒后自动消失：改为仅在实例列表/连接状态变化时刷新菜单，定时器不再强制重建
- 实例发现后菜单未更新：用 `Manager.Snapshot()` 消除 `Instances`/`ConnectedPort` 并发读写竞态
- 启动时立即扫描移至 `tray.Setup()` 之后，确保托盘就绪再接受刷新回调
- 构建脚本 `if exist ... & goto` 改为括号写法，修复双击 bat 闪退
- 构建脚本自动探测 Go 安装路径（`C:\tools\go\bin` 等），无需手动配置 PATH
- GitHub Actions CI 构建类型改为 `release`（info 日志 + `-H=windowsgui` + `-s -w`）
- 托盘退出按钮不稳定（0 个或 2 个）：改为手动添加并设置 `IsQuit=true`，阻止 Fyne 重复注入，始终保证恰好一个「退出」
- `appVersion` 由 `const` 改为 `var`，支持链接器 `-X` 注入版本号

### Changed
- MCP HTTP 默认端口从 6900 改为 6700
- 默认启用中转服务器（`Enabled` 默认值改为 `true`）
- 托盘图标改用 Fyne 内置图标并区分连接状态：已连接显示 `ComputerIcon`，未连接显示 `InfoIcon`

## [1.0.0] - 2026-07-07

### Added
- 独立 MCP HTTP 服务器（`POST /stream` Streamable HTTP + `GET /sse`/`/stream` SSE 通知流）
- JSON-RPC 2.0 协议 + MCP 会话状态机（initialize/initialized/ping/tools）
- per-session 会话隔离（`Mcp-Session-Id` header），多 AI 客户端并发连接互不干扰
- UE 实例自动发现（并发 `GET /status` 端口扫描，默认 45000–45100）
- WebSocket 长连接，串行请求避免 UE GameThread 并发积压
- 保活 ping（空闲 15s / 忙 5s），断连/超时区分
- `tools/list` 缓存 + UE `tools/list_changed` 推送失效
- `preferredPort`：用户手动选择后断连优先恢复
- `arguments.targetPort` 一次性路由（多实例并发查询）
- `initialize.instructions` 从 UE `nexus/instructions` 拉取并拼接
- 代理层 `nexus/proxy_config` 从 UE 下发，未连接时使用内置 fallback
- Fyne 系统托盘：状态行、实例子菜单、启用开关、复制配置、设置、日志目录、开机自启、退出
- Fyne 设置窗口：所有配置项表单 + 实例列表 + 保存热更新；关闭仅隐藏回托盘
- 单实例锁（lockfile）防多开
- 跨平台开机自启：Windows 注册表 Run / macOS LaunchAgent / Linux XDG autostart（预留）
- 首发 Windows + macOS
