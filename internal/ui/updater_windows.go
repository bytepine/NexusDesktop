//go:build windows

// Copyright byteyang. All Rights Reserved.

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bytepine/NexusDesktop/internal/log"
)

const (
	appExeName     = "NexusDesktop.exe"
	uninstGUID     = `{B8E4D6A2-3C71-4F9E-A5B0-1D7C8E9F2A34}`
	uninstSubKey   = `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + uninstGUID + `_is1`
	uninstRegPath  = `HKCU\` + uninstSubKey
	uninstRegPathM = `HKLM\` + uninstSubKey
)

// ApplyInPlaceUpdate 下载 zip、解出 exe，启动替换脚本后返回；调用方应随后退出。
func ApplyInPlaceUpdate(currentVersion, latestVersion string) error {
	if !SupportsInPlaceUpdate(currentVersion) {
		return ErrDevBuild
	}
	latest := sanitizeVersion(latestVersion)
	if latest == "" {
		return fmt.Errorf("无效的目标版本")
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.Abs(exe)

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
	newExe, err := findNamedFile(extractDir, appExeName)
	if err != nil {
		return err
	}

	batPath := filepath.Join(tmp, "NexusDesktop-update-"+latest+".bat")
	if err := writeUpdateBat(batPath, newExe, exe, latest, extractDir, zipPath); err != nil {
		return err
	}

	elevated := needsElevation(exe)
	if elevated {
		log.Info("安装目录需要提权，将弹出 UAC")
	}
	if err := startDetachedBat(batPath, elevated); err != nil {
		return err
	}
	log.Infof("更新脚本已启动，即将退出以替换 %s", exe)
	return nil
}

func needsElevation(destExe string) bool {
	dir := filepath.Dir(destExe)
	f, err := os.CreateTemp(dir, ".nexus-write-*")
	if err != nil {
		return true
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return false
}

func writeUpdateBat(batPath, srcExe, destExe, version, extractDir, zipPath string) error {
	src := batQuote(srcExe)
	dest := batQuote(destExe)
	ext := batQuote(extractDir)
	zp := batQuote(zipPath)
	ver := sanitizeVersion(version)
	body := fmt.Sprintf(`@echo off
chcp 65001 >nul
timeout /t 2 /nobreak >nul
set RETRIES=8
:retry
move /Y %s %s >nul 2>&1
if not errorlevel 1 goto moved
set /a RETRIES-=1
if %%RETRIES%% leq 0 goto fail
timeout /t 1 /nobreak >nul
goto retry
:moved
reg add "%s" /v DisplayVersion /d "%s" /f >nul 2>&1
reg add "%s" /v DisplayVersion /d "%s" /f >nul 2>&1
rd /s /q %s >nul 2>&1
del /f /q %s >nul 2>&1
start "" %s
del "%%~f0"
exit /b 0
:fail
exit /b 1
`, src, dest, uninstRegPath, ver, uninstRegPathM, ver, ext, zp, dest)
	return os.WriteFile(batPath, append([]byte{0xEF, 0xBB, 0xBF}, []byte(body)...), 0o755)
}

func startDetachedBat(batPath string, elevated bool) error {
	if elevated {
		ps := fmt.Sprintf(
			"Start-Process -FilePath 'cmd.exe' -Verb RunAs -ArgumentList '/c', '%s'",
			strings.ReplaceAll(batPath, "'", "''"),
		)
		cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", ps)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("提权启动更新脚本失败: %v (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	cmd := exec.Command("cmd", "/C", "start", "", "/MIN", batPath)
	return cmd.Run()
}

func batQuote(s string) string {
	return `"` + s + `"`
}

func sanitizeVersion(v string) string {
	v = strings.TrimSpace(v)
	var b strings.Builder
	for _, r := range v {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '.' || r == '-' || r == '+' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
