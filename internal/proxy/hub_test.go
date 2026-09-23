// Copyright byteyang. All Rights Reserved.

package proxy

import "testing"

func TestConfirmIfNeededDoesNotRememberAlways(t *testing.T) {
	h := NewHub()
	calls := 0
	h.SetPrompter(func(CallInfo) GateDecision {
		calls++
		return DecisionAlways
	})
	info := CallInfo{Capability: "delete_asset"}
	if h.ConfirmIfNeeded(info) != DecisionAlways {
		t.Fatal("first decision")
	}
	if h.ConfirmIfNeeded(info) != DecisionAlways {
		t.Fatal("second decision")
	}
	if calls != 2 {
		t.Fatalf("prompts=%d, hub stored always-allow", calls)
	}
}
