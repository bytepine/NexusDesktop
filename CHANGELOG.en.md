# Changelog — NexusDesktop

**语言 / Language**: [简体中文](CHANGELOG.md) · **English**

All notable changes to NexusDesktop are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

## [2.3.3] - 2026-11-10

### Fixed

- Section caches and disconnected snapshots are isolated by the complete read arguments, preventing data from leaking across components, properties, filters, and default-value baselines; different Sections can still reuse covering results.
- Brief HTTP probe failures while UE is busy no longer interrupt live WebSockets or in-flight requests. If the explicitly selected Editor is unavailable, do not switch to Game/DS; disconnected writes are never replayed automatically.

## [2.3.2] - 2026-11-05

### Added

- Long editor operations support progress notifications, owned task history, and cancellation under the same session identity. Task responses are not cached and writes are not retried after disconnection. Read caches and disconnected snapshots are suspended for the target instance during asynchronous writes, then cleared and restored after all known writes finish.

### Changed

- Standard MCP images pass through inline with their metadata; responses containing images bypass large-text offloading and proxy caching.
- Chinese and English landing pages clarify the Unreal Engine MCP desktop-client role, NexusLink dependency, and installation entry.

### Fixed

- Explicit `TtlSeconds: 0` responses bypass caching and clear the corresponding old caches and disconnected snapshots; standard structured responses, text JSON, and legacy fields are supported.
- Project-wide writes such as GameplayTags invalidate the target UE instance's read cache, including partial failures; other instances stay isolated.
- Align the vertical baselines of Chinese labels and mixed-font text.
- Offline connection guidance uses the current SearchCapabilities tool name.

## [2.3.1-beta.1] - 2026-10-28

> Pre-release; use outside production environments only.

### Changed

- Responses containing standard MCP image blocks retain inline images and existing metadata, bypassing large-text offloading and proxy caching.

- Align Chinese and English landing pages around the Unreal Engine MCP desktop-client role, highlight the NexusLink dependency, and add GitHub links for installers, the core plugin, and documentation.

### Added

- Long editor operations use dedicated connections with per-request MCP progress and wait cancellation. Isolate session identities, skip intermediate notifications and match final responses, and disable task-state caching and write retries.


### Fixed

- Fix inconsistent vertical baselines for Chinese labels and mixed-font text, aligning settings tabs and text within the same row.
- Add Wayland development dependencies to Linux CI, fixing missing native-check headers after the Fyne upgrade.

## [2.3.0] - 2026-10-27

### Added

- Client configuration supports Cursor / Windsurf, Codex, Claude Code / VS Code, CodeBuddy, Cline, and Roo Code. Identical formats share an option; Codex automatically generates Streamable HTTP TOML.

### Changed

- Group settings into General, Connection, and Security; collapse infrequent options and hide tokens by default.
- Unify settings, client-configuration, and write-confirmation window layouts with resizing and always-visible bottom actions.
- Copy only client configuration; display placement instructions and copy results separately.

### Fixed

- Allow saving without scrolling to the bottom; reopening settings preserves unsaved edits, and language changes are saved with other settings.
- Refresh the tool list after reconnecting UE, preventing failed calls after plugin upgrades or instance switches.
- When switching instances, an old connection exiting cannot interrupt the new connection or cause duplicate forwarding.

## [2.2.0] - 2026-10-23

### Added

- Each startup creates separate logs with automatic 4 MiB rotation and retained historical files.

### Fixed

- Fix disconnections caused by concurrent initialization and session-capacity reclamation; protect active sessions and support client-initiated session deletion.
- Add diagnostic logs for MCP sessions, HTTP, tool calls, and UE connections without recording sensitive data.

## [2.1.2] - 2026-10-22

### Changed

- The tray always shows UE connection state at the top; remove the recent-call label.

### Fixed

