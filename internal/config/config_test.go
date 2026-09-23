// Copyright byteyang. All Rights Reserved.

package config

import "testing"

func TestSanitizeUpdateChannel(t *testing.T) {
	c := DefaultConfig()
	if c.UpdateChannel != UpdateChannelAuto {
		t.Fatalf("default = %q", c.UpdateChannel)
	}
	c.UpdateChannel = ""
	sanitize(&c)
	if c.UpdateChannel != UpdateChannelAuto {
		t.Fatalf("empty → %q", c.UpdateChannel)
	}
	c.UpdateChannel = "nope"
	sanitize(&c)
	if c.UpdateChannel != UpdateChannelAuto {
		t.Fatalf("illegal → %q", c.UpdateChannel)
	}
	c.UpdateChannel = UpdateChannelPre
	sanitize(&c)
	if c.UpdateChannel != UpdateChannelPre {
		t.Fatalf("pre was rewritten to %q", c.UpdateChannel)
	}
}

func TestIsPrerelease(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"2.0.0", false},
		{"2.0.0-beta.3", true},
		{"v2.0.0-beta.3", true},
		{"2.0.0+build", false},
		{"dev", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsPrerelease(c.v); got != c.want {
			t.Errorf("IsPrerelease(%q) = %v, want %v", c.v, got, c.want)
		}
	}
}

func TestResolveUpdateChannel(t *testing.T) {
	cases := []struct {
		stored, current, want string
	}{
		{"", "2.0.0", UpdateChannelStable},
		{"auto", "2.0.0", UpdateChannelStable},
		{"auto", "2.0.0-beta.3", UpdateChannelPre},
		{"", "2.0.0-beta.3", UpdateChannelPre},
		{"stable", "2.0.0-beta.3", UpdateChannelStable},
		{"pre", "2.0.0", UpdateChannelPre},
		{"stable", "2.0.0", UpdateChannelStable},
	}
	for _, c := range cases {
		if got := ResolveUpdateChannel(c.stored, c.current); got != c.want {
			t.Errorf("ResolveUpdateChannel(%q, %q) = %q, want %q", c.stored, c.current, got, c.want)
		}
	}
}

func TestClampScanPorts(t *testing.T) {
	s, e := ClampScanPorts(45000, 45100)
	if s != 45000 || e != 45100 {
		t.Fatalf("default range %d-%d", s, e)
	}
	s, e = ClampScanPorts(45100, 45000)
	if s != 45000 || e != 45100 {
		t.Fatalf("swapped %d-%d", s, e)
	}
	s, e = ClampScanPorts(45000, 46000)
	if s != 45000 || e != 45000+MaxScanPortSpan-1 {
		t.Fatalf("wide %d-%d", s, e)
	}
	s, e = ClampScanPorts(0, 0)
	if s != 45000 || e != 45100 {
		t.Fatalf("invalid %d-%d", s, e)
	}
}

func TestParsePortAndInterval(t *testing.T) {
	if _, err := ParsePortField("abc"); err != ErrInvalidPort {
		t.Fatalf("abc: %v", err)
	}
	if _, err := ParsePortField("80"); err != ErrInvalidPort {
		t.Fatalf("80: %v", err)
	}
	n, err := ParsePortField("6700")
	if err != nil || n != 6700 {
		t.Fatalf("6700: %d %v", n, err)
	}
	if _, err := ParseScanIntervalField("0"); err != ErrInvalidInterval {
		t.Fatalf("0: %v", err)
	}
	if n, err := ParseScanIntervalField("5"); err != nil || n != 5 {
		t.Fatalf("5: %d %v", n, err)
	}
	if err := ValidateScanRange(45000, 45199); err != nil {
		t.Fatalf("200 ports: %v", err)
	}
	if err := ValidateScanRange(45000, 45200); err != ErrScanSpanTooWide {
		t.Fatalf("201 ports: %v", err)
	}
}

func TestSanitizeClampsScanSpan(t *testing.T) {
	c := DefaultConfig()
	c.ScanPortStart = 45000
	c.ScanPortEnd = 47000
	sanitize(&c)
	if c.ScanPortEnd != 45000+MaxScanPortSpan-1 {
		t.Fatalf("end=%d", c.ScanPortEnd)
	}
}
