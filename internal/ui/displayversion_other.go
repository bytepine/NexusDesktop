//go:build !windows

// Copyright byteyang. All Rights Reserved.

package ui

// RepairDisplayVersion Windows 以外无卸载注册表，空操作。
func RepairDisplayVersion(version string) {}
