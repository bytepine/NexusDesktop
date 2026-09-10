// Copyright byteyang. All Rights Reserved.

package ui

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bytepine/NexusDesktop/internal/config"
	"github.com/bytepine/NexusDesktop/internal/log"
)

const (
	// 官方「最新 Release」重定向端点（非 REST API，无速率限制、无需 Auth）。
	releasesLatestURL  = "https://github.com/bytepine/NexusDesktop/releases/latest"
	releasesAtomURL    = "https://github.com/bytepine/NexusDesktop/releases.atom"
	releasesURL        = "https://github.com/bytepine/NexusDesktop/releases/latest"
	tagPrefix          = "nexus-desktop-v"
	githubDownloadBase = "https://github.com/bytepine/NexusDesktop/releases/download/"
	githubTagBase      = "https://github.com/bytepine/NexusDesktop/releases/tag/"
	// UpdateCheckInterval 后台复检间隔。
	UpdateCheckInterval = 6 * time.Hour
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
	LatestVersion string // 选中的目标版本号（不含前缀），如 "1.1.0"
	Error         string // 非空表示检查或应用失败（菜单/日志）
	UpToDate      bool   // 成功且无新版本
	Silent        bool   // 后台检查；失败时不覆盖已有菜单状态
}

var updateCheckInFlight atomic.Bool

// CheckUpdate 异步检查 GitHub 最新 Release，完成后调用 onDone。
func CheckUpdate(currentVersion string, onDone func(state UpdateState)) {
	startCheck(currentVersion, false, onDone)
}

// startCheck 发起一次检查；已有检查在途时返回 false 且不回调。
func startCheck(currentVersion string, silent bool, onDone func(state UpdateState)) bool {
	if onDone == nil {
		return false
	}
	if !updateCheckInFlight.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		defer updateCheckInFlight.Store(false)
		state := checkUpdateSync(currentVersion)
		state.Silent = silent
		onDone(state)
	}()
	return true
}

// StartPeriodicUpdateCheck 每 6 小时回调一次（启动检查由调用方另行触发）。
func StartPeriodicUpdateCheck(onTick func()) {
	if onTick == nil {
		return
	}
	go func() {
		t := time.NewTicker(UpdateCheckInterval)
		defer t.Stop()
		for range t.C {
			onTick()
		}
	}()
}

func checkUpdateSync(currentVersion string) UpdateState {
	current := strings.TrimSpace(currentVersion)
	ua := "NexusDesktop-UpdateChecker/" + current
	resolved := config.ResolveUpdateChannel(config.Get().UpdateChannel, current)

	stable, stableErr := fetchLatestStableTag(ua)
	var atom []string
	var atomErr error
	if resolved == config.UpdateChannelPre {
		atom, atomErr = fetchAtomTags(ua)
		if atomErr != nil {
			log.Debugf("检查更新：atom 失败: %v", atomErr)
		}
	}
	if stableErr != nil {
		log.Debugf("检查更新：latest 失败: %v", stableErr)
		if resolved != config.UpdateChannelPre || len(atom) == 0 {
			msg := "无法解析最新版本"
			if stableErr != nil {
				msg = stableErr.Error()
			} else if atomErr != nil {
				msg = atomErr.Error()
			}
			return UpdateState{Error: msg}
		}
	}

	picked := selectUpdate(current, resolved, stable, atom)
	log.Debugf("检查更新：当前 %s，渠道 %s→%s，latest %s，选中 %s",
		current, config.Get().UpdateChannel, resolved, stable, picked)

	if current == "" || current == "dev" {
		ver := picked
		if ver == "" {
			ver = stable
		}
		return UpdateState{LatestVersion: ver}
	}
	if picked == "" {
		ver := stable
		if ver == "" {
			ver = current
		}
		return UpdateState{UpToDate: true, LatestVersion: ver}
	}
	return UpdateState{HasUpdate: true, LatestVersion: picked}
}

func fetchLatestStableTag(ua string) (string, error) {
	if v := requestLatestLocation(http.MethodHead, ua); v != "" {
		return v, nil
	}
	if v := requestLatestLocation(http.MethodGet, ua); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("无法解析最新版本")
}

