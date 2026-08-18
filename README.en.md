# NexusDesktop

**Language / 语言**: [简体中文](README.md) · English

Standalone local MCP **proxy**: no IDE plugin. Run it; it lives in the system tray (menu bar on macOS), discovers local Unreal Engine instances, and forwards tool calls over WebSocket. Capabilities come from the UE **NexusLink** plugin.

Ports and switch layers: [NexusLink usage guide](https://github.com/bytepine/NexusLink/blob/master/docs/usage-guide.md). This app is the client layer of the **two-layer** switch (UE Enable MCP + tray Enable proxy).

---

## Requirements

| Component | Requirement |
|-----------|-------------|
| **NexusDesktop** | Download Setup.exe / `.dmg` — no Go / Node / runtime |
| **NexusLink** | [NexusLink Releases](https://github.com/bytepine/NexusLink/releases); UE 4.26+ |
| **Windows** | Windows 10 / 11 (amd64) |
| **macOS** | macOS 12+ (Monterey); Intel / Apple Silicon Universal |

---

## Download & install

From [Releases](https://github.com/bytepine/NexusDesktop/releases):

- **Windows**: `NexusDesktop-windows-amd64-v<version>-setup.exe` — installer; uninstall from Apps & features
- **macOS**: `NexusDesktop-darwin-universal.dmg` (Universal Binary)
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

Tray **Check for updates** downloads that version’s **zip update package** from GitHub Releases (not the installer), replaces files, and restarts:

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

**Windows**: after Setup, start from the Start menu; the app enters the tray.

**macOS**: open the dmg, drag `NexusDesktop.app` into `Applications`, then launch. It does **not** appear in the Dock — menu bar only.

| Tray item | Notes |
|-----------|-------|
| Status line | UE connection (project name / disconnected) |
| Select UE instance | Switch instance |
| Scan UE instances | Manual port scan |
| ✓ Enable proxy | Toggle MCP HTTP (default `:6700`) |
| Copy MCP client config | Copy JSON |
| Check for updates | Download zip update and restart |
| Settings… | Open settings |
| Open log directory | Open logs |
| Launch on login | Toggle autostart |
| Quit | Exit |

### 3. AI client

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

### 4. Settings window

Double-click the tray icon or **Settings…**. Closing the window hides it to the tray; the app keeps running.

| Setting | Default | Notes |
|---------|---------|-------|
| Enable proxy | On | Master switch |
| MCP HTTP port | 6700 | AI client port |
| UE scan start / end | 45000 / 45100 | Discovery range |
| Scan interval (s) | 5 | Periodic discovery |
| Language | Follow system | Simplified Chinese / English |

---

## Building locally

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
# release: python3 scripts/build_desktop.py --build-type release --arch universal
```

Or `./build.command`.

Record product changes in [CHANGELOG.md](CHANGELOG.md) `[Unreleased]`. Tag prefix: `nexus-desktop-v`.

---

## License

[MIT](LICENSE) © byteyang
