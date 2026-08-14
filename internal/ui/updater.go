// Copyright byteyang. All Rights Reserved.

package ui

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/bytepine/NexusDesktop/internal/log"
)

const (
	// 官方「最新 Release」重定向端点（非 REST API，无速率限制、无需 Auth）。
	releasesLatestURL  = "https://github.com/bytepine/NexusDesktop/releases/latest"
	releasesURL        = "https://github.com/bytepine/NexusDesktop/releases/latest"
	tagPrefix          = "nexus-desktop-v"
	githubDownloadBase = "https://github.com/bytepine/NexusDesktop/releases/download/"
)

var (
	// ErrRunFromDMG 当前从只读磁盘映像运行，无法覆盖 .app。
	ErrRunFromDMG = errors.New("running from read-only disk image")
	// ErrUnsupportedOS 当前平台不支持应用内替换。
	ErrUnsupportedOS = errors.New("in-place update not supported on this OS")
	// ErrDevBuild 开发构建不参与应用内替换。
	ErrDevBuild = errors.New("dev build cannot apply in-place update")
)

// UpdateState 记录检查更新的结果。
type UpdateState struct {
	Checking      bool   // 正在检查中
	Downloading   bool   // 正在下载/应用更新包
	HasUpdate     bool   // 发现新版本（latest > current）
	LatestVersion string // 最新版本号（不含前缀），如 "1.1.0"
	Error         string // 非空表示检查或应用失败（菜单/日志）
}

// CheckUpdate 异步检查 GitHub 最新 Release，完成后调用 onDone。
func CheckUpdate(currentVersion string, onDone func(state UpdateState)) {
	go func() {
		state := checkUpdateSync(currentVersion)
		onDone(state)
	}()
}

func checkUpdateSync(currentVersion string) UpdateState {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", releasesLatestURL, nil)
	if err != nil {
		log.Debugf("检查更新：构建请求失败: %v", err)
		return UpdateState{Error: err.Error()}
	}
	// GitHub 对无 UA 的机器人请求常直接 403；与 NexusLink 一致带上标识。
	req.Header.Set("User-Agent", "NexusDesktop-UpdateChecker/"+currentVersion)

	resp, err := client.Do(req)
	if err != nil {
		log.Debugf("检查更新：请求失败: %v", err)
		return UpdateState{Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		msg := "HTTP " + strconv.Itoa(resp.StatusCode)
		log.Debugf("检查更新：%s", msg)
		return UpdateState{Error: msg}
	}

	// 跟随 302 后落地 URL：.../releases/tag/nexus-desktop-vX.Y.Z
	latest := parseLatestTagFromURL(resp.Request.URL.String())
	if latest == "" {
		log.Debugf("检查更新：无法从落地 URL 解析版本: %s", resp.Request.URL.String())
		return UpdateState{Error: "无法解析最新版本"}
	}

	current := strings.TrimSpace(currentVersion)
	log.Debugf("检查更新：当前 %s，最新 %s", current, latest)

	// 开发构建（未注入 -X）不参与比较提示；其余用 semver 判断是否有更新。
	hasUpdate := current != "" && current != "dev" && IsNewerVersion(latest, current)
	return UpdateState{HasUpdate: hasUpdate, LatestVersion: latest}
}

// SupportsInPlaceUpdate 是否走应用内 zip 替换（dev / 非 Win/Mac 为 false）。
func SupportsInPlaceUpdate(currentVersion string) bool {
	v := strings.TrimSpace(currentVersion)
	if v == "" || v == "dev" {
		return false
	}
	switch runtime.GOOS {
	case "windows", "darwin":
		return true
	default:
		return false
	}
}

func updateZipURLFor(goos, version string) string {
	v := strings.TrimSpace(version)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if v == "" {
		return ""
	}
	tag := tagPrefix + v
	switch goos {
	case "windows":
		return githubDownloadBase + tag + "/NexusDesktop-windows-amd64-v" + v + "-update.zip"
	case "darwin":
		return githubDownloadBase + tag + "/NexusDesktop-darwin-universal-v" + v + "-update.zip"
	default:
		return ""
	}
}

func downloadUpdateZip(version, dest, userAgent string) error {
	url := updateZipURLFor(runtime.GOOS, version)
	if url == "" {
		return ErrUnsupportedOS
	}
	log.Infof("下载更新包: %s", url)
	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	if userAgent == "" {
		userAgent = "NexusDesktop-UpdateChecker"
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func zipSlipSafe(destDir, name string) (string, error) {
	cleaned := filepath.Clean(name)
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("非法 zip 路径: %s", name)
	}
	target := filepath.Join(destDir, cleaned)
	rel, err := filepath.Rel(destDir, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("非法 zip 路径: %s", name)
	}
	return target, nil
}

func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		target, err := zipSlipSafe(destDir, f.Name)
		if err != nil {
			return err
		}
		mode := f.Mode()
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func findNamedFile(root, name string) (string, error) {
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.EqualFold(info.Name(), name) {
			found = path
			return io.EOF
		}
		return nil
	})
	if found != "" {
		return found, nil
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return "", fmt.Errorf("更新包中未找到 %s", name)
}

func findAppBundle(root string) (string, error) {
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".app") {
			found = path
			return io.EOF
		}
		return nil
	})
	if found != "" {
		return found, nil
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return "", fmt.Errorf("更新包中未找到 .app")
}

// parseLatestTagFromURL 从 Release 落地页 URL 提取版本号（去掉 nexus-desktop-v / 前导 v）。
func parseLatestTagFromURL(rawURL string) string {
	const marker = "/releases/tag/"
	idx := strings.Index(strings.ToLower(rawURL), marker)
	if idx < 0 {
		return ""
	}
	tag := rawURL[idx+len(marker):]
	if end := strings.IndexAny(tag, "?#"); end >= 0 {
		tag = tag[:end]
	}
	tag = strings.TrimSpace(tag)
	tag = strings.TrimPrefix(tag, tagPrefix)
	tag = strings.TrimPrefix(tag, "v")
	tag = strings.TrimPrefix(tag, "V")
	return tag
}

// IsNewerVersion 语义版本比较（"X.Y.Z"，忽略 -beta 等后缀的主段）。
// 返回 true 表示 A 比 B 新（A > B）。
func IsNewerVersion(a, b string) bool {
	pa := semverParts(a)
	pb := semverParts(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		va, vb := 0, 0
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va != vb {
			return va > vb
		}
	}
	return false
}

func semverParts(v string) []int {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	// 截断预发布后缀：1.2.0-beta.1 → 1.2.0
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	segs := strings.Split(v, ".")
	out := make([]int, 0, len(segs))
	for _, s := range segs {
		if s == "" {
			continue
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			// 非纯数字段停止（避免把垃圾解析成 0）
			break
		}
		out = append(out, n)
	}
	return out
}
