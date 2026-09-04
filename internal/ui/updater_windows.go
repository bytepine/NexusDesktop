//go:build windows

// Copyright byteyang. All Rights Reserved.

package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/bytepine/NexusDesktop/internal/config"
	"github.com/bytepine/NexusDesktop/internal/log"
)

const (
	appExeName      = "NexusDesktop.exe"
	uninstGUID      = `{B8E4D6A2-3C71-4F9E-A5B0-1D7C8E9F2A34}`
	uninstSubKey    = `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + uninstGUID + `_is1`
	updateHelperArg = "--nexus-apply-update"
)

// updateHelperParams 由旧进程写给新 exe 的替换参数（JSON，避免路径空格/引号问题）。
type updateHelperParams struct {
	Dest       string `json:"dest"`
	Pid        int    `json:"pid"`
	Version    string `json:"version"`
	ExtractDir string `json:"extractDir"`
	ZipPath    string `json:"zipPath"`
}

// ApplyInPlaceUpdate 下载 zip、解出 exe，启动无窗口助手后返回；调用方应随后退出。
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

	paramPath := filepath.Join(tmp, "NexusDesktop-update-"+latest+".json")
	raw, err := json.Marshal(updateHelperParams{
		Dest:       exe,
		Pid:        os.Getpid(),
		Version:    latest,
		ExtractDir: extractDir,
		ZipPath:    zipPath,
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(paramPath, raw, 0o600); err != nil {
		return err
	}

	elevated := needsElevation(exe)
	if elevated {
		log.Info("安装目录需要提权，将弹出 UAC")
	}
	if err := startUpdateHelper(newExe, paramPath, elevated); err != nil {
		return err
	}
	log.Infof("更新助手已启动，即将退出以替换 %s", exe)
	return nil
}

// runUpdateHelperIfRequested 若以助手参数启动则执行替换并返回 true（不进托盘）。
// 须在 AcquireLock 之前调用：助手不能抢锁，旧进程退出后才覆盖 exe。
func runUpdateHelperIfRequested() bool {
	if len(os.Args) < 3 || os.Args[1] != updateHelperArg {
		return false
	}
	log.Init(config.AppDir())
	if err := applyUpdateFromParams(os.Args[2]); err != nil {
		log.Errorf("静默更新失败: %v", err)
	}
	return true
}

func applyUpdateFromParams(paramPath string) error {
	raw, err := os.ReadFile(paramPath)
	if err != nil {
		return err
	}
	var p updateHelperParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	if !validHelperDest(p.Dest) {
		return fmt.Errorf("非法替换目标: %s", p.Dest)
	}
	waitForPidExit(p.Pid, 30*time.Second)
	src, err := os.Executable()
	if err != nil {
		return err
	}
	var copyErr error
	for i := 0; i < 8; i++ {
		copyErr = replaceFile(src, p.Dest)
		if copyErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if copyErr != nil {
		return copyErr
	}
	RepairDisplayVersion(p.Version)
	if err := startDetachedGUI(p.Dest); err != nil {
		return fmt.Errorf("启动新版本失败: %w", err)
	}
	_ = os.Remove(p.ZipPath)
	_ = os.Remove(paramPath)
	return nil
}

func validHelperDest(dest string) bool {
	return strings.EqualFold(filepath.Base(dest), appExeName)
}

func waitForPidExit(pid int, timeout time.Duration) {
	if pid <= 0 {
		return
	}
	deadline := time.Now().Add(timeout)
	for isProcessRunning(pid) && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
}

func replaceFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dest + ".updating"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	_ = os.Remove(dest)
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func startUpdateHelper(exe, paramPath string, elevated bool) error {
	if elevated {
		verb, err := windows.UTF16PtrFromString("runas")
		if err != nil {
			return err
		}
		file, err := windows.UTF16PtrFromString(exe)
		if err != nil {
			return err
		}
		args, err := windows.UTF16PtrFromString(updateHelperArg + " " + quoteWinArg(paramPath))
		if err != nil {
			return err
		}
		if err := windows.ShellExecute(0, verb, file, args, nil, windows.SW_HIDE); err != nil {
			return fmt.Errorf("提权启动更新助手失败: %w", err)
		}
		return nil
	}
	cmd := exec.Command(exe, updateHelperArg, paramPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新助手失败: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}

func startDetachedGUI(exe string) error {
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return nil
}

func quoteWinArg(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
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
