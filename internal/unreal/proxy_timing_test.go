// Copyright byteyang. All Rights Reserved.

package unreal

import "testing"

func TestDecideMaintain(t *testing.T) {
	skip := decideMaintain(true, 1, 3)
	if skip.Action != MaintainSkip || skip.NextCountdown != 3 {
		t.Fatalf("skip: %+v", skip)
	}

	probe := decideMaintain(true, 0, 1)
	if probe.Action != MaintainProbe || probe.NextCountdown != 0 {
		t.Fatalf("probe: %+v", probe)
	}

	full := decideMaintain(true, 0, 0)
	if full.Action != MaintainFullScan || full.NextCountdown != FULL_SCAN_EVERY_N_TICKS {
		t.Fatalf("full: %+v", full)
	}

	closed := decideMaintain(false, 2, 4)
	if closed.Action != MaintainFullScan || closed.NextCountdown != FULL_SCAN_EVERY_N_TICKS {
		t.Fatalf("closed: %+v", closed)
	}
}
