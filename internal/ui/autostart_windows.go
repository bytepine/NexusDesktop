//go:build windows

// Copyright byteyang. All Rights Reserved.

package ui

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/bytepine/NexusDesktop/internal/log"
)

const regRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const regValueName = "NexusDesktop"

// IsAutostartEnabled 检查开机自启是否已启用，且注册表中的 exe 路径仍然有效。
func IsAutostartEnabled() bool {
	path, ok := readAutostartPath()
	if !ok {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// SetAutostart 启用或禁用开机自启。
func SetAutostart(enable bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, regRunKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if enable {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		exe, _ = filepath.Abs(exe)
		return k.SetStringValue(regValueName, `"`+exe+`"`)
	}
	return k.DeleteValue(regValueName)
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
	if fileExists(path) && samePath(path, exe) {
		return
	}
	log.Warnf("开机自启路径已失效（%s），已更新为 %s", path, exe)
	if err := SetAutostart(true); err != nil {
		log.Errorf("修复开机自启失败: %v", err)
	}
}

func readAutostartPath() (string, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, regRunKey, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	val, _, err := k.GetStringValue(regValueName)
	if err != nil {
		return "", false
	}
	return strings.Trim(val, `"`), true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
