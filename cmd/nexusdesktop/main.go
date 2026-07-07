// Copyright byteyang. All Rights Reserved.

// NexusDesktop — 独立 MCP 中转程序（系统托盘常驻 + 设置窗口）。
//
// 启动后在系统托盘显示 UE 连接状态；AI 客户端经 MCP HTTP 服务器连接，
// 服务器自动发现本地 UE 实例并经 WebSocket JSON-RPC 转发工具调用。
//
// 端点：http://127.0.0.1:6700/stream（端口可在设置窗口调整）
package main

import (
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"github.com/bytepine/NexusDesktop/assets"
	"github.com/bytepine/NexusDesktop/internal/config"
	nlog "github.com/bytepine/NexusDesktop/internal/log"
	"github.com/bytepine/NexusDesktop/internal/mcp"
	"github.com/bytepine/NexusDesktop/internal/ui"
	"github.com/bytepine/NexusDesktop/internal/unreal"
)

// appVersion 由构建脚本通过 -X main.appVersion=<ver> 注入；默认 dev。
var appVersion = "dev"

func main() {
	// 单实例锁：第二个实例直接退出
	if !ui.AcquireLock() {
		return
	}
	defer ui.ReleaseLock()

	// 初始化配置与日志
	cfg, _ := config.Load()
	nlog.Init(config.AppDir())
	nlog.Infof("NexusDesktop v%s 启动", appVersion)

	// 创建 UE 实例管理器
	mgr := unreal.NewManager()
	mgr.ScanPortStart = cfg.ScanPortStart
	mgr.ScanPortEnd = cfg.ScanPortEnd

	// Fyne 应用（主 UI 入口）
	fyneApp := app.NewWithID("com.bytepine.nexusdesktop")
	appIcon := fyne.NewStaticResource("icon.png", assets.IconPNG)
	fyneApp.SetIcon(appIcon)

	// 创建设置窗口（初始隐藏）
	settingsWin := ui.NewSettingsWindow(fyneApp, mgr)

	// 创建托盘控制器
	tray := ui.NewTrayController(fyneApp, mgr, settingsWin)
	settingsWin.SetTray(tray)

	// 双击托盘图标打开设置窗口
	// （Fyne 托盘双击事件通过菜单设置项替代，此处为占位说明）

	// ----- 服务器生命周期管理 -----
	var (
		mu        sync.Mutex
		server    *mcp.Server
		scanTimer *time.Ticker
		scanStop  chan struct{}
	)

	startServer := func() {
		mu.Lock()
		defer mu.Unlock()
		c := config.Get()
		if !c.Enabled || server != nil {
			return
		}
		srv := mcp.NewServer(mgr, appVersion)
		port, err := srv.Start(c.HTTPPort)
		if err != nil {
			nlog.Errorf("MCP 服务器启动失败: %v", err)
			return
		}
		if port != c.HTTPPort {
			nlog.Infof("端口 %d 被占用，实际使用 %d", c.HTTPPort, port)
		}
		server = srv

		// UE 连接变化 → 刷新托盘 + 推送 tools/list_changed
		mgr.OnConnectionChanged = func(connPort int) {
			if connPort > 0 {
				nlog.Infof("已连接 UE 实例（端口 %d）", connPort)
				go func() {
					_, _ = mgr.FetchToolsList()
					srv.SendToolsChangedNotification()
				}()
			} else {
				nlog.Info("UE 实例已断开")
			}
			tray.Refresh()
		}
		mgr.OnToolsChanged = func() {
			srv.SendToolsChangedNotification()
		}
		// 实例列表变化（新增/消失）→ 刷新托盘，但不影响已打开的菜单正常交互
		mgr.OnInstancesChanged = func() {
			tray.Refresh()
		}
	}

	stopServer := func() {
		mu.Lock()
		defer mu.Unlock()
		if server == nil {
			return
		}
		server.Stop()
		server = nil
		mgr.OnConnectionChanged = nil
		mgr.OnToolsChanged = nil
		mgr.OnInstancesChanged = nil
	}

	startScanTimer := func() {
		mu.Lock()
		defer mu.Unlock()
		c := config.Get()
		if scanTimer != nil {
			scanTimer.Stop()
			if scanStop != nil {
				close(scanStop)
			}
		}
		scanTimer = time.NewTicker(time.Duration(c.ScanIntervalSeconds) * time.Second)
		scanStop = make(chan struct{})
		stop := scanStop
		tick := scanTimer

		go func() {
			for {
				select {
				case <-stop:
					return
				case <-tick.C:
					mu.Lock()
					srvRunning := server != nil
					mu.Unlock()
					if srvRunning {
						mgr.MaintainConnection()
					}
				}
			}
		}()
	}

	stopScanTimer := func() {
		mu.Lock()
		defer mu.Unlock()
		if scanTimer != nil {
			scanTimer.Stop()
			scanTimer = nil
		}
		if scanStop != nil {
			close(scanStop)
			scanStop = nil
		}
	}

	// 注入托盘回调
	tray.OnToggleServer = func(enabled bool) {
		if enabled {
			startServer()
			startScanTimer()
		} else {
			stopServer()
			stopScanTimer()
			mgr.Disconnect()
		}
		tray.Refresh()
	}
	tray.OnRefreshInstances = func() {
		go func() {
			mgr.DiscoverInstances()
			tray.Refresh()
		}()
	}

	// 配置变更热更新（扫描参数）
	config.OnChange(func(c config.Config) {
		mgr.ScanPortStart = c.ScanPortStart
		mgr.ScanPortEnd = c.ScanPortEnd
		if c.Enabled {
			startScanTimer()
		}
	})

	// 初始启动
	if cfg.Enabled {
		startServer()
		startScanTimer()
	}

	// 初始化托盘（必须在立即扫描之前，确保托盘已就绪再接受刷新回调）
	tray.Setup()

	// 托盘就绪后立即触发一次扫描，结果通过 OnInstancesChanged 回调刷新菜单
	if cfg.Enabled {
		go mgr.MaintainConnection()
	}

	nlog.Info("进入 Fyne 主事件循环")
	fyneApp.Run() // 阻塞直到退出

	// 退出清理
	nlog.Info("NexusDesktop 退出")
	stopScanTimer()
	stopServer()
	mgr.Dispose()
}
