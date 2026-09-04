//go:build windows

// Copyright byteyang. All Rights Reserved.

package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidHelperDest(t *testing.T) {
	if !validHelperDest(`C:\Program Files\NexusDesktop\NexusDesktop.exe`) {
		t.Fatal("expected dest exe to be accepted")
	}
	if validHelperDest(`C:\Windows\System32\notepad.exe`) {
		t.Fatal("expected foreign exe to be rejected")
	}
}

func TestReplaceFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.exe")
	dest := filepath.Join(dir, "NexusDesktop.exe")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(src, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("dest = %q", got)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("src should remain after copy")
	}
}

func TestApplyUpdateFromParamsRejectsBadDest(t *testing.T) {
	dir := t.TempDir()
	param := filepath.Join(dir, "params.json")
	raw, _ := json.Marshal(updateHelperParams{
		Dest: filepath.Join(dir, "notepad.exe"),
		Pid:  0,
	})
	if err := os.WriteFile(param, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := applyUpdateFromParams(param); err == nil {
		t.Fatal("expected illegal dest to fail")
	}
}
