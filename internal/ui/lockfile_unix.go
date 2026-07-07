//go:build darwin || linux

package ui

import "github.com/shirou/gopsutil/v3/process"

// isProcessRunning 检查指定 PID 的进程是否仍在运行（Unix 实现）。
func isProcessRunning(pid int) bool {
	exists, _ := process.PidExists(int32(pid))
	return exists
}
