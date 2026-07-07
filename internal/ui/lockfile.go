// Copyright byteyang. All Rights Reserved.

// 跨平台单实例锁：在 AppDir 下写 nexusdesktop.lock，包含当前 PID。
// 启动时若 lock 文件已存在且对应进程仍在运行，则视为重复启动。
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bytepine/NexusDesktop/internal/config"
)

const lockFileName = "nexusdesktop.lock"

// AcquireLock 尝试获取单实例锁。返回 true 表示成功（本进程是唯一实例）。
func AcquireLock() bool {
	path := filepath.Join(config.AppDir(), lockFileName)
	_ = os.MkdirAll(config.AppDir(), 0o755)

	// 读取已有锁文件
	if data, err := os.ReadFile(path); err == nil {
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		if pid > 0 && isProcessRunning(pid) {
			return false // 已有实例在运行
		}
	}

	// 写入当前 PID
	return os.WriteFile(path, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644) == nil
}

// ReleaseLock 释放单实例锁（退出时调用）。
func ReleaseLock() {
	path := filepath.Join(config.AppDir(), lockFileName)
	_ = os.Remove(path)
}
