# Changelog

All notable changes to NexusDesktop are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

### Added

- feat(mcp): 代理会话层——TTL/section 读缓存、断线 `degraded` 快照、写门控（设置项，默认破坏性操作确认）、托盘暂停/恢复 Agent 转发与最近调用；超大响应落盘临时目录
- feat(mcp): `listenLan` + `remoteUnrealText`；扫描/连接走 `host:port`
- feat(ui): 设置面板展示本机鉴权 Token；托盘 / 配置窗可一键复制 token
- feat(updater): 更新渠道 `auto` / `stable` / `pre`（默认跟随版本：正式不收 beta，预发布可收 beta 且可改成仅正式版）；启动 + 每 6 小时检查；托盘区分检查失败 / 已是最新；semver 按预发布数字段比较（`2.0.0` > `2.0.0-beta.N`，`beta.10` > `beta.9`），只升不降

### Changed

- perf(mcp): `handleInitialize` 的连接状态文案改为固定句（以 tools/list 为准），避免随 WS 通断打穿 Prompt Cache
- docs: README 改为本产品落地页；全家桶端口与开关矩阵改链 NexusLink `docs/usage-guide.md`
- 新安装默认关闭中转服务器（已有 `config.json` 不变）
- feat(mcp): 鉴权 token 改为本机唯一，与 UE / Rider / VSCode 共用 `NexusLink/mcp-auth-token`；新增「启用 MCP 鉴权」（默认开）；额外 Token 与 Bearer 逗号分隔支持多 token；连本机 UE 自动读 token 文件；复制 MCP 配置时多网卡可选 IP，Bearer 仅本机 token；LAN 且关鉴权时确认；README 鉴权说明指向 usage-guide §1.1
- ui: 额外鉴权 Token 改为逐条添加/删除，不再手写分号或逗号分隔
- chore(ci): 新增 `build.yml`（push / PR 跑 vet + test）；release 增加 `VERSION` 与 tag 一致性门禁、显式 `prerelease` 标记与 Release 名称

### Fixed

- fix(mcp): `tools/call` 在 Ensure 失败时仍转发一次再重试，与 IDE 代理对齐
- fix(mcp): 设置保存后立即按 Enabled/端口启停或重启 MCP；扫描参数即时生效；MCP 端口落在扫描区间时告警
- fix(config): `config.json` 解析失败时先备份为 `config.json.bak`，且启动不再用默认值覆盖回写（原先一次启动就清掉用户设置）
- fix(mcp): 缺少 `Content-Length` 的超大 body 现在返回 413，而不是带着被截断的内容当 JSON 解析失败

### Security

- MCP `/stream` 须 Bearer 且拒绝 Origin；去掉 CORS `*`；body 上限 1MB
- Bearer 比对改常量时间（`crypto/subtle`），与 UE / Rider 端一致
- 连 UE 时读实例注册表 token，WebSocket 首帧 `auth`；`GET /status` 须含 nexus 且不跟随重定向
- 应用内更新校验 GitHub Release `SHA256SUMS`；CI 钉死 Inno SHA256、w64devkit 体积、`goversioninfo@v1.5.0`
- 默认仍绑 loopback；开 LAN 后须 Bearer，勿映射公网

## [1.1.1] - 2026-08-14

### Added
- feat(updater): 应用内静默下载/替换失败时打开对应版本的 GitHub Release 页，便于手动下安装包（从 DMG 运行除外）

### Changed
- chore(release): 更新包文件名改为 `*-update.zip`，与安装包（`-setup.exe` / `.dmg`）区分，避免手动下载装错

## [1.1.0] - 2026-08-14

### Added
- feat(i18n): 界面支持简体中文 / 英文；默认跟随系统语言，设置窗口可手动切换
- feat(installer): Windows 正式安装包（Inno Setup Setup.exe）；当前用户默认 `%LOCALAPPDATA%\Programs\NexusDesktop`，全部用户默认 `C:\Program Files\NexusDesktop`；已安装旧版时原地升级（保留配置）；卸载走系统「应用和功能」，可选删除当前用户配置/日志
- feat(updater): 托盘发现新版本后按版本号下载 zip 更新包，覆盖安装目录并重启（Windows / macOS）；Windows 同步「应用和功能」DisplayVersion；手动运行旧 Setup 时若已装 exe 更新则拒绝降级
- feat(release): 同一次 release 构建同时产出安装包（Setup.exe / DMG）与 zip 更新包（Windows 内含 exe，macOS 内含 `.app`）

### Changed
- chore(installer): 构建依赖改为 Inno Setup 7（64 位 ISCC，`SetupArchitecture=x64`）
- chore(release): Windows 发布产物改为仅 Setup.exe，不再打 zip 便携包

### Fixed
- fix(installer): Inno Setup 7 将 Pascal `#13#10` 当成预处理器指令、将行首 `[...]` 当成 section tag；AppId 的 `{GUID}` 需写成 `{{GUID}`

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
