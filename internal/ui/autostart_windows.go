//go:build windows

// Copyright byteyang. All Rights Reserved.

package ui

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

const regRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const regValueName = "NexusDesktop"

// IsAutostartEnabled 检查开机自启是否已启用（Windows 注册表 Run 键）。
func IsAutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, regRunKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(regValueName)
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
		exe, _ := os.Executable()
		return k.SetStringValue(regValueName, `"`+exe+`"`)
	}
	return k.DeleteValue(regValueName)
}
