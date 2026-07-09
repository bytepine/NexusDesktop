//go:build darwin

// Copyright byteyang. All Rights Reserved.

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bytepine/NexusDesktop/internal/log"
)

const plistName = "com.bytepine.nexusdesktop.plist"

func launchAgentDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents")
}

func plistPath() string {
	return filepath.Join(launchAgentDir(), plistName)
}

// IsAutostartEnabled 检查 LaunchAgent plist 是否存在，且 ProgramArguments 中的路径有效。
func IsAutostartEnabled() bool {
	path, ok := readAutostartPath()
	if !ok {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// SetAutostart 通过 LaunchAgent plist 启用/禁用开机自启。
func SetAutostart(enable bool) error {
	if !enable {
		return os.Remove(plistPath())
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.Abs(exe)
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
	data, err := os.ReadFile(plistPath())
	if err != nil {
		return "", false
	}
	// 简易解析：取 ProgramArguments 数组中第一个 <string>...</string>
	const marker = "<key>ProgramArguments</key>"
	idx := strings.Index(string(data), marker)
	if idx < 0 {
		return "", false
	}
	rest := string(data)[idx+len(marker):]
	start := strings.Index(rest, "<string>")
	end := strings.Index(rest, "</string>")
	if start < 0 || end < 0 || end <= start {
		return "", false
	}
	return rest[start+len("<string>"):end], true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
