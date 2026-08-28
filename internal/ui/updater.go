// Copyright byteyang. All Rights Reserved.

package ui

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
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
	githubTagBase      = "https://github.com/bytepine/NexusDesktop/releases/tag/"
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

// githubReleasePage 返回指定版本的 GitHub Release 页；版本为空则打开 latest。
func githubReleasePage(version string) string {
	v := strings.TrimSpace(version)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if v == "" {
		return releasesURL
	}
	return githubTagBase + tagPrefix + v
}

func checksumsURLFor(version string) string {
	v := strings.TrimSpace(version)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if v == "" {
		return ""
	}
	return githubDownloadBase + tagPrefix + v + "/SHA256SUMS"
}

func updateZipNameFor(goos, version string) string {
	v := strings.TrimSpace(version)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	switch goos {
	case "windows":
		return "NexusDesktop-windows-amd64-v" + v + "-update.zip"
	case "darwin":
		return "NexusDesktop-darwin-universal-v" + v + "-update.zip"
	default:
		return ""
	}
}

func parseSHA256SUMS(body, filename string) (string, error) {
	want := strings.ToLower(filename)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if strings.EqualFold(filepath.Base(name), want) {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS 中没有 %s", filename)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func verifyUpdateZip(version, zipPath, userAgent string) error {
	sumsURL := checksumsURLFor(version)
	if sumsURL == "" {
		return fmt.Errorf("无法构造 SHA256SUMS URL")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", sumsURL, nil)
	if err != nil {
		return err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载 SHA256SUMS 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 SHA256SUMS 失败: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	want, err := parseSHA256SUMS(string(raw), filepath.Base(zipPath))
	if err != nil {
		name := updateZipNameFor(runtime.GOOS, version)
		want, err = parseSHA256SUMS(string(raw), name)
		if err != nil {
			return err
		}
	}
	got, err := fileSHA256(zipPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("更新包校验失败")
	}
	return nil
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

// IsNewerVersion 语义版本比较。
// 主段（X.Y.Z）不同时按数值比；主段相同时按 semver 规则——正式版新于同主段的预发布版
// （2.0.0 > 2.0.0-beta.3），两边都是预发布则按后缀字符串比。
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

	// 主段相同：无预发布后缀者更新（否则 beta 用户永远收不到同主段的正式版）
	sa := prereleaseSuffix(a)
	sb := prereleaseSuffix(b)
	if sa == sb {
		return false
	}
	if sa == "" {
		return true
	}
	if sb == "" {
		return false
	}
	return sa > sb
}

// prereleaseSuffix 取 semver 预发布/构建后缀（不含分隔符）；正式版返回空串。
func prereleaseSuffix(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		return v[i+1:]
	}
	return ""
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
