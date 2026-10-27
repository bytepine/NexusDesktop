# NexusDesktop

**Language / 语言**: [简体中文](README.md) · English

Standalone local MCP **proxy**: no IDE plugin. Run it; it lives in the system tray (menu bar on macOS), discovers local Unreal Engine instances, and forwards tool calls over WebSocket. Capabilities come from the UE **NexusLink** plugin.

Ports and switch layers: [NexusLink usage guide](https://github.com/bytepine/NexusLink/blob/master/docs/usage-guide.md). This app is the client layer of the **two-layer** switch (UE Enable MCP + tray Enable proxy). Do not run the Rider or VSCode proxy on the same machine at the same time.

---

## Requirements

| Component | Requirement |
|-----------|-------------|
| **NexusDesktop** | Download Setup.exe / `.dmg` — no Go / Node / runtime |
| **NexusLink** | [NexusLink Releases](https://github.com/bytepine/NexusLink/releases); UE 4.26+ |
| **Windows** | Windows 10 / 11 (amd64) |
| **macOS** | macOS 12+ (Monterey); Apple Silicon (arm64) only |

---

## Download & install

From [Releases](https://github.com/bytepine/NexusDesktop/releases):

- **Windows**: `NexusDesktop-windows-amd64-v<version>-setup.exe` — installer; uninstall from Apps & features
- **macOS**: `NexusDesktop-darwin-arm64.dmg` (Apple Silicon only)
- **Do not download `*-update.zip`**: that is the in-app update package, not an installer

### Windows install & uninstall

Install scope follows the Python.org Windows installer:

| Scope | Privileges | Default directory |
|-------|------------|-------------------|
| Current user (default) | No admin | `%LOCALAPPDATA%\Programs\NexusDesktop` |
| All users | UAC | `C:\Program Files\NexusDesktop` |

- Older Setup installs **upgrade in place** (same directory and scope; settings/logs kept)
- Running an older Setup after an in-app update is **rejected**
- Switching current-user ↔ all-users: uninstall first, then reinstall
- **Uninstall**: Windows Settings → Apps → NexusDesktop; optionally delete `%APPDATA%\NexusDesktop`
- Settings/logs always live in `%APPDATA%\NexusDesktop`

### In-app updates

Checks on startup and every 6 hours. Tray **Check for updates** downloads that version’s **zip update package** from GitHub Releases (not the installer), replaces files, and restarts. On Windows the replace step has no console window.

**Update channel** in Settings (default **Follow current version**): stable builds only see stable releases; pre-release builds also see newer betas and the same-core stable, or you can switch to **Stable only**. **Include pre-releases** lets a stable install move to a *newer* core beta (never `2.0.0` → `2.0.0-beta.N`).

| Platform | Installer | Update package |
|----------|-----------|----------------|
| Windows | `*-setup.exe` | `*-update.zip` (contains `NexusDesktop.exe`) |
| macOS | `.dmg` | `*-update.zip` (contains `NexusDesktop.app`) |

- **macOS**: drag the `.app` into `Applications` first; updating while running from the DMG fails
- Replacing a copy under `Program Files` prompts for administrator permission once

---

## Usage

### 1. UE prerequisites

Install and enable NexusLink, then check **Enable MCP Server** ([usage-guide §2](https://github.com/bytepine/NexusLink/blob/master/docs/usage-guide.md)).

### 2. Launch

Editor / PIE sessions use **Edit → Editor Preferences → Plugins → NexusLink → Enable MCP Server**. Standalone Development / DebugGame Game / DS processes do not inherit that switch: append `-EnableNexusMcp` to their existing launch arguments. Shipping builds do not support MCP.

**Windows**: after Setup, start from the Start menu; the app enters the tray.

**macOS**: open the dmg, drag `NexusDesktop.app` into `Applications`, then launch. It does **not** appear in the Dock — menu bar only.

Open the tray menu, enable the relay server (off for new installs), and verify that the status line shows the intended UE project. Use **Select UE instance** to switch processes or **Scan UE instances** if the target is missing.

![NexusDesktop relay switch and client configuration entry (Chinese UI)](docs/images/desktop-relay-tray.png)

Screenshots illustrate UI locations. Version numbers and legacy tool names are examples; use the tool list returned by your current connection.

| Tray item | Notes |
|-----------|-------|
| Status line | Always shows the current UE connection (project name / disconnected) |
| Select UE instance | Switch instance |
| Scan UE instances | Manual port scan |
| Pause / resume agent forwarding | Queue remote calls at the proxy |
| ✓ Enable proxy | Toggle MCP HTTP (default `:6700`; **off for new installs**) |
| MCP client config… | Pick protocol (Streamable HTTP / SSE) and client (Cursor / CodeBuddy), then copy one JSON snippet; pick NIC IP when LAN has multiple addresses; Bearer is this machine's token only |
| Copy auth token | Copy the machine-shared token only |
| Check for updates | Auto-check on startup and every 6 hours; click to download zip and restart |
| Settings… | Open settings |
| Open log directory | Open logs |
| Launch on login | Toggle autostart |
| Quit | Exit |

A single instance auto-connects; multiple instances prefer `hostKind=Editor` (legacy UE without that field falls back to `netRole=Editor`). Read caches and disconnected snapshots are isolated by UE host/port, so switching instances cannot reuse another instance's data.

Batch calls, including partial failures, invalidate the target instance's read cache. Failed reads are never cached. Large-response files are isolated between hubs; only files older than one hour are cleaned up.

Released 2.x and current tool-call formats are supported without rewriting requests. New payloads use a separate temporary root so older 2.x processes cannot delete them during startup cleanup.

### 3. AI client

Choose **MCP client config…** in the tray, select **Streamable HTTP** and Cursor / CodeBuddy, then copy the generated configuration. Use the URL and token from that window; the port may advance if the default is occupied.

![Generated MCP client configuration with the token redacted (Chinese UI)](docs/images/desktop-client-config.png)

**Cursor**: merge the generated server entry into `mcpServers` in `~/.cursor/mcp.json` (`%USERPROFILE%\.cursor\mcp.json` on Windows), keeping existing servers. The complete structure is shown below. You can also copy the token from the tray **Copy auth token** or Settings. Multiple tokens: `Bearer <tok1>, <tok2>`. See [usage-guide §1.1](https://github.com/bytepine/NexusLink/blob/master/docs/usage-guide.md#11-身份验证).

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

**CodeBuddy / Windsurf**: paste the generated server entry into the client's custom MCP configuration. Its Streamable HTTP entry includes `"transportType": "streamable-http"`.

> Replace `<token>` with the copied value. Treat it as a credential: do not commit real tokens or expose them in documentation, chats, or screenshots.

#### Verify the connection

1. Restart the MCP session or reopen the AI panel and check that the server is connected
2. UE's default SearchMode provides `SearchCapabilities`, `CallCapability`, and `SubmitFeedback`. The relay adds `list_unreal_instances` and `connect_unreal_instance`, usually making 5 tools. If only the two instance tools appear initially, discover and connect to UE, then refresh the list. Older UE plugin versions use snake_case names; follow the actual tool list
3. Ask the AI to **use MCP to report the current UE editor information** and check the returned project, engine version, and runtime state

See the [illustrated setup and daily-use examples](https://github.com/bytepine/NexusLink/blob/master/docs/usage-guide.md#3-nexusdesktop) in the NexusLink usage guide (Chinese).

### 4. Settings window

Double-click the tray icon or **Settings…**. Closing the window hides it to the tray; the app keeps running.

| Setting | Default | Notes |
|---------|---------|-------|
| Enable proxy | Off | Master switch; existing `config.json` is unchanged |
| MCP HTTP port | 6700 | AI client port; listen restarts immediately after save |
| UE scan start / end | 45000 / 45100 | Discovery range |
| Scan interval (s) | 5 | Periodic discovery |
| Write gate | Destructive | Off / destructive (delete, rename, stop PIE) / all writes |
| Update channel | Follow current version | Stable builds: stable only; pre-releases include betas (or force stable / include pre-releases) |
| Allow LAN | Off | Bind MCP to `0.0.0.0` |
| Require MCP auth | On | Off: AI clients need no Bearer (legacy proxy); UE WS auth still follows the editor |
| Extra auth tokens | (empty) | Add one token per row; not needed for local UE |
| Remote UE | (empty) | One `host:port [token...]` per line |
| Language | Follow system | Simplified Chinese / English |

Cross-machine and auth: [usage-guide §1](https://github.com/bytepine/NexusLink/blob/master/docs/usage-guide.md).

---

## Logs and connection diagnostics

Use the tray's log-directory menu to find `nexusdesktop_<startup>_pid<PID>_<part>_*.log` (`%APPDATA%\NexusDesktop\logs` on Windows). Each startup creates a new file; files from previous runs are preserved. A run continues in a new part after 4 MiB. Release builds record startup version and process ID, server stop reasons, MCP session creation and initialization, missing or unknown sessions, session eviction, SSE connections, and tool-call start, completion, duration and error codes at INFO/WARN level. Routine scan and heartbeat details remain at DEBUG.

Match session events by the hashed `session` value and calls by the `request` number within one server run. `missing_session` means the header was absent; `unknown_session` means the ID is absent from the current server. Logs omit Bearer tokens, complete session IDs, tool arguments and response bodies.

The server retains up to 50 MCP sessions. At capacity, it reclaims only sessions with no requests for 30 minutes and no in-flight calls or SSE connections (`idle_timeout`). New connections never evict sessions that are initializing or active; if no capacity can be freed, initialization returns 503 with `Retry-After` (`session_limit`). Clients can end a session with `DELETE /stream` and its session ID (`client_delete`). Reclaimed IDs require a new `initialize`.

When reporting a disconnect, include the incident time, AI client name/version, client error log and Desktop logs covering that time, including relevant rotated `nexusdesktop_*.log` files. Restarting the server invalidates existing sessions; clients must initialize again, which can be triggered by toggling the MCP connection in the client.

## Building locally

Run `go vet ./...` and `go test ./...` before submitting. Tests cover the PascalCase contract and local HTTP/WebSocket multi-instance routing. Without CGO/OpenGL, use `go vet -tags ci ./...` and `go test -tags ci ./...` for headless checks; these do not replace a native GUI build.

- Go 1.25+
- GCC / MinGW-w64 (Windows) or Xcode CLI (macOS) — Fyne needs CGO

**Windows**:

```powershell
$env:CGO_ENABLED = "1"
go build -ldflags "-H=windowsgui -s -w" -o NexusDesktop.exe ./cmd/nexusdesktop/
```

Or `build.bat`. GCC 16+ (binutils 2.46+) produces BigOBJ that Go CGO does not support; use GCC 14.x (e.g. [w64devkit v1.23.0](https://github.com/skeeto/w64devkit/releases/tag/v1.23.0)). Release Setup.exe also needs [Inno Setup 7](https://jrsoftware.org/isdl.php).

**macOS**:

```bash
python3 scripts/build_desktop.py --build-type develop
# release: python3 scripts/build_desktop.py --build-type release --arch arm64
```

Or `./build.command`.

Record product changes in [CHANGELOG.md](CHANGELOG.md) `[Unreleased]`. Tag prefix: `nexus-desktop-v`.

---

## License

[MIT](LICENSE) © byteyang
