# Changelog

All notable changes to NexusDesktop are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

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
