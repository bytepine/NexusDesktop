// Copyright byteyang. All Rights Reserved.

package ui

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/bytepine/NexusDesktop/internal/config"
	"github.com/bytepine/NexusDesktop/internal/log"
	"github.com/bytepine/NexusDesktop/internal/unreal"
)

// SettingsWindow 封装 Fyne 设置窗口。
// 关闭时仅隐藏（Hide）回托盘，不退出程序。
type SettingsWindow struct {
	win     fyne.Window
	manager *unreal.Manager
	tray    *TrayController // 保存后回调 Refresh
}

// NewSettingsWindow 创建设置窗口（初始隐藏）。
func NewSettingsWindow(app fyne.App, mgr *unreal.Manager) *SettingsWindow {
	sw := &SettingsWindow{manager: mgr}
	w := app.NewWindow("NexusDesktop 设置")
	w.Resize(fyne.NewSize(480, 460))
	w.SetFixedSize(true)
	// 关闭按钮仅隐藏，不退出
	w.SetCloseIntercept(func() {
		w.Hide()
	})
	sw.win = w
	sw.buildContent()
	return sw
}

// SetTray 注入 TrayController 引用，保存后在 Refresh 时刷新托盘。
func (sw *SettingsWindow) SetTray(tc *TrayController) {
	sw.tray = tc
}

// Show 显示设置窗口（若已显示则置前）。
func (sw *SettingsWindow) Show() {
	sw.buildContent() // 刷新内容再显示
	sw.win.Show()
	sw.win.RequestFocus()
}

func (sw *SettingsWindow) buildContent() {
	cfg := config.Get()

	// ---- 服务器开关 ----
	enabledCheck := widget.NewCheck("启用中转服务器", nil)
	enabledCheck.SetChecked(cfg.Enabled)

	// ---- 端口设置 ----
	httpPortEntry := widget.NewEntry()
	httpPortEntry.SetText(fmt.Sprintf("%d", cfg.HTTPPort))
	httpPortEntry.SetPlaceHolder("默认 6900")

	scanStartEntry := widget.NewEntry()
	scanStartEntry.SetText(fmt.Sprintf("%d", cfg.ScanPortStart))
	scanStartEntry.SetPlaceHolder("默认 45000")

	scanEndEntry := widget.NewEntry()
	scanEndEntry.SetText(fmt.Sprintf("%d", cfg.ScanPortEnd))
	scanEndEntry.SetPlaceHolder("默认 45100")

	scanIntervalEntry := widget.NewEntry()
	scanIntervalEntry.SetText(fmt.Sprintf("%d", cfg.ScanIntervalSeconds))
	scanIntervalEntry.SetPlaceHolder("默认 5")

	// ---- 已发现实例列表 ----
	instances := sw.manager.Instances
	connPort := sw.manager.ConnectedPort
	wsOpen := sw.manager.IsWsOpen()

	instanceRows := []fyne.CanvasObject{
		widget.NewLabelWithStyle("已发现 UE 实例", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	}
	if len(instances) == 0 {
		instanceRows = append(instanceRows, widget.NewLabel("（未发现实例）"))
	}
	for _, inst := range instances {
		inst := inst
		label := fmt.Sprintf("%s  :%d  [%s]", inst.ProjectName, inst.Port, inst.NetRole)
		var btn *widget.Button
		if inst.Port == connPort && wsOpen {
			btn = widget.NewButton("✓ "+label, nil)
			btn.Importance = widget.HighImportance
		} else {
			btn = widget.NewButton(label, func() {
				sw.manager.ConnectTo(inst.Port, true)
				if sw.tray != nil {
					sw.tray.Refresh()
				}
				sw.buildContent() // 刷新界面
				sw.win.Content().Refresh()
			})
		}
		instanceRows = append(instanceRows, btn)
	}
	refreshBtn := widget.NewButton("刷新实例列表", func() {
		sw.manager.DiscoverInstances()
		sw.buildContent()
		sw.win.Content().Refresh()
		if sw.tray != nil {
			sw.tray.Refresh()
		}
	})
	instanceRows = append(instanceRows, refreshBtn)

	// ---- 保存/取消按钮 ----
	statusLabel := widget.NewLabel("")

	saveBtn := widget.NewButton("保存", func() {
		httpPort, _ := strconv.Atoi(httpPortEntry.Text)
		scanStart, _ := strconv.Atoi(scanStartEntry.Text)
		scanEnd, _ := strconv.Atoi(scanEndEntry.Text)
		interval, _ := strconv.Atoi(scanIntervalEntry.Text)

		newCfg := config.Config{
			Enabled:             enabledCheck.Checked,
			HTTPPort:            httpPort,
			ScanPortStart:       scanStart,
			ScanPortEnd:         scanEnd,
			ScanIntervalSeconds: interval,
		}
		if err := config.Save(newCfg); err != nil {
			statusLabel.SetText("保存失败: " + err.Error())
			log.Errorf("设置保存失败: %v", err)
			return
		}
		statusLabel.SetText("已保存")
		if sw.tray != nil {
			sw.tray.Refresh()
		}
	})
	saveBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton("关闭", func() {
		sw.win.Hide()
	})

	// ---- 布局 ----
	form := container.NewVBox(
		widget.NewLabelWithStyle("服务器配置", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		enabledCheck,
		container.NewGridWithColumns(2,
			widget.NewLabel("MCP HTTP 端口"),
			httpPortEntry,
			widget.NewLabel("UE 扫描起始端口"),
			scanStartEntry,
			widget.NewLabel("UE 扫描结束端口"),
			scanEndEntry,
			widget.NewLabel("扫描间隔（秒）"),
			scanIntervalEntry,
		),
		widget.NewSeparator(),
	)
	for _, row := range instanceRows {
		form.Add(row)
	}
	form.Add(widget.NewSeparator())
	form.Add(container.NewHBox(saveBtn, cancelBtn))
	form.Add(statusLabel)

	sw.win.SetContent(container.NewScroll(form))
}
