// Copyright byteyang. All Rights Reserved.

package log

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("write /dev/stdout: The handle is invalid.")
}

func TestStartupFilesAndRotation(t *testing.T) {
	oldLogger, oldFile, oldDir, oldPrefix, oldPart, oldLevel := logger, logFile, logDir, logPrefix, logPart, activeLevel
	dir := t.TempDir()
	t.Cleanup(func() {
		if logFile != nil && logFile != oldFile {
			_ = logFile.Close()
		}
		logger, logFile, logDir, logPrefix, logPart, activeLevel = oldLogger, oldFile, oldDir, oldPrefix, oldPart, oldLevel
	})
	// 模拟两个进程的首次 Init；不增加供测试专用的生产接口。
	logger, logFile, logPart = nil, nil, 0
	Init(dir)
	Info("first startup")
	first := logFile.Name()
	before, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	Init(dir)
	if logFile.Name() != first {
		t.Fatal("repeated Init in the same process must retain its current file")
	}
	_ = logFile.Close()
	logger, logFile, logPart = nil, nil, 0
	Init(dir)
	Info("second startup")
	second := logFile.Name()
	after, err := os.ReadFile(first)
	if err != nil || first == second || !bytes.Equal(before, after) {
		t.Fatal("a new startup must not overwrite or append to the previous log")
	}
	if err := logFile.Truncate(maxLogFileSize); err != nil {
		t.Fatal(err)
	}
	Info("after rotation")
	rotated, err := os.ReadFile(logFile.Name())
	if err != nil || logFile.Name() == second || !strings.Contains(string(rotated), "after rotation") {
		t.Fatal("rotation must continue in a new file")
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatal("rotation must preserve the previous file")
	}
}

func TestFileLogSurvivesInvalidStdout(t *testing.T) {
	var file bytes.Buffer
	w := io.MultiWriter(&file, bestEffortWriter{errWriter{}})
	line := []byte("2026-09-23 11:27:00.000 [INFO] NexusDesktop 启动\n")
	n, err := w.Write(line)
	if err != nil {
		t.Fatalf("write err: %v", err)
	}
	if n != len(line) {
		t.Fatalf("n=%d want %d", n, len(line))
	}
	if !bytes.Equal(file.Bytes(), line) {
		t.Fatalf("file=%q", file.Bytes())
	}
}
