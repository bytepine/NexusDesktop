//go:build !windows && !darwin

// Copyright byteyang. All Rights Reserved.

package ui

// ApplyInPlaceUpdate 非 Windows / macOS 不支持应用内替换。
func ApplyInPlaceUpdate(currentVersion, latestVersion string) error {
	return ErrUnsupportedOS
}
