# NexusDesktop

**Language / 语言**: [简体中文](README.md) · English

---

NexusDesktop is a **standalone local MCP proxy** — no IDE plugin needed. Just run it; it lives in the system tray (menu bar on macOS), letting AI clients connect via MCP HTTP while it auto-discovers local Unreal Engine instances and forwards tool calls over WebSocket.

Comparison with IDE plugin alternatives:

| Method | Endpoint | Use case |
|--------|----------|----------|
| **NexusDesktop** (this app) | `http://127.0.0.1:6700/stream` | No IDE needed; any AI client; double-click to start |
| nexus-vscode | `http://127.0.0.1:6900/stream` | VSCode / Cursor extension |
| nexus-rider | `http://127.0.0.1:6800/stream` | JetBrains Rider plugin |
| Direct UE | `http://127.0.0.1:45000/stream` | Requires manual port selection |

---

## Requirements

| Component | Requirement |
|-----------|-------------|
| **NexusDesktop** | Download Setup.exe / `.dmg` — no Go / Node / runtime needed |
| **NexusLink** (UE plugin) | [NexusLink Releases](https://github.com/bytepine/NexusLink/releases); UE 4.26+ |
| **Windows** | Windows 10 / 11 (amd64) |
| **macOS** | macOS 12+ (Monterey); Intel & Apple Silicon Universal Binary |

---

## Download

Get the latest release from [Releases](https://github.com/bytepine/NexusDesktop/releases):

- **Windows**: `NexusDesktop-windows-amd64-v<version>-setup.exe` — installer; uninstall from Apps & features
- **macOS**: `NexusDesktop-darwin-universal.dmg` — Universal Binary (Intel + Apple Silicon)
- Do not download `*-update.zip` — that is for in-app updates, not first-time install

### Windows install & uninstall

Install scope follows the Python.org Windows installer; the default directory depends on the scope:

| Scope | Privileges | Default directory |
|-------|------------|-------------------|
| Current user (default) | No admin | `%LOCALAPPDATA%\Programs\NexusDesktop` |
| All users | UAC required | `C:\Program Files\NexusDesktop` |

- If an older Setup install is found, the installer **upgrades in place** (same directory and scope; settings/logs kept)
- Running an older Setup after an in-app update is **rejected**, so the installed app is not downgraded
- To switch between current-user and all-users, uninstall first, then reinstall
- **Uninstall**: Windows Settings → Apps → NexusDesktop; optionally delete current-user data (`%APPDATA%\NexusDesktop`)
- Settings/logs always live in `%APPDATA%\NexusDesktop`, regardless of install scope

### In-app updates

When the tray “Check for updates” item finds a newer release, it downloads that version’s **zip update package** from GitHub Releases (not the installer), replaces the install, and restarts:

| Platform | Installer | Update package |
|----------|-----------|----------------|
| Windows | `NexusDesktop-windows-amd64-v<version>-setup.exe` | `NexusDesktop-windows-amd64-v<version>-update.zip` (contains `NexusDesktop.exe`) |
| macOS | `NexusDesktop-darwin-universal.dmg` | `NexusDesktop-darwin-universal-v<version>-update.zip` (contains `NexusDesktop.app`) |

- `*-update.zip` is **not** a portable install; first-time install still uses Setup.exe / DMG
- **macOS**: drag the `.app` into `Applications` first; updating while running from the DMG will fail
- Replacing a copy under `Program Files` prompts for administrator permission once
- Version checks use the running app; Windows also refreshes the Apps & features display version

---

## Usage

### 1. UE Prerequisites

1. Download `nexus-mcp-unreal-*.zip` from [NexusLink Releases](https://github.com/bytepine/NexusLink/releases) and extract to `Plugins/Developer/NexusLink`
2. UE: **Edit → Plugins → Developer → NexusLink** — enable the plugin
3. UE: **Edit → Editor Preferences → Plugins → NexusLink** — check **Enable MCP Server**

### 2. Launch NexusDesktop

**Windows**: After Setup, launch from the Start menu. The app enters the system tray.

**macOS**: Open `NexusDesktop-darwin-universal.dmg`, drag `NexusDesktop.app` into `Applications`, then launch it. The app does **not** appear in the Dock — it lives in the menu bar only.

Tray menu items:

| Item | Description |
|------|-------------|
| Status line | Shows current UE connection state (project name / disconnected) |
| Select UE instance | Switch to a specific UE instance |
| Scan UE instances | Manually trigger a port scan |
| ✓ Enable proxy | Toggle MCP HTTP listener (default `:6700`) |
| Copy MCP client config | Copy JSON snippet to clipboard |
| Check for updates | Query latest Release; downloads the zip update package and restarts in place when a newer version is found |
| Settings… | Open settings window |
| Open log directory | Opens the log folder |
| Launch on login | Toggle autostart |
| Quit | Exit the app |

### 3. Configure your AI client

**Cursor** (`~/.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "nexus-unreal": {
      "url": "http://127.0.0.1:6700/stream"
    }
  }
}
```

**CodeBuddy / Windsurf**:

```json
"Nexus": {
  "url": "http://127.0.0.1:6700/stream",
  "transportType": "streamable-http"
}
```

### 4. Settings window

Double-click the tray icon or click "Settings…" to open the configuration panel:

| Setting | Default | Description |
|---------|---------|-------------|
| Enable proxy | On | Master on/off switch |
| MCP HTTP port | 6700 | Port for AI clients to connect |
| UE scan start port | 45000 | UE instance scan range |
| UE scan end port | 45100 | UE instance scan range |
| Scan interval (s) | 5 | Periodic re-discovery interval |
| Language | Follow system | Simplified Chinese / English; can be changed manually |

Closing the window only hides it back to the tray — the app keeps running.

---

## Architecture

```
AI Client ──POST /stream──► MCP HTTP Server (:6700)
                                    │
                             Dispatcher (JSON-RPC 2.0)
                                    │
                          UnrealManager (discover + WS)
                                    │
                ◄──── WebSocket JSON-RPC ──────► UE NexusLink
```

---

## Building Locally

### Prerequisites

- Go 1.25+
- GCC / MinGW-w64 (Windows) or Xcode CLI (macOS) — required by Fyne (CGO)

### Windows

```powershell
$env:CGO_ENABLED = "1"
go build -ldflags "-H=windowsgui -s -w" -o NexusDesktop.exe ./cmd/nexusdesktop/
```

Or use the one-click script:

```bat
build.bat
```

### macOS

One-click Universal Binary DMG (arm64 + amd64):

```bash
python3 scripts/build_desktop.py --build-type develop
# release build
python3 scripts/build_desktop.py --build-type release --arch universal
```

Or use the script shortcut:

```bash
./build.command
```

Manual single-arch or Universal build:

```bash
# current arch
CGO_ENABLED=1 go build -ldflags "-s -w" -o NexusDesktop ./cmd/nexusdesktop/

# Universal Binary (requires lipo)
CGO_ENABLED=1 GOARCH=arm64 go build -o NexusDesktop-arm64 ./cmd/nexusdesktop/
CGO_ENABLED=1 GOARCH=amd64 go build -o NexusDesktop-amd64 ./cmd/nexusdesktop/
lipo -create -output NexusDesktop NexusDesktop-arm64 NexusDesktop-amd64
```

> **Windows note**: GCC 16+ (binutils 2.46+) produces BigOBJ format which Go CGO does not support. Use GCC 14.x — e.g. [w64devkit v1.23.0](https://github.com/skeeto/w64devkit/releases/tag/v1.23.0). The release Setup.exe also needs [Inno Setup 7](https://jrsoftware.org/isdl.php) (`winget install JRSoftware.InnoSetup.7`).

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

---

## License

[MIT](LICENSE) © byteyang
