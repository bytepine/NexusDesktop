// Copyright byteyang. All Rights Reserved.

// Package proxy 实现代理会话层（TTL 缓存 / degraded / 写门控），与 docs/proxy-session.md 对齐。
package proxy

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

type WriteGateMode string

const (
	GateOff         WriteGateMode = "off"
	GateDestructive WriteGateMode = "destructive"
	GateAll         WriteGateMode = "all"
)

type CacheKind string

const (
	CacheHit        CacheKind = "hit"
	CacheSectionHit CacheKind = "section_hit"
)

type GateDecision string

const (
	DecisionAllow  GateDecision = "allow"
	DecisionDeny   GateDecision = "deny"
	DecisionAlways GateDecision = "always"
)

const (
	SectionWindowMS      = 30_000
	DefaultVolatileTTLMS = 8_000
	DefaultReadTTLMS     = 30_000
	DefaultSearchTTLMS   = 60_000
	MaxCacheEntries      = 64
	OffloadChars         = 48_000
	GateTimeoutMS        = 120_000
	DegradedNote         = "UE editor unreachable (compile/restart?). Serving last snapshot. Do not loop list_unreal_instances."
)

var destructiveCaps = map[string]struct{}{"DeleteAsset": {}, "RenameAsset": {}}
var stopPie = map[string]struct{}{"stop": {}, "end": {}, "quit": {}, "endplay": {}, "end_play": {}, "end-play": {}}

type CallInfo struct {
	ToolName    string
	Capability  string
	InnerArgs   map[string]interface{}
	Identity    string
	Sections    []string
	ExactKey    string
	IdentityKey string
	CacheScope  string
	IsWrite     bool
	IsBatch     bool
}

func ParseWriteGate(raw string) WriteGateMode {
	switch raw {
	case "off", "all", "destructive":
		return WriteGateMode(raw)
	default:
		return GateDestructive
	}
}

func IsWriteCapability(cap string) bool {
	if cap == "" || cap == "SubmitFeedback" || cap == "SearchCapabilities" {
		return false
	}
	if strings.HasPrefix(cap, "Get") || strings.HasPrefix(cap, "List") || strings.HasPrefix(cap, "Search") {
		return false
	}
	return true
}

func IsDestructive(cap string, inner map[string]interface{}) bool {
	if _, ok := destructiveCaps[cap]; ok {
		return true
	}
	if cap == "ControlPie" {
		action, _ := inner["Action"].(string)
		_, hit := stopPie[strings.ToLower(action)]
		return hit
	}
	if strings.HasPrefix(cap, "Manage") {
		ops, _ := inner["Operations"].([]interface{})
		for _, op := range ops {
			m, _ := op.(map[string]interface{})
			action, _ := m["Action"].(string)
			a := strings.ToLower(action)
			if strings.Contains(a, "delete") || strings.Contains(a, "remove") || a == "destroy" {
				return true
			}
		}
	}
	return false
}

func NeedsGate(mode WriteGateMode, cap string, inner map[string]interface{}) bool {
	switch mode {
	case GateOff:
		return false
	case GateAll:
		return IsWriteCapability(cap)
	default:
		return IsDestructive(cap, inner)
	}
}

func IsVolatileCap(cap string) bool {
	return cap == "GetOutputLog" || cap == "CaptureViewport" ||
		strings.HasPrefix(cap, "ListRuntime") || strings.HasPrefix(cap, "GetRuntime")
}

func IsDurableRead(cap string) bool {
	if IsWriteCapability(cap) || IsVolatileCap(cap) {
		return false
	}
	return strings.HasPrefix(cap, "Search") || strings.HasPrefix(cap, "GetAsset") ||
		strings.HasPrefix(cap, "GetEditor") || cap == "GetGameplayTags" ||
		cap == "GetAssetRefs" || cap == "GetAssetLuaBinding"
}

func DefaultTtlMS(cap string) int64 {
	if IsVolatileCap(cap) {
		return DefaultVolatileTTLMS
	}
	if strings.HasPrefix(cap, "Search") {
		return DefaultSearchTTLMS
	}
	return DefaultReadTTLMS
}

