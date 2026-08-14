// Copyright byteyang. All Rights Reserved.

// Package i18n 提供界面文案的简体中文 / 英文切换。
// 配置 language=auto（或缺省）时跟随系统语言；其余为手动覆盖。
package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync"

	golocale "github.com/jeandeaual/go-locale"
)

const (
	// LangAuto 跟随系统（默认）。
	LangAuto = "auto"
	// LangZhCN 简体中文。
	LangZhCN = "zh-CN"
	// LangEn 英语。
	LangEn = "en"
)

var (
	mu      sync.RWMutex
	current = LangZhCN
)

// Apply 根据用户偏好生效语言：auto 走系统检测，其余为手动覆盖。
func Apply(pref string) {
	lang := Resolve(pref)
	mu.Lock()
	current = lang
	mu.Unlock()
}

// Current 返回当前生效语言（zh-CN 或 en）。
func Current() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Resolve 将配置值解析为实际语言标签。
func Resolve(pref string) string {
	switch pref {
	case LangZhCN, LangEn:
		return pref
	default:
		return DetectSystem()
	}
}

// T 取当前语言文案；key 缺失时回退英文。args 非空则 fmt.Sprintf。
func T(key string, args ...any) string {
	mu.RLock()
	lang := current
	mu.RUnlock()
	s, ok := catalogs[lang][key]
	if !ok {
		s = catalogs[LangEn][key]
	}
	if s == "" {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// DetectSystem 根据 OS 界面语言选择 zh-CN 或 en。
func DetectSystem() string {
	loc, err := golocale.GetLocale()
	if err != nil || loc == "" {
		loc = os.Getenv("LC_ALL")
		if loc == "" {
			loc = os.Getenv("LANG")
		}
	}
	return Normalize(loc)
}

// Normalize 将 BCP-47 / LANG 值映射到支持的语言；非中文一律英文。
func Normalize(tag string) string {
	t := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(tag), "_", "-"))
	if i := strings.IndexByte(t, '.'); i >= 0 {
		t = t[:i]
	}
	if t == "" {
		return LangEn
	}
	if t == "zh" || strings.HasPrefix(t, "zh-") {
		return LangZhCN
	}
	return LangEn
}

// PrefFromSelect 将设置页下拉选项映射为配置值。
func PrefFromSelect(label string) string {
	if label == "简体中文" {
		return LangZhCN
	}
	if label == "English" {
		return LangEn
	}
	return LangAuto
}

// SelectOptions 返回语言下拉的显示项（跟随系统文案随当前语言变化）。
func SelectOptions() []string {
	return []string{T("settings.lang_auto"), "简体中文", "English"}
}

// SelectIndex 返回偏好对应的下拉下标。
func SelectIndex(pref string) int {
	switch pref {
	case LangZhCN:
		return 1
	case LangEn:
		return 2
	default:
		return 0
	}
}