- Support both released 2.x and current tool-call formats, handling cache lifetimes and dangerous-write confirmation correctly while forwarding requests unchanged.
- Read caches and disconnected snapshots are isolated by UE instance. Batch calls, including partial failures, invalidate the target instance's cache; failed reads are never cached.
- Large-response temporary files are isolated by session. Old and new 2.x processes cannot delete one another's results, and offloading preserves the original cache lifetime.
- Reject invalid target ports directly instead of forwarding to the current instance.

## [2.1.1] - 2026-10-08

### Changed

- docs: Replace coined terms in documentation and UI with ordinary wording: authentication and confirmation before writes.

### Fixed

- fix(log): In release builds without a console, logs/nexusdesktop.log is no longer left empty by an invalid stdout handle.
- fix(proxy): Always Allow This Session remembers a capability only within the current MCP session; closing the confirmation window denies immediately.
- fix(ui): Copied MCP client configuration uses the actual listening port, including when a collision moves it beyond the configured port.
- fix(mcp): Set HTTP ReadHeaderTimeout to 10s and IdleTimeout to 60s; limit SSE connections to 32.
- fix(config): Limit the UE scan range to 200 ports including endpoints; reject invalid ports, intervals, and excessively wide ranges when saving settings.
- fix(proxy): Use permissions 0700 for large-response directories and 0600 for files; clean startup leftovers and delete files older than one hour during writes.
- fix(ui): Confirm that plaintext WebSocket exposes tokens when configuring remote UE for the first time.

## [2.1.0] - 2026-09-21

### Added

- feat(unreal): Prefer `hostKind=Editor` for multi-instance auto-connection, falling back to `netRole=Editor` for older UE; expose `hostKind` / `hasPlayWorld` in `list_unreal_instances`.

### Changed

- chore(release): Package macOS only for Apple Silicon (arm64), using NexusDesktop-darwin-arm64.dmg and *-arm64-v*-update.zip instead of Universal/Intel packages. Older installed versions cannot find the renamed in-app update files and must download a new DMG from Releases.

### Fixed

- fix(updater): Replace the Windows executable through a hidden helper process during in-app updates, avoiding command windows.

## [2.0.1] - 2026-09-04

### Changed

- ui: Generate one MCP client configuration snippet at a time by transport (Streamable HTTP / SSE) and client (Cursor / CodeBuddy), so copying never includes another client's configuration.

## [2.0.0] - 2026-08-31

### Added

- feat(mcp): Add a proxy session layer with TTL/section read caches, disconnected degraded snapshots, a configurable write gate defaulting to destructive-operation confirmation, tray pause/resume of agent forwarding and recent calls, and temporary-file offloading for oversized responses.
- feat(mcp): Add listenLan and remoteUnrealText, using host:port for scanning and connection.
- feat(ui): Display the machine authentication token in settings and support one-click token copying from the tray and configuration window.
- feat(updater): Add auto/stable/pre update channels, following the installed version by default: stable excludes betas, prereleases may include betas, and stable-only remains selectable. Check on startup and every 6 hours; distinguish failed checks from up-to-date results in the tray. Compare numeric prerelease segments correctly (`2.0.0` > `2.0.0-beta.N`, `beta.10` > `beta.9`) and never downgrade.

### Changed

- perf(mcp): Make handleInitialize connection text stable and defer live state to tools/list, preserving the prompt cache across WebSocket connection changes.
- docs: Turn the README into a product landing page; link the shared port and switch matrix to NexusLink docs/usage-guide.md.
- Disable the relay server for new installations without changing existing config.json files.
- feat(mcp): Use one machine token shared with UE / Rider / VSCode in NexusLink/mcp-auth-token; add Require MCP Auth, enabled by default, and comma-separated extra tokens and Bearer values. Local UE reads the token file automatically. Configuration copying supports IP selection with multiple adapters and includes only the machine token in Bearer. Confirm LAN access without authentication; link README guidance to usage-guide §1.1.
- ui: Add and remove extra authentication tokens individually instead of manually typing semicolon or comma separators.
- chore(ci): Add build.yml running vet and test on pushes/PRs; release checks VERSION/tag consistency and sets explicit prerelease flags and Release names.