func extractIdentity(args map[string]interface{}) string {
	for _, key := range []string{"AssetPath", "ActorName", "WidgetName", "LuaPath", "ScriptPath"} {
		if v, ok := args[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func extractSections(args map[string]interface{}) []string {
	raw, ok := args["Sections"].([]interface{})
	if !ok {
		return nil
	}
	var out []string
	for _, v := range raw {
		s, _ := v.(string)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func SectionsCovered(cached, requested []string) bool {
	if len(cached) == 0 {
		return false
	}
	for _, s := range cached {
		if s == "All" {
			return true
		}
	}
	if len(requested) == 0 {
		return false
	}
	set := map[string]struct{}{}
	for _, s := range cached {
		set[s] = struct{}{}
	}
	for _, s := range requested {
		if _, ok := set[s]; !ok {
			return false
		}
	}
	return true
}

// ExplicitTargetPort 与三端路由统一：只接受合法整数端口。
func ExplicitTargetPort(args map[string]interface{}) int {
	port, ok := args["targetPort"].(float64)
	if !ok || port < 1024 || port > 65535 || math.IsNaN(port) || math.Trunc(port) != port {
		return -1
	}
	return int(port)
}

func ParseCall(toolName string, args map[string]interface{}, instanceScope ...string) CallInfo {
	if args == nil {
		args = map[string]interface{}{}
	}
	capability := toolName
	scope := ""
	if len(instanceScope) > 0 {
		scope = instanceScope[0]
	}
	if port := ExplicitTargetPort(args); port > 0 {
		host, _ := args["targetHost"].(string)
		host = strings.TrimSpace(host)
		if host == "" || strings.EqualFold(host, "localhost") || host == "::1" {
			host = "127.0.0.1"
		}
		scope = host + ":" + strconv.Itoa(port)
	}
	// 仅剥离顶层路由字段；嵌套能力参数仍参与缓存键。
	inner := make(map[string]interface{}, len(args))
	for k, v := range args {
		if k != "targetPort" && k != "targetHost" {
			inner[k] = v
		}
	}
	isBatch := false
	if toolName == "CallCapability" {
		if _, ok := args["Calls"].([]interface{}); ok {
			isBatch = true
			capability = "CallCapability.Calls"
		} else {
			capability, _ = args["Capability"].(string)
			if m, ok := args["Arguments"].(map[string]interface{}); ok {
				inner = m
			} else {
				inner = map[string]interface{}{}
			}
		}
	}
	ident := extractIdentity(inner)
	sections := extractSections(inner)
	raw, _ := json.Marshal(inner)
	exact, _ := json.Marshal([]string{scope, toolName, capability, string(raw)})
	identityKey, _ := json.Marshal([]string{scope, capability, ident})
	return CallInfo{
		ToolName: toolName, Capability: capability, InnerArgs: inner,
		Identity: ident, Sections: sections, ExactKey: string(exact), CacheScope: scope,
		IdentityKey: string(identityKey),
		IsWrite:     isBatch || IsWriteCapability(capability),
		IsBatch:     isBatch,
	}
}

func resultTextObject(result interface{}) map[string]interface{} {
	rec, ok := result.(map[string]interface{})
	if !ok {
		return nil
	}
	content, _ := rec["content"].([]interface{})
	if len(content) == 0 {
		return nil
	}
	first, _ := content[0].(map[string]interface{})
	text, _ := first["text"].(string)
	var obj map[string]interface{}
	if json.Unmarshal([]byte(text), &obj) != nil {
		return nil
	}
	return obj
}

func ExtractTtlMS(result interface{}, cap string, nowMS int64) (ttlMS int64, snapshotISO string) {
	ttlMS = DefaultTtlMS(cap)
	snapshotISO = ""
	src := resultTextObject(result)
	if src == nil {
		if m, ok := result.(map[string]interface{}); ok {
			src = m
		}
	}
	if src == nil {
		return ttlMS, snapshotISO
	}
	if snap, ok := src["SnapshotAt"].(string); ok && snap != "" {
		snapshotISO = snap
	}
	switch v := src["TtlSeconds"].(type) {
	case float64:
		if v > 0 {
			ttlMS = int64(v * 1000)
		}
	}
	return ttlMS, snapshotISO
}

func InjectProxyMeta(result interface{}, meta map[string]interface{}) interface{} {
	clone := cloneJSON(result)
	rec, ok := clone.(map[string]interface{})
	if !ok {
		return map[string]interface{}{"value": result, "_proxy": meta}
	}
	content, _ := rec["content"].([]interface{})
	if len(content) > 0 {
		first, _ := content[0].(map[string]interface{})
		text, _ := first["text"].(string)
		var obj map[string]interface{}
		if json.Unmarshal([]byte(text), &obj) == nil {
			prev, _ := obj["_proxy"].(map[string]interface{})
			if prev == nil {
				prev = map[string]interface{}{}
			}
			for k, v := range meta {
				prev[k] = v
			}
			obj["_proxy"] = prev
			b, _ := json.Marshal(obj)
			first["text"] = string(b)
			return rec
		}
	}
	prev, _ := rec["_proxy"].(map[string]interface{})
	if prev == nil {
		prev = map[string]interface{}{}
	}
	for k, v := range meta {
		prev[k] = v
	}
	rec["_proxy"] = prev
	return rec
}

func ContentTextLen(result interface{}) int {
	rec, _ := result.(map[string]interface{})
	content, _ := rec["content"].([]interface{})
	if len(content) == 0 {
		return 0
	}
	first, _ := content[0].(map[string]interface{})
	text, _ := first["text"].(string)
	return len(text)
}

func cloneJSON(v interface{}) interface{} {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out interface{}
	_ = json.Unmarshal(b, &out)
	return out
}

func WrapCached(result interface{}, kind CacheKind, snapshotAt string) interface{} {
	return InjectProxyMeta(result, map[string]interface{}{"cache": string(kind), "snapshotAt": snapshotAt})
}

func WrapDegraded(result interface{}, snapshotAt string) interface{} {
	return InjectProxyMeta(result, map[string]interface{}{
		"degraded":   "unavailable",
		"snapshotAt": snapshotAt,
		"note":       DegradedNote,
	})
}
