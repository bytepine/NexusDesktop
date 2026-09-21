// Copyright byteyang. All Rights Reserved.

package unreal

import "testing"

func TestPickEditorInstancePrefersHostKind(t *testing.T) {
	game := InstanceInfo{Port: 45000, HostKind: "Game", NetRole: "Standalone"}
	editor := InstanceInfo{Port: 45001, HostKind: "Editor", NetRole: "Standalone"}
	got := pickEditorInstance([]InstanceInfo{game, editor})
	if got == nil || got.Port != 45001 {
		t.Fatalf("want editor port 45001, got %+v", got)
	}
}

func TestPickEditorInstanceLegacyNetRole(t *testing.T) {
	game := InstanceInfo{Port: 45000, NetRole: "Standalone"}
	editor := InstanceInfo{Port: 45001, NetRole: "Editor"}
	got := pickEditorInstance([]InstanceInfo{game, editor})
	if got == nil || got.Port != 45001 {
		t.Fatalf("want editor port 45001, got %+v", got)
	}
}