### Fixed

- fix(mcp): After Ensure fails, tools/call still attempts forwarding once before retrying, matching IDE proxies.
- fix(mcp): Saving settings immediately starts, stops, or restarts MCP according to Enabled/port; apply scan parameters immediately and warn if the MCP port falls within the scan range.
- fix(config): Back up invalid config.json to config.json.bak and avoid writing defaults over it on startup, preventing loss of user settings.
- fix(mcp): Return 413 for oversized bodies without Content-Length instead of parsing truncated content and reporting invalid JSON.

### Security

- Require Bearer authentication and reject Origin on MCP `/stream`; remove CORS `*` and limit request bodies to 1MB.
- Compare Bearer tokens in constant time with crypto/subtle, matching UE / Rider.
- Read the instance-registry token when connecting to UE and send auth as the first WebSocket frame; GET /status must identify nexus and must not follow redirects.
- Verify GitHub Release SHA256SUMS for in-app updates; pin Inno SHA256, w64devkit size, and goversioninfo@v1.5.0 in CI.
- Keep loopback binding as the default; LAN requires Bearer authentication and must not be exposed to the public internet.

## [1.1.1] - 2026-08-14

### Added
- feat(updater): If silent in-app download/replacement fails, open that version's GitHub Release page for manual installation, except when running from a DMG.

### Changed
- chore(release): Rename update archives to *-update.zip, distinguishing them from installers (-setup.exe / .dmg) and avoiding manual installation mistakes.

## [1.1.0] - 2026-08-14

### Added
- feat(i18n): Support Simplified Chinese and English UI, following the system language by default with a manual settings override.
- feat(installer): Add a Windows Inno Setup installer. Default current-user installation to `%LOCALAPPDATA%\Programs\NexusDesktop` and all-users to `C:\Program Files\NexusDesktop`. Upgrade existing installations in place while preserving settings; uninstall through Apps & Features with optional removal of the current user's settings/logs.
- feat(updater): When the tray detects a new version, download its versioned ZIP update, replace the installation, and restart on Windows/macOS. Update Windows Apps & Features DisplayVersion; reject downgrades from an older Setup after an executable update.
- feat(release): Produce installers (Setup.exe / DMG) and ZIP updates in the same release build; Windows updates contain the executable and macOS updates contain the .app.

### Changed
- chore(installer): Require Inno Setup 7 with 64-bit ISCC and SetupArchitecture=x64.
- chore(release): Publish only Setup.exe for Windows, removing the portable ZIP package.

### Fixed
- fix(installer): Account for Inno Setup 7 interpreting Pascal #13#10 as a preprocessor directive and leading [...] as section tags; write AppId {GUID} as {{GUID}.

## [1.0.6] - 2026-07-17

### Fixed
- fix(updater): Check updates through GitHub releases/latest redirects with a User-Agent instead of the REST API, whose unauthenticated 403s caused silent failures. Compare latest > current with semver; display the current version and support manual checks from the tray.

## [1.0.5] - 2026-07-17

### Added
- feat: Report proxy forwarding failures, including disconnections, timeouts, and failed `connect_unreal_instance`, through the new `nexus/proxy_feedback` method to UE `.nexus-feedback/`, allowing AI feedback to include relay errors. Buffer in-process and resend after reconnecting; older NexusLink versions without this method degrade silently without disrupting normal use.

### Fixed
- Display MCP client-configuration snippets with foreground color instead of disabling the Entry, improving readability and manual selection/copying.

## [1.0.4] - 2026-07-09

### Fixed
- Startup: verify the executable path in the registry/LaunchAgent and rewrite stale paths to the current executable, fixing enabled-looking but broken startup after moving the executable.
- Fix intermittent crashes while switching UE instances by sharing a lock between keepalive pings and JSON-RPC writes, preventing gorilla/websocket concurrent-write panics.
- Refresh the tray through fyne.Do in Refresh / SetUpdateState, avoiding occasional crashes from background goroutines changing UI directly on Fyne 2.6+.
- Write unhandled panics to logs/crash.log and log-open failures to logs/init-error.txt, since stdout is invisible with windowsgui.

