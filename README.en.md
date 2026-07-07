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
| **NexusDesktop** | Download `.exe` / `.dmg` — no Go / Node / runtime needed |
| **NexusLink** (UE plugin) | [NexusLink Releases](https://github.com/bytepine/NexusLink/releases); UE 4.26+ |
| **Windows** | Windows 10 / 11 (amd64) |
| **macOS** | macOS 12+ (Monterey); Intel & Apple Silicon Universal Binary |

---

## Download

Get the latest release from [Releases](https://github.com/bytepine/NexusDesktop/releases):

- **Windows**: `NexusDesktop-windows-amd64.exe` — no installer, double-click to run
- **macOS**: `NexusDesktop-darwin-universal.dmg` — Universal Binary (Intel + Apple Silicon)

---

## Usage

### 1. UE Prerequisites

1. Download `nexus-mcp-unreal-*.zip` from [NexusLink Releases](https://github.com/bytepine/NexusLink/releases) and extract to `Plugins/Developer/NexusLink`
2. UE: **Edit → Plugins → Developer → NexusLink** — enable the plugin
3. UE: **Edit → Editor Preferences → Plugins → NexusLink** — check **Enable MCP Server**

### 2. Launch NexusDesktop

**Windows**: Double-click `NexusDesktop.exe` — the app enters the system tray.

**macOS**: Open `NexusDesktop-darwin-universal.dmg`, drag `NexusDesktop.app` into `Applications`, then launch it. The app does **not** appear in the Dock — it lives in the menu bar only.

Tray menu items:

| Item | Description |
|------|-------------|
| Status line | Shows current UE connection state (project name / disconnected) |
| Select UE instance | Switch to a specific UE instance |
| ✓ Enable proxy | Toggle MCP HTTP listener (default `:6700`) |
| Copy MCP client config | Copy JSON snippet to clipboard |
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

- Go 1.24+
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

> **Windows note**: GCC 16+ (binutils 2.46+) produces BigOBJ format which Go CGO does not support. Use GCC 14.x — e.g. [w64devkit v1.23.0](https://github.com/skeeto/w64devkit/releases/tag/v1.23.0).

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

---

## License

[MIT](LICENSE) © byteyang
