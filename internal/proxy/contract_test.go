// Copyright byteyang. All Rights Reserved.

package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "nexus-proxy-test-")
	if err != nil {
		panic(err)
	}
	offloadRoot = filepath.Join(dir, "nexus-mcp-offload-v2")
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestLegacyV2PolicyCompatibility(t *testing.T) {
	args := map[string]interface{}{"capability": "get_asset_blueprint", "arguments": map[string]interface{}{"assetPath": "/synthetic/BP", "sections": []interface{}{"all"}}, "targetPort": float64(45000)}
	before := mustJSON(args)
	read := ParseCall("call_capability", args)
	if read.IsWrite || read.Identity != "/synthetic/BP" || NeedsGate(GateAll, read.Capability, read.InnerArgs) {
		t.Fatal("legacy read classification failed")
	}
	if IsWriteCapability("get_asset_blueprint") || DefaultTtlMS("get_output_log") != 8000 || DefaultTtlMS("search_asset") != 60000 || !IsDurableRead("get_asset_blueprint") || IsDurableRead("list_runtime_actors") {
		t.Fatal("legacy policy classification failed")
	}
	if !NeedsGate(GateDestructive, "delete_asset", nil) || !NeedsGate(GateDestructive, "rename_asset", nil) ||
		!NeedsGate(GateDestructive, "control_pie", map[string]interface{}{"action": "stop", "Action": "start"}) ||
		!NeedsGate(GateDestructive, "manage_asset_blueprint", map[string]interface{}{"operations": []interface{}{map[string]interface{}{"action": "remove_node"}}}) {
		t.Fatal("legacy dangerous writes must prompt")
	}
	conflicting := ParseCall("call_capability", map[string]interface{}{"capability": "delete_asset", "Capability": "GetAssetBlueprint"})
	if !NeedsGate(GateDestructive, conflicting.Capability, conflicting.InnerArgs) {
		t.Fatal("legacy wire fields must take precedence")
	}
	payload := map[string]interface{}{"content": []interface{}{map[string]interface{}{"type": "text", "text": `{"_ttl_seconds":1,"_snapshotAt":"2026-10-22T00:00:00Z"}`}}}
	h := NewHub()
	h.Store(read, payload, 1000)
	if ttl, snap := ExtractTtlMS(payload, read.Capability, 1000); ttl != 1000 || snap != "2026-10-22T00:00:00Z" {
		t.Fatal("legacy TTL metadata lost")
	}
	subset := ParseCall("call_capability", map[string]interface{}{"capability": "get_asset_blueprint", "arguments": map[string]interface{}{"assetPath": "/synthetic/BP", "sections": []interface{}{"defaults"}}, "targetPort": float64(45000)})
	if hit := h.LookupFresh(subset, 1100); hit == nil || hit.Kind != CacheSectionHit {
		t.Fatal("legacy all sections must cover subsets")
	}
	if h.LookupFresh(read, 32000) != nil || h.LookupDegraded(read, 100000) == nil {
		t.Fatal("legacy TTL or degraded snapshot failed")
	}
	modern := ParseCall("CallCapability", map[string]interface{}{"Capability": "GetAssetBlueprint", "Arguments": map[string]interface{}{"AssetPath": "/synthetic/BP"}, "targetPort": float64(45000)})
	if h.LookupFresh(modern, 1100) != nil {
		t.Fatal("different wire formats must not share cached response shapes")
	}
	batch := ParseCall("call_capability", map[string]interface{}{"calls": []interface{}{map[string]interface{}{"capability": "manage_asset_blueprint", "arguments": map[string]interface{}{"assetPath": "/synthetic/BP"}}}, "targetPort": float64(45000)})
	h.Store(batch, map[string]interface{}{}, 1200)
	if !batch.IsBatch || h.LookupFresh(read, 1300) != nil || mustJSON(args) != before {
		t.Fatal("legacy batch invalidation or immutable arguments failed")
	}
}