func requestLatestLocation(method, ua string) string {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest(method, releasesLatestURL, nil)
	if err != nil {
		return ""
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	} else {
		req.Header.Set("User-Agent", "NexusDesktop-UpdateChecker")
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	loc := resp.Header.Get("Location")
	if loc == "" {
		return parseLatestTagFromURL(resp.Request.URL.String())
	}
	base, err := url.Parse(releasesLatestURL)
	if err != nil {
		return parseLatestTagFromURL(loc)
	}
	resolved, err := base.Parse(loc)
	if err != nil {
		return parseLatestTagFromURL(loc)
	}
	return parseLatestTagFromURL(resolved.String())
}

type atomFeed struct {
	Entries []atomEntry `xml:"http://www.w3.org/2005/Atom entry"`
}

type atomEntry struct {
	Title string `xml:"http://www.w3.org/2005/Atom title"`
	Link  struct {
		Href string `xml:"href,attr"`
	} `xml:"http://www.w3.org/2005/Atom link"`
}

func fetchAtomTags(ua string) ([]string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, releasesAtomURL, nil)
	if err != nil {
		return nil, err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	} else {
		req.Header.Set("User-Agent", "NexusDesktop-UpdateChecker")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("atom HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	return parseAtomTags(raw)
}

func parseAtomTags(raw []byte) ([]string, error) {
	var feed atomFeed
	if err := xml.Unmarshal(raw, &feed); err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]struct{}{}
	for _, e := range feed.Entries {
		v := stripTagPrefix(e.Title)
		if !looksLikeVersion(v) {
			v = parseLatestTagFromURL(e.Link.Href)
		}
		if !looksLikeVersion(v) {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out, nil
}

func stripTagPrefix(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.TrimPrefix(s, tagPrefix)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	return s
}

func looksLikeVersion(v string) bool {
	if v == "" {
		return false
	}
	return v[0] >= '0' && v[0] <= '9'
}

// selectUpdate 按渠道从 latest + atom 里挑出严格新于 current 的最高版本；无则空串。
func selectUpdate(current, resolved, stableLatest string, atomTags []string) string {
	seen := map[string]struct{}{}
	var cands []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		cands = append(cands, v)
	}
	if resolved == config.UpdateChannelPre {
		add(stableLatest)
		for _, t := range atomTags {
			add(t)
		}
	} else if !config.IsPrerelease(stableLatest) {
		add(stableLatest)
	}

	current = strings.TrimSpace(current)
	var best string
	for _, c := range cands {
		if current == "" || current == "dev" {
			continue
		}
		if !IsNewerVersion(c, current) {
			continue
		}
		if best == "" || IsNewerVersion(c, best) {
			best = c
		}
	}
	return best
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
		return githubDownloadBase + tag + "/NexusDesktop-darwin-arm64-v" + v + "-update.zip"
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
		return "NexusDesktop-darwin-arm64-v" + v + "-update.zip"
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
	return stripTagPrefix(tag)
}

// IsNewerVersion 语义版本比较。
// 主段（X.Y.Z）不同时按数值比；主段相同时按 semver——正式版新于同主段预发布
// （2.0.0 > 2.0.0-beta.3）；预发布标识按点分段，数字段按整数（beta.10 > beta.9）。
// 构建元数据（'+' 后缀）忽略。返回 true 表示 A 比 B 新（A > B）。
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
	return comparePrerelease(sa, sb) > 0
}

func stripBuildMeta(v string) string {
	if i := strings.IndexByte(v, '+'); i >= 0 {
		return v[:i]
	}
	return v
}

// prereleaseSuffix 取 semver 预发布后缀（不含 '-'）；正式版或仅有 +build 返回空串。
func prereleaseSuffix(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	v = stripBuildMeta(v)
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[i+1:]
	}
	return ""
}

func parsePreNumeric(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func comparePrerelease(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		if i >= len(as) {
			return -1
		}
		if i >= len(bs) {
			return 1
		}
		na, aNum := parsePreNumeric(as[i])
		nb, bNum := parsePreNumeric(bs[i])
		switch {
		case aNum && bNum:
			if na == nb {
				continue
			}
			if na > nb {
				return 1
			}
			return -1
		case aNum && !bNum:
			return -1
		case !aNum && bNum:
			return 1
		default:
			if as[i] == bs[i] {
				continue
			}
			if as[i] > bs[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

func semverParts(v string) []int {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	v = stripBuildMeta(v)
	if i := strings.IndexByte(v, '-'); i >= 0 {
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
			break
		}
		out = append(out, n)
	}
	return out
}

// RunUpdateHelperIfRequested 若本进程是 Windows 静默更新助手则执行替换并返回 true。
// 必须在 AcquireLock / Fyne 启动之前调用。
func RunUpdateHelperIfRequested() bool {
	return runUpdateHelperIfRequested()
}

// CleanupUpdateTemp 清掉临时目录里 NexusDesktop-update-* 残留（助手无法删除自身所在目录）。
func CleanupUpdateTemp() {
	cleanupUpdateTempOnce()
	go func() {
		time.Sleep(3 * time.Second)
		cleanupUpdateTempOnce()
	}()
}

func cleanupUpdateTempOnce() {
	cleanupUpdateTempDir(os.TempDir())
}

func cleanupUpdateTempDir(tmp string) {
	matches, err := filepath.Glob(filepath.Join(tmp, "NexusDesktop-update-*"))
	if err != nil {
		return
	}
	for _, p := range matches {
		_ = os.RemoveAll(p)
	}
}
