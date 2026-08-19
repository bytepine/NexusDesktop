// Copyright byteyang. All Rights Reserved.

// Package proxy 实现代理会话层（TTL 缓存 / degraded / 写门控），与 docs/proxy-session.md 对齐。
package proxy

import (
	"encoding/json"
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

var destructiveCaps = map[string]struct{}{"delete_asset": {}, "rename_asset": {}}
var stopPie = map[string]struct{}{"stop": {}, "end": {}, "quit": {}, "endplay": {}, "end_play": {}, "end-play": {}}

type CallInfo struct {
	ToolName    string
	Capability  string
	InnerArgs   map[string]interface{}
	Identity    string
	Sections    []string
	ExactKey    string
	IdentityKey string
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
	if cap == "" || cap == "submit_feedback" || cap == "search_capabilities" {
		return false
	}
	if strings.HasPrefix(cap, "get_") || strings.HasPrefix(cap, "list_") || strings.HasPrefix(cap, "search_") {
		return false
	}
	return true
}

func IsDestructive(cap string, inner map[string]interface{}) bool {
	if _, ok := destructiveCaps[cap]; ok {
		return true
	}
	if cap == "control_pie" {
		action, _ := inner["action"].(string)
		_, hit := stopPie[strings.ToLower(action)]
		return hit
	}
	if strings.HasPrefix(cap, "manage_") {
		ops, _ := inner["operations"].([]interface{})
		for _, op := range ops {
			m, _ := op.(map[string]interface{})
			action, _ := m["action"].(string)
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
	return cap == "get_output_log" || cap == "capture_viewport" ||
		strings.HasPrefix(cap, "list_runtime_") || strings.HasPrefix(cap, "get_runtime_")
}

func IsDurableRead(cap string) bool {
	if IsWriteCapability(cap) || IsVolatileCap(cap) {
		return false
	}
	return strings.HasPrefix(cap, "search_") || strings.HasPrefix(cap, "get_asset_") ||
		strings.HasPrefix(cap, "get_editor_") || cap == "get_gameplay_tags" ||
		cap == "get_asset_refs" || cap == "get_asset_lua_binding"
}

func DefaultTtlMS(cap string) int64 {
	if IsVolatileCap(cap) {
		return DefaultVolatileTTLMS
	}
	if strings.HasPrefix(cap, "search_") {
		return DefaultSearchTTLMS
	}
	return DefaultReadTTLMS
}

func extractIdentity(args map[string]interface{}) string {
	for _, key := range []string{"assetPath", "actorName", "widgetName", "luaPath", "scriptPath"} {
		if v, ok := args[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func extractSections(args map[string]interface{}) []string {
	raw, ok := args["sections"].([]interface{})
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
		if s == "all" {
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

func ParseCall(toolName string, args map[string]interface{}) CallInfo {
	if args == nil {
		args = map[string]interface{}{}
	}
	capability := toolName
	inner := args
	isBatch := false
	if toolName == "call_capability" {
		if _, ok := args["calls"]; ok {
			isBatch = true
			capability = "call_capability.calls"
		} else {
			capability, _ = args["capability"].(string)
			if m, ok := args["arguments"].(map[string]interface{}); ok {
				inner = m
			} else {
				inner = map[string]interface{}{}
			}
		}
	}
	ident := extractIdentity(inner)
	sections := extractSections(inner)
	raw, _ := json.Marshal(inner)
	exact := toolName + "|" + capability + "|" + string(raw)
	return CallInfo{
		ToolName: toolName, Capability: capability, InnerArgs: inner,
		Identity: ident, Sections: sections, ExactKey: exact,
		IdentityKey: capability + "|" + ident,
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
	if snap, ok := src["_snapshotAt"].(string); ok && snap != "" {
		snapshotISO = snap
	}
	switch v := src["_ttl_seconds"].(type) {
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
