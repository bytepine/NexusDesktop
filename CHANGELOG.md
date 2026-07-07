# Changelog

All notable changes to NexusDesktop are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

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
- 托盘退出按钮重复（保留 Fyne 自动注入的 Quit，删除手写项）
- `appVersion` 由 `const` 改为 `var`，支持链接器 `-X` 注入版本号

### Changed
- MCP HTTP 默认端口从 6900 改为 6700
- 默认启用中转服务器（`Enabled` 默认值改为 `true`）

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
