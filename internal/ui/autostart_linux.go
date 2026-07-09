//go:build linux

// Copyright byteyang. All Rights Reserved.

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bytepine/NexusDesktop/internal/log"
)

const desktopFileName = "nexusdesktop.desktop"

func autostartDir() string {
	cfgDir, _ := os.UserConfigDir()
	return filepath.Join(cfgDir, "autostart")
}

func desktopFilePath() string {
	return filepath.Join(autostartDir(), desktopFileName)
}

// IsAutostartEnabled 检查 XDG autostart 文件是否存在，且 Exec 路径有效。
func IsAutostartEnabled() bool {
	path, ok := readAutostartPath()
	if !ok {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// SetAutostart 写入或删除 XDG autostart .desktop 文件。
func SetAutostart(enable bool) error {
	if !enable {
		return os.Remove(desktopFilePath())
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.Abs(exe)
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

// RepairAutostart 若用户曾启用自启但路径失效/已迁移，则重写为当前可执行文件。
func RepairAutostart() {
	path, ok := readAutostartPath()
	if !ok {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	exe, _ = filepath.Abs(exe)
	if fileExists(path) && filepath.Clean(path) == filepath.Clean(exe) {
		return
	}
	log.Warnf("开机自启路径已失效（%s），已更新为 %s", path, exe)
	if err := SetAutostart(true); err != nil {
		log.Errorf("修复开机自启失败: %v", err)
	}
}

func readAutostartPath() (string, bool) {
	data, err := os.ReadFile(desktopFilePath())
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Exec=") {
			return strings.TrimPrefix(line, "Exec="), true
		}
	}
	return "", false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