## [1.0.3] - 2026-07-07

### Changed
- Rename the tray's Copy MCP Client Configuration action to MCP Client Configuration…, opening a window with Streamable HTTP / SSE snippets and one-click clipboard copying, following Rider settings interaction.

## [1.0.2] - 2026-07-07

### Added
- Add Check for Updates to the tray. Query the latest GitHub Release asynchronously on startup; when newer, show [New Version] vX.X.X → Download and open its download page on click.
- Package Windows as ZIP with a versioned filename (`NexusDesktop-windows-amd64-v<ver>.zip`) containing the fixed executable name NexusDesktop.exe.

## [1.0.1] - 2026-07-07

### Added
- Add build.bat, build_beta.bat, build_release.bat, and build.command for one-click cross-platform packaging.
- Support --build-type develop|release in build_desktop.py, distinguishing debug and release builds.
- Improve logging with Debug/Debugf, level filtering (develop=debug / release=info), and millisecond timestamps.
- Add Debug logs to MCP, WebSocket, and scanning layers.
- Add Scan UE Instances to the tray for immediate manual port scanning.

### Fixed
- Fix menus disappearing after 1–2 seconds: rebuild only when instance lists or connection state change, instead of forcing recreation on every timer tick.
- Use Manager.Snapshot() to eliminate concurrent read/write races on Instances/ConnectedPort and refresh menus after discovery.
- Move the immediate startup scan after tray.Setup(), ensuring the tray is ready before receiving refresh callbacks.
- Replace build-script `if exist ... & goto` with parenthesized blocks, fixing BAT windows closing immediately on double-click.
- Automatically detect Go installation paths such as C:\tools\go\bin, avoiding manual PATH configuration.
- Use release builds in GitHub Actions CI with info logging, -H=windowsgui, and -s -w.
- Fix missing or duplicate tray Quit buttons by adding one manually with IsQuit=true, preventing Fyne from injecting another and ensuring exactly one Quit action.
- Change appVersion from const to var to support version injection with linker -X.

### Changed
- Change the default MCP HTTP port from 6900 to 6700.
- Enable the relay server by default (Enabled defaults to true).
- Use built-in Fyne tray icons to distinguish connection state: ComputerIcon when connected and InfoIcon when disconnected.

## [1.0.0] - 2026-07-07

### Added
- Add a standalone MCP HTTP server with POST /stream Streamable HTTP and GET /sse or /stream SSE notification streams.
- Implement JSON-RPC 2.0 and the MCP session state machine (initialize/initialized/ping/tools).
- Isolate sessions using the Mcp-Session-Id header, allowing concurrent AI clients without interference.
- Discover UE instances automatically through concurrent GET /status port scans, defaulting to 45000–45100.
- Proxy through persistent WebSockets and serialize requests to avoid concurrent accumulation on the UE GameThread.
- Send keepalive pings every 15s while idle and 5s while busy, distinguishing disconnection from timeout.
- Cache tools/list and invalidate it on UE tools/list_changed notifications.
- Use preferredPort to restore the manually selected instance first after disconnection.
- Support one-shot arguments.targetPort routing for concurrent multi-instance queries.
- Fetch initialize.instructions from UE nexus/instructions and append them to proxy guidance.
- Consume nexus/proxy_config from UE, with built-in fallback when disconnected.
- Add a Fyne system tray with status, instance submenu, enable switch, configuration copying, settings, log directory, startup, and quit.
- Add a Fyne settings window with all configuration fields, instance list, and hot reload on save; closing hides it to the tray.
- Use a lockfile to prevent multiple application instances.
- Support cross-platform startup through Windows Run registry entries and macOS LaunchAgent, with Linux XDG autostart reserved.
- Initial release for Windows and macOS.
