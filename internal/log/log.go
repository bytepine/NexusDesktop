// Copyright byteyang. All Rights Reserved.

// Package log 提供带级别过滤的多输出日志封装：控制台 + 滚动文件日志。
//
// 日志级别（由低到高）：debug < info < warn < error
//
// Level 变量在构建时由链接器注入：
//
//	develop 包：-X github.com/bytepine/NexusDesktop/internal/log.Level=debug
//	release 包：-X github.com/bytepine/NexusDesktop/internal/log.Level=info
package log

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	maxLogFileSize = 4 * 1024 * 1024 // 4MB 滚动阈值
	logFileName    = "nexusdesktop.log"
)

// Level 由构建脚本通过 -X 注入，决定最低输出级别。
// 有效值："debug" | "info" | "warn" | "error"；默认 debug（develop 包）。
var Level = "debug"

type lvl int

const (
	lvlDebug lvl = iota
	lvlInfo
	lvlWarn
	lvlError
)

var levelNames = map[lvl]string{
	lvlDebug: "DEBUG",
	lvlInfo:  "INFO",
	lvlWarn:  "WARN",
	lvlError: "ERROR",
}

func parseLevel(s string) lvl {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return lvlDebug
	case "warn", "warning":
		return lvlWarn
	case "error":
		return lvlError
	default:
		return lvlInfo
	}
}

var (
	mu         sync.Mutex
	logFile    *os.File
	logger     *log.Logger
	logDir     string
	activeLevel lvl = lvlDebug
)

// Init 初始化日志，传入应用数据根目录。
// 控制台 + 文件同时输出；可多次调用（幂等，仅首次有效）。
func Init(appDataDir string) {
	mu.Lock()
	defer mu.Unlock()
	if logger != nil {
		return
	}
	activeLevel = parseLevel(Level)
	logDir = filepath.Join(appDataDir, "logs")
	_ = os.MkdirAll(logDir, 0o755)
	openLogFile()
}

func openLogFile() {
	path := filepath.Join(logDir, logFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
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

// CurrentLevel 返回当前生效的日志级别字符串。
func CurrentLevel() string {
	mu.Lock()
	defer mu.Unlock()
	return levelNames[activeLevel]
}

func write(l lvl, msg string) {
	mu.Lock()
	defer mu.Unlock()
	if l < activeLevel {
		return
	}
	line := fmt.Sprintf("%s [%s] %s", timestamp(), levelNames[l], msg)
	if logger == nil {
		fmt.Println(line)
		return
	}
	// 检查文件大小，超限则滚动
	if logFile != nil {
		if fi, err := logFile.Stat(); err == nil && fi.Size() >= maxLogFileSize {
			rotateLog()
		}
	}
	logger.Print(line)
}

func rotateLog() {
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
	old := filepath.Join(logDir, logFileName)
	bak := filepath.Join(logDir, fmt.Sprintf("nexusdesktop_%s.log", time.Now().Format("20060102_150405")))
	_ = os.Rename(old, bak)
	openLogFile()
}

func timestamp() string {
	return time.Now().Format("2006-01-02 15:04:05.000")
}

// Debug 输出 DEBUG 级别日志（develop 包可见）。
func Debug(msg string) { write(lvlDebug, msg) }

// Debugf 格式化 DEBUG 日志。
func Debugf(format string, args ...any) { Debug(fmt.Sprintf(format, args...)) }

// Info 输出 INFO 级别日志。
func Info(msg string) { write(lvlInfo, msg) }

// Infof 格式化 INFO 日志。
func Infof(format string, args ...any) { Info(fmt.Sprintf(format, args...)) }

// Warn 输出 WARN 级别日志。
func Warn(msg string) { write(lvlWarn, msg) }

// Warnf 格式化 WARN 日志。
func Warnf(format string, args ...any) { Warn(fmt.Sprintf(format, args...)) }

// Error 输出 ERROR 级别日志。
func Error(msg string) { write(lvlError, msg) }

// Errorf 格式化 ERROR 日志。
func Errorf(format string, args ...any) { Error(fmt.Sprintf(format, args...)) }
