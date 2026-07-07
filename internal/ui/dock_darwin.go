//go:build darwin

// Copyright byteyang. All Rights Reserved.

package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

void suppressDockIcon() {
    // Accessory：保留状态栏托盘，不在 Dock / App Switcher 中出现。
    // 必须在 Fyne Run() 内的 applicationDidFinishLaunching: 之后调用，
    // 才能覆盖 Fyne driver 默认设置的 Regular 策略。
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}
*/
import "C"

// SuppressDockIcon 移除 Dock 图标。
// 须通过 fyne.App.Lifecycle().SetOnStarted() 在主事件循环就绪后调用。
func SuppressDockIcon() {
	C.suppressDockIcon()
}
