//go:build darwin

// Copyright byteyang. All Rights Reserved.

package ui

import (
	"fmt"
	"os"
	"path/filepath"
)

const plistName = "com.bytepine.nexusdesktop.plist"

func launchAgentDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents")
}

func plistPath() string {
	return filepath.Join(launchAgentDir(), plistName)
}

// IsAutostartEnabled 检查 LaunchAgent plist 是否存在。
func IsAutostartEnabled() bool {
	_, err := os.Stat(plistPath())
	return err == nil
}

// SetAutostart 通过 LaunchAgent plist 启用/禁用开机自启。
func SetAutostart(enable bool) error {
	if !enable {
		return os.Remove(plistPath())
	}
	exe, _ := os.Executable()
	_ = os.MkdirAll(launchAgentDir(), 0o755)
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>`, plistName, exe)
	return os.WriteFile(plistPath(), []byte(content), 0o644)
}