func TestCanonicalContractAndInstanceCache(t *testing.T) {
	query := func(port float64, host string, sections ...interface{}) map[string]interface{} {
		return map[string]interface{}{
			"Capability": "GetAssetBlueprint", "Arguments": map[string]interface{}{"AssetPath": "/synthetic/BP", "Sections": sections},
			"targetPort": port, "targetHost": host,
		}
	}
	a := ParseCall("CallCapability", query(45000, "", "All"))
	if a.IsWrite || a.Identity != "/synthetic/BP" {
		t.Fatalf("canonical read misclassified: %+v", a)
	}
	b := ParseCall("CallCapability", query(45001, "", "All"))
	remote := ParseCall("CallCapability", query(45000, "192.0.2.1", "All"))
	localAlias := ParseCall("CallCapability", query(45000, "localhost", "All"))
	if a.ExactKey != localAlias.ExactKey {
		t.Fatal("loopback aliases must share a cache scope")
	}
	h := NewHub()
	payload := map[string]interface{}{"content": []interface{}{map[string]interface{}{"type": "text", "text": `{"TtlSeconds":1,"SnapshotAt":"2026-10-22T00:00:00Z"}`}}}
	h.Store(a, payload, 1000)
	if h.LookupFresh(b, 1100) != nil || h.LookupFresh(remote, 1100) != nil || h.LookupDegraded(b, 100000) != nil {
		t.Fatal("cache crossed UE instances")
	}
	if h.LookupFresh(ParseCall("CallCapability", query(45001, "", "Defaults")), 1100) != nil {
		t.Fatal("section cache crossed instances")
	}
	if ttl, snapshot := ExtractTtlMS(payload, "GetAssetBlueprint", 1000); ttl != 1000 || snapshot != "2026-10-22T00:00:00Z" {
		t.Fatal("canonical TTL metadata not parsed")
	}
	if h.LookupFresh(a, 31001) != nil || h.LookupDegraded(a, 100000) == nil {
		t.Fatal("canonical TTL or same-instance degraded snapshot failed")
	}
	if !NeedsGate(GateDestructive, "DeleteAsset", nil) || !NeedsGate(GateDestructive, "ControlPie", map[string]interface{}{"Action": "Stop"}) ||
		!NeedsGate(GateDestructive, "ManageAssetBlueprint", map[string]interface{}{"Operations": []interface{}{map[string]interface{}{"Action": "RemoveNode"}}}) {
		t.Fatal("canonical destructive calls bypassed the gate")
	}
	if DefaultTtlMS("GetOutputLog") != 8000 || IsDurableRead("GetRuntimeActorProperty") {
		t.Fatal("volatile reads misclassified")
	}
	defaultA := ParseCall("CallCapability", map[string]interface{}{"Capability": "GetAssetBlueprint", "Arguments": a.InnerArgs}, "127.0.0.1:45000")
	if defaultA.ExactKey != a.ExactKey {
		t.Fatal("default and explicit routes must share the same instance scope")
	}
	h.Store(b, payload, 1000)
	write := ParseCall("ManageAssetBlueprint", map[string]interface{}{"AssetPath": "/synthetic/BP", "targetPort": float64(45000)})
	h.Store(write, map[string]interface{}{}, 1200)
	if h.LookupFresh(a, 1300) != nil || h.LookupFresh(b, 1300) == nil {
		t.Fatal("writes must invalidate only the same instance")
	}
	for _, port := range []interface{}{45000.5, float64(65536), "45000"} {
		if ExplicitTargetPort(map[string]interface{}{"targetPort": port}) > 0 {
			t.Fatal("invalid port accepted", port)
		}
	}
	if !ParseCall("CallCapability", map[string]interface{}{"Calls": []interface{}{}}).IsBatch {
		t.Fatal("canonical batch not recognized")
	}
}

func TestBatchInvalidationAndFailedReads(t *testing.T) {
	h := NewHub()
	a := ParseCall("GetAssetBlueprint", map[string]interface{}{"AssetPath": "/synthetic/BP", "targetPort": float64(45000)})
	b := ParseCall("GetAssetBlueprint", map[string]interface{}{"AssetPath": "/synthetic/BP", "targetPort": float64(45001)})
	success := map[string]interface{}{"content": []interface{}{map[string]interface{}{"type": "text", "text": `{"value":"old"}`}}}
	failure := map[string]interface{}{"isError": true, "content": []interface{}{map[string]interface{}{"type": "text", "text": "temporary failure"}}}
	for _, result := range []interface{}{success, failure} {
		h.Store(a, success, 1000)
		h.Store(b, success, 1000)
		batch := ParseCall("CallCapability", map[string]interface{}{
			"Calls":      []interface{}{map[string]interface{}{"Capability": "ManageAssetBlueprint", "Arguments": map[string]interface{}{"AssetPath": "/synthetic/BP"}}},
			"targetPort": float64(45000),
		})
		h.Store(batch, result, 1100)
		if h.LookupFresh(a, 1200) != nil || h.LookupDegraded(a, 100000) != nil || h.LookupFresh(b, 1200) == nil {
			t.Fatal("batch must invalidate only its target, including partial failure")
		}
	}
	h.Store(a, failure, 1300)
	if h.LookupFresh(a, 1400) != nil || h.LookupDegraded(a, 100000) != nil {
		t.Fatal("failed read must not become a fresh or degraded snapshot")
	}
}
