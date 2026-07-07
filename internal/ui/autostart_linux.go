//go:build linux

// Copyright byteyang. All Rights Reserved.

package ui

import (
	"fmt"
	"os"
	"path/filepath"
)

const desktopFileName = "nexusdesktop.desktop"

func autostartDir() string {
	cfgDir, _ := os.UserConfigDir()
	return filepath.Join(cfgDir, "autostart")
}

func desktopFilePath() string {
	return filepath.Join(autostartDir(), desktopFileName)
}

// IsAutostartEnabled 检查 ~/.config/autostart/nexusdesktop.desktop 是否存在。
func IsAutostartEnabled() bool {
	_, err := os.Stat(desktopFilePath())
	return err == nil
}

// SetAutostart 写入或删除 XDG autostart .desktop 文件。
func SetAutostart(enable bool) error {
	if !enable {
		return os.Remove(desktopFilePath())
	}
	exe, _ := os.Executable()
	_ = os.MkdirAll(autostartDir(), 0o755)
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=NexusDesktop
Exec=%s
Hidden=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
`, exe)
	return os.WriteFile(desktopFilePath(), []byte(content), 0o644)
}
