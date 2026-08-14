//go:build windows

// Copyright byteyang. All Rights Reserved.

package ui

import (
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/bytepine/NexusDesktop/internal/log"
)

// RepairDisplayVersion 把卸载项 DisplayVersion 写成当前 appVersion。
// 当前用户安装（HKCU）可直接写；全部用户（HKLM）无管理员权限时静默跳过，
// 由提权更新脚本在替换 exe 时一并写入。
func RepairDisplayVersion(version string) {
	v := strings.TrimSpace(version)
	if v == "" || v == "dev" {
		return
	}
	writeDisplayVersion(registry.CURRENT_USER, v)
	writeDisplayVersion(registry.LOCAL_MACHINE, v)
}

func writeDisplayVersion(root registry.Key, version string) {
	k, err := registry.OpenKey(root, uninstSubKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if err := k.SetStringValue("DisplayVersion", version); err != nil {
		log.Debugf("写入 DisplayVersion 失败: %v", err)
	}
}
