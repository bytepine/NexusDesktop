// Copyright byteyang. All Rights Reserved.

package log

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("write /dev/stdout: The handle is invalid.")
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
