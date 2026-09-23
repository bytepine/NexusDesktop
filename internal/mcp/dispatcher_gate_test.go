// Copyright byteyang. All Rights Reserved.

package mcp

import (
	"testing"

	"github.com/bytepine/NexusDesktop/internal/proxy"
	"github.com/bytepine/NexusDesktop/internal/unreal"
)

func TestAlwaysAllowStaysInsideMCPSession(t *testing.T) {
	mgr := unreal.NewManager()
	prompts := 0
	mgr.Hub.SetPrompter(func(proxy.CallInfo) proxy.GateDecision {
		prompts++
		return proxy.DecisionAlways
	})
	info := proxy.ParseCall("delete_asset", map[string]interface{}{"assetPath": "/Game/A"})

	first := NewDispatcher(mgr, nil, "test")
	if first.confirmWrite(info) != proxy.DecisionAllow {
		t.Fatal("first allow")
	}
	if first.confirmWrite(info) != proxy.DecisionAllow {
		t.Fatal("repeat allow")
	}
	if prompts != 1 {
		t.Fatalf("same session prompts=%d", prompts)
	}

	second := NewDispatcher(mgr, nil, "test")
	if second.confirmWrite(info) != proxy.DecisionAllow {
		t.Fatal("other session")
	}
	if prompts != 2 {
		t.Fatalf("other session prompts=%d", prompts)
	}
}
