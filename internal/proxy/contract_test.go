// Copyright byteyang. All Rights Reserved.

package proxy

import "testing"

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
