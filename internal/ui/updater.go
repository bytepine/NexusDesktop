// Copyright byteyang. All Rights Reserved.

package ui

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/bytepine/NexusDesktop/internal/log"
)

const (
	releasesAPI = "https://api.github.com/repos/bytepine/NexusDesktop/releases/latest"
	releasesURL = "https://github.com/bytepine/NexusDesktop/releases/latest"
	tagPrefix   = "nexus-desktop-v"
)

// UpdateState 记录检查更新的结果。
type UpdateState struct {
	Checking      bool   // 正在检查中
	HasUpdate     bool   // 发现新版本
	LatestVersion string // 最新版本号（不含前缀），如 "1.1.0"
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
	req, err := http.NewRequest("GET", releasesAPI, nil)
	if err != nil {
		log.Debugf("检查更新：构建请求失败: %v", err)
		return UpdateState{}
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		log.Debugf("检查更新：请求失败: %v", err)
		return UpdateState{}
	}
	defer resp.Body.Close()

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		log.Debugf("检查更新：解析响应失败: %v", err)
		return UpdateState{}
	}

	latest := strings.TrimPrefix(payload.TagName, tagPrefix)
	if latest == "" {
		return UpdateState{}
	}

	log.Debugf("检查更新：当前 %s，最新 %s", currentVersion, latest)
	hasUpdate := latest != currentVersion && currentVersion != "dev"
	return UpdateState{HasUpdate: hasUpdate, LatestVersion: latest}
}
