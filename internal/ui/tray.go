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
	app     fyne.App
	manager *unreal.Manager
	// 懒创建窗口：首次点击时才调用 app.NewWindow()，
	// 避免启动时创建窗口触发 Fyne 将激活策略切回 Regular（导致 Dock 图标）。
	settings  *SettingsWindow
	configWin *configWindow

	// 注入钩子：由 main 提供，用于热重启服务器
	OnToggleServer     func(enabled bool)
	OnRefreshInstances func()

	// 版本信息：由 main 注入当前版本，检查更新后写入结果
	AppVersion  string
	updateState UpdateState

	deskApp desktop.App
	menu    *fyne.Menu
}

// NewTrayController 创建托盘控制器。app 必须实现 desktop.App（Fyne 桌面应用）。
func NewTrayController(app fyne.App, mgr *unreal.Manager) *TrayController {
	tc := &TrayController{
		app:     app,
		manager: mgr,
	}
	if da, ok := app.(desktop.App); ok {
		tc.deskApp = da
	}
	return tc
}

// openSettings 懒创建并显示设置窗口。
func (tc *TrayController) openSettings() {
	if tc.settings == nil {
		tc.settings = NewSettingsWindow(tc.app, tc.manager)
		tc.settings.SetTray(tc)
	}
	tc.settings.Show()
}

// openConfigWindow 懒创建并显示 MCP 客户端配置窗口。
func (tc *TrayController) openConfigWindow() {
	if tc.configWin == nil {
		tc.configWin = newConfigWindow(tc.app)
	}
	port := config.Get().HTTPPort
	tc.configWin.show(port)
}

// SetUpdateState 更新检查结果写入后刷新菜单，供外部（main）在事件循环就绪后调用。
// 可从任意 goroutine 调用。
func (tc *TrayController) SetUpdateState(state UpdateState) {
	if tc.deskApp == nil {
		tc.updateState = state
		return
	}
	fyne.Do(func() {
		tc.updateState = state
		tc.rebuildMenu()
		tc.updateIcon()
	})
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
// 可从任意 goroutine 调用；内部经 fyne.Do 切回 UI 线程（Fyne ≥2.6 要求）。
func (tc *TrayController) Refresh() {
	if tc.deskApp == nil {
		return
	}
	fyne.Do(func() {
		tc.rebuildMenu()
		tc.updateIcon()
	})
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

	// 「MCP 客户端配置」—— 打开配置展示窗口，参考 NexusRider 设置面板
	copyConfig := fyne.NewMenuItem("MCP 客户端配置…", func() {
		tc.openConfigWindow()
	})

	// 「设置…」打开设置窗口（懒创建）
	settingsItem := fyne.NewMenuItem("设置…", func() {
		tc.openSettings()
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

	// 「检查更新」：有新版本时标签变更，点击跳转下载页
	var updateLabel string
	switch {
	case tc.updateState.HasUpdate:
		updateLabel = fmt.Sprintf("[新版本] v%s → 下载", tc.updateState.LatestVersion)
	default:
		updateLabel = "检查更新"
	}
	updateItem := fyne.NewMenuItem(updateLabel, func() {
		openURL(releasesURL)
	})

	quitItem := fyne.NewMenuItem("退出", func() {
		tc.app.Quit()
	})
	quitItem.IsQuit = true // 告知 Fyne 这是 Quit 项，阻止其再自动追加一个

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
		fyne.NewMenuItemSeparator(),
		updateItem,
		quitItem,
	}
	tc.menu = fyne.NewMenu("NexusDesktop", menuItems...)
	tc.deskApp.SetSystemTrayMenu(tc.menu)
}

func (tc *TrayController) updateIcon() {
	if tc.manager.Snapshot().WsOpen {
		tc.deskApp.SetSystemTrayIcon(theme.ComputerIcon()) // 已连接 UE
	} else {
		tc.deskApp.SetSystemTrayIcon(theme.InfoIcon()) // 未连接，待机
	}
}

// openURL 用系统默认浏览器打开 URL。
func openURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
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
