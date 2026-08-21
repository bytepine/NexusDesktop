//go:build darwin

// Copyright byteyang. All Rights Reserved.

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bytepine/NexusDesktop/internal/log"
)

// ApplyInPlaceUpdate 下载 zip、解出 .app，启动替换脚本后返回；调用方应随后退出。
func ApplyInPlaceUpdate(currentVersion, latestVersion string) error {
	if !SupportsInPlaceUpdate(currentVersion) {
		return ErrDevBuild
	}
	latest := strings.TrimSpace(latestVersion)
	if latest == "" {
		return fmt.Errorf("无效的目标版本")
	}

	appPath, err := currentAppBundle()
	if err != nil {
		return err
	}
	if strings.HasPrefix(appPath, "/Volumes/") {
		return ErrRunFromDMG
	}
	if !canReplaceApp(appPath) {
		return ErrRunFromDMG
	}

	tmp := os.TempDir()
	zipPath := filepath.Join(tmp, "NexusDesktop-update-"+latest+".zip")
	extractDir := filepath.Join(tmp, "NexusDesktop-update-"+latest)
	_ = os.RemoveAll(extractDir)

	ua := "NexusDesktop-UpdateChecker/" + currentVersion
	if err := downloadUpdateZip(latest, zipPath, ua); err != nil {
		return err
	}
	if err := verifyUpdateZip(latest, zipPath, ua); err != nil {
		return err
	}
	if err := extractZip(zipPath, extractDir); err != nil {
		return fmt.Errorf("解压更新包失败: %w", err)
	}
	newApp, err := findAppBundle(extractDir)
	if err != nil {
		return err
	}

	scriptPath := filepath.Join(tmp, "NexusDesktop-update-"+latest+".sh")
	script := fmt.Sprintf(`#!/bin/bash
sleep 2
rm -rf %s
mv %s %s
xattr -dr com.apple.quarantine %s || true
open %s
rm -rf %s
rm -f %s
rm -f "$0"
`, shellQuote(appPath), shellQuote(newApp), shellQuote(appPath),
		shellQuote(appPath), shellQuote(appPath),
		shellQuote(extractDir), shellQuote(zipPath))
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return err
	}

	cmd := exec.Command("/bin/bash", scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新脚本失败: %w", err)
	}
	log.Infof("更新脚本已启动，即将退出以替换 %s", appPath)
	return nil
}

func currentAppBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	macosDir := filepath.Dir(exe)
	if filepath.Base(macosDir) != "MacOS" {
		return "", fmt.Errorf("当前不在 .app bundle 内运行")
	}
	contents := filepath.Dir(macosDir)
	if filepath.Base(contents) != "Contents" {
		return "", fmt.Errorf("当前不在 .app bundle 内运行")
	}
	app := filepath.Dir(contents)
	if !strings.HasSuffix(app, ".app") {
		return "", fmt.Errorf("当前不在 .app bundle 内运行")
	}
	return app, nil
}

func canReplaceApp(appPath string) bool {
	f, err := os.CreateTemp(appPath, ".nexus-write-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
