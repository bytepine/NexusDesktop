// Copyright byteyang. All Rights Reserved.

package ui

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bytepine/NexusDesktop/internal/log"
)

const (
	// 官方「最新 Release」重定向端点（非 REST API，无速率限制、无需 Auth）。
	releasesLatestURL = "https://github.com/bytepine/NexusDesktop/releases/latest"
	releasesURL       = "https://github.com/bytepine/NexusDesktop/releases/latest"
	tagPrefix         = "nexus-desktop-v"
)

// UpdateState 记录检查更新的结果。
type UpdateState struct {
	Checking      bool   // 正在检查中
	HasUpdate     bool   // 发现新版本（latest > current）
	LatestVersion string // 最新版本号（不含前缀），如 "1.1.0"
	Error         string // 非空表示检查失败（仅日志/调试用）
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
