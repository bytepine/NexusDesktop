// Copyright byteyang. All Rights Reserved.

// Package ui 实现 Fyne 托盘菜单与设置窗口。
package ui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"

	"github.com/bytepine/NexusDesktop/internal/config"
	"github.com/bytepine/NexusDesktop/internal/log"
	"github.com/bytepine/NexusDesktop/internal/unreal"
)

// TrayController 管理系统托盘的菜单与图标状态。
type TrayController struct {
	app      fyne.App
	manager  *unreal.Manager
	settings *SettingsWindow

	// 注入钩子：由 main 提供，用于热重启服务器
	OnToggleServer    func(enabled bool)
	OnRefreshInstances func()

	deskApp desktop.App
	menu    *fyne.Menu
}

// NewTrayController 创建托盘控制器。app 必须实现 desktop.App（Fyne 桌面应用）。
func NewTrayController(app fyne.App, mgr *unreal.Manager, sw *SettingsWindow) *TrayController {
	tc := &TrayController{
		app:      app,
		manager:  mgr,
		settings: sw,
	}
	if da, ok := app.(desktop.App); ok {
		tc.deskApp = da
	}
	return tc
}

// Setup 初始化托盘图标与菜单，需在 Fyne 事件循环启动后调用。
func (tc *TrayController) Setup() {
	if tc.deskApp == nil {
		log.Warn("当前平台不支持系统托盘（非桌面环境）")
		return
	}
	tc.rebuildMenu()
	tc.updateIcon()
}

// Refresh 重建托盘菜单（连接状态/实例列表变化后调用）。
func (tc *TrayController) Refresh() {
	if tc.deskApp == nil {
		return
	}
	tc.rebuildMenu()
	tc.updateIcon()
}

func (tc *TrayController) rebuildMenu() {
	cfg := config.Get()
	snap := tc.manager.Snapshot()
	connPort := snap.ConnectedPort
	wsOpen := snap.WsOpen

	// 顶部状态行
	var statusLabel string
	if wsOpen && connPort > 0 {
		for _, inst := range snap.Instances {
			if inst.Port == connPort {
				statusLabel = fmt.Sprintf("已连接：%s (:%d)", inst.ProjectName, connPort)
				break
			}
		}
		if statusLabel == "" {
			statusLabel = fmt.Sprintf("已连接 :%d", connPort)
		}
	} else {
		statusLabel = "未连接 UE 实例"
	}

	// 实例子菜单
	instances := snap.Instances
	var instanceItems []*fyne.MenuItem
	for _, inst := range instances {
		inst := inst // 捕获
		label := fmt.Sprintf("%s :%d [%s]", inst.ProjectName, inst.Port, inst.NetRole)
		if inst.Port == connPort && wsOpen {
			label = "✓ " + label
		}
		item := fyne.NewMenuItem(label, func() {
			tc.manager.ConnectTo(inst.Port, true)
			tc.Refresh()
		})
		instanceItems = append(instanceItems, item)
	}
	if len(instanceItems) == 0 {
		instanceItems = []*fyne.MenuItem{
			fyne.NewMenuItem("（未发现实例）", nil),
		}
	}
	instancesMenu := fyne.NewMenuItem("选择 UE 实例", nil)
	instancesMenu.ChildMenu = fyne.NewMenu("", instanceItems...)

	// 「启用中转服务器」开关
	serverToggleLabel := "启用中转服务器"
	if cfg.Enabled {
		serverToggleLabel = "✓ " + serverToggleLabel
	}
	serverToggle := fyne.NewMenuItem(serverToggleLabel, func() {
		newEnabled := !cfg.Enabled
		newCfg := cfg
		newCfg.Enabled = newEnabled
		if err := config.Save(newCfg); err != nil {
			log.Errorf("保存配置失败: %v", err)
		}
		if tc.OnToggleServer != nil {
			tc.OnToggleServer(newEnabled)
		}
		tc.Refresh()
	})

	// 「复制 MCP 客户端配置」
	copyConfig := fyne.NewMenuItem("复制 MCP 客户端配置", func() {
		port := tc.manager.ConnectedPort
		if port <= 0 {
			port = cfg.HTTPPort
		}
		json := fmt.Sprintf(`{"mcpServers":{"nexus-unreal":{"url":"http://127.0.0.1:%d/stream"}}}`, cfg.HTTPPort)
		tc.app.Clipboard().SetContent(json)
	})

	// 「设置…」打开设置窗口
	settingsItem := fyne.NewMenuItem("设置…", func() {
		tc.settings.Show()
	})

	// 「打开日志目录」
	openLogs := fyne.NewMenuItem("打开日志目录", func() {
		openDirectory(log.LogDir())
	})

	// 「开机自启」开关
	autostartLabel := "开机自启"
	if IsAutostartEnabled() {
		autostartLabel = "✓ " + autostartLabel
	}
	autostartItem := fyne.NewMenuItem(autostartLabel, func() {
		enable := !IsAutostartEnabled()
		if err := SetAutostart(enable); err != nil {
			log.Errorf("设置开机自启失败: %v", err)
		}
		tc.Refresh()
	})

	// 「断开连接」
	disconnectItem := fyne.NewMenuItem("断开 UE 连接", func() {
		tc.manager.Disconnect()
		tc.Refresh()
	})
	disconnectItem.Disabled = !wsOpen

	// 「扫描实例」主动触发一次发现
	scanItem := fyne.NewMenuItem("扫描 UE 实例", func() {
		if tc.OnRefreshInstances != nil {
			tc.OnRefreshInstances()
		}
	})

	menuItems := []*fyne.MenuItem{
		fyne.NewMenuItem(statusLabel, nil),
		fyne.NewMenuItemSeparator(),
		serverToggle,
		instancesMenu,
		scanItem,
		disconnectItem,
		fyne.NewMenuItemSeparator(),
		copyConfig,
		settingsItem,
		openLogs,
		autostartItem,
	}
	tc.menu = fyne.NewMenu("NexusDesktop", menuItems...)
	tc.deskApp.SetSystemTrayMenu(tc.menu)
}

func (tc *TrayController) updateIcon() {
	if tc.manager.Snapshot().WsOpen {
		tc.deskApp.SetSystemTrayIcon(theme.ComputerIcon())
	} else {
		tc.deskApp.SetSystemTrayIcon(theme.InfoIcon())
	}
}

// openDirectory 用系统默认文件管理器打开目录。
func openDirectory(dir string) {
	if dir == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	_ = cmd.Start()
}
