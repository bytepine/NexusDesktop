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
