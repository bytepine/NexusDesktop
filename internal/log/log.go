// Copyright byteyang. All Rights Reserved.

// Package log 提供简单的多输出日志封装：控制台 + 滚动文件日志。
// 接口与 nexus-vscode 的 logger 命名一致（Info/Warn/Error），方便对照参考实现。
package log

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxLogFileSize = 4 * 1024 * 1024 // 4MB 滚动阈值
	logFileName    = "nexusdesktop.log"
)

var (
	mu      sync.Mutex
	logFile *os.File
	logger  *log.Logger
	logDir  string
)

// Init 初始化日志，传入应用数据根目录（由 config 包提供）。
// 控制台 + 文件同时输出；可多次调用（幂等，仅首次有效）。
func Init(appDataDir string) {
	mu.Lock()
	defer mu.Unlock()
	if logger != nil {
		return
	}
	logDir = filepath.Join(appDataDir, "logs")
	_ = os.MkdirAll(logDir, 0o755)
	openLogFile()
}

func openLogFile() {
	path := filepath.Join(logDir, logFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		// 文件打开失败时仅用控制台
		logger = log.New(os.Stdout, "", 0)
		return
	}
	logFile = f
	w := io.MultiWriter(os.Stdout, f)
	logger = log.New(w, "", 0)
}

// LogDir 返回日志目录路径（Init 后才有值）。
func LogDir() string {
	mu.Lock()
	defer mu.Unlock()
	return logDir
}

func write(level, msg string) {
	mu.Lock()
	defer mu.Unlock()
	if logger == nil {
		// Init 未调用时先输出到 stdout
		fmt.Println(timestamp(), level, msg)
		return
	}
	// 检查文件大小，超限则滚动
	if logFile != nil {
		if fi, err := logFile.Stat(); err == nil && fi.Size() >= maxLogFileSize {
			rotateLog()
		}
	}
	logger.Printf("%s [%s] %s", timestamp(), level, msg)
}

func rotateLog() {
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
	// 将旧文件重命名
	old := filepath.Join(logDir, logFileName)
	bak := filepath.Join(logDir, fmt.Sprintf("nexusdesktop_%s.log", time.Now().Format("20060102_150405")))
	_ = os.Rename(old, bak)
	openLogFile()
}

func timestamp() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// Info 输出 INFO 级别日志。
func Info(msg string) { write("INFO", msg) }

// Infof 格式化 INFO 日志。
func Infof(format string, args ...any) { Info(fmt.Sprintf(format, args...)) }

// Warn 输出 WARN 级别日志。
func Warn(msg string) { write("WARN", msg) }

// Warnf 格式化 WARN 日志。
func Warnf(format string, args ...any) { Warn(fmt.Sprintf(format, args...)) }

// Error 输出 ERROR 级别日志。
func Error(msg string) { write("ERROR", msg) }

// Errorf 格式化 ERROR 日志。
func Errorf(format string, args ...any) { Error(fmt.Sprintf(format, args...)) }
