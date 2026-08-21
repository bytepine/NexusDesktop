// Copyright byteyang. All Rights Reserved.

package ui

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/bytepine/NexusDesktop/internal/config"
	"github.com/bytepine/NexusDesktop/internal/i18n"
	"github.com/bytepine/NexusDesktop/internal/log"
	"github.com/bytepine/NexusDesktop/internal/proxy"
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
	w := app.NewWindow(i18n.T("settings.title"))
	w.Resize(fyne.NewSize(480, 520))
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
	sw.win.SetTitle(i18n.T("settings.title"))

	// ---- 服务器开关 ----
	enabledCheck := widget.NewCheck(i18n.T("settings.enable_proxy"), nil)
	enabledCheck.SetChecked(cfg.Enabled)

	// ---- 端口设置 ----
	httpPortEntry := widget.NewEntry()
	httpPortEntry.SetText(fmt.Sprintf("%d", cfg.HTTPPort))
	httpPortEntry.SetPlaceHolder(i18n.T("settings.placeholder", 6700))

	scanStartEntry := widget.NewEntry()
	scanStartEntry.SetText(fmt.Sprintf("%d", cfg.ScanPortStart))
	scanStartEntry.SetPlaceHolder(i18n.T("settings.placeholder", 45000))

	scanEndEntry := widget.NewEntry()
	scanEndEntry.SetText(fmt.Sprintf("%d", cfg.ScanPortEnd))
	scanEndEntry.SetPlaceHolder(i18n.T("settings.placeholder", 45100))

	scanIntervalEntry := widget.NewEntry()
	scanIntervalEntry.SetText(fmt.Sprintf("%d", cfg.ScanIntervalSeconds))
	scanIntervalEntry.SetPlaceHolder(i18n.T("settings.placeholder", 5))

	gateOpts := []string{i18n.T("settings.gate_off"), i18n.T("settings.gate_destructive"), i18n.T("settings.gate_all")}
	gateSelect := widget.NewSelect(gateOpts, nil)
	switch cfg.WriteGate {
	case "off":
		gateSelect.SetSelected(gateOpts[0])
	case "all":
		gateSelect.SetSelected(gateOpts[2])
	default:
		gateSelect.SetSelected(gateOpts[1])
	}

	// ---- 界面语言：立即生效并落盘 ----
	langOpts := i18n.SelectOptions()
	langSelect := widget.NewSelect(langOpts, nil)
	idx := i18n.SelectIndex(cfg.Language)
	if idx >= 0 && idx < len(langOpts) {
		langSelect.SetSelected(langOpts[idx])
	}
	langSelect.OnChanged = func(label string) {
		newCfg := config.Get()
		newCfg.Language = i18n.PrefFromSelect(label)
		if err := config.Save(newCfg); err != nil {
			log.Errorf("保存语言失败: %v", err)
			return
		}
		i18n.Apply(newCfg.Language)
		if sw.tray != nil {
			sw.tray.ApplyLanguage()
		} else {
			sw.buildContent()
		}
	}

	// ---- 已发现实例列表 ----
	snap := sw.manager.Snapshot()
	instances := snap.Instances
	connPort := snap.ConnectedPort
	wsOpen := snap.WsOpen

	instanceRows := []fyne.CanvasObject{
		widget.NewLabelWithStyle(i18n.T("settings.instances"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	}
	if len(instances) == 0 {
		instanceRows = append(instanceRows, widget.NewLabel(i18n.T("tray.no_instances")))
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
				sw.buildContent()
				sw.win.Content().Refresh()
			})
		}
		instanceRows = append(instanceRows, btn)
	}
	refreshBtn := widget.NewButton(i18n.T("settings.refresh"), func() {
		sw.manager.DiscoverInstances()
		sw.buildContent()
		sw.win.Content().Refresh()
		if sw.tray != nil {
			sw.tray.Refresh()
		}
	})
	instanceRows = append(instanceRows, refreshBtn)

	statusLabel := widget.NewLabel("")

	saveBtn := widget.NewButton(i18n.T("settings.save"), func() {
		httpPort, _ := strconv.Atoi(httpPortEntry.Text)
		scanStart, _ := strconv.Atoi(scanStartEntry.Text)
		scanEnd, _ := strconv.Atoi(scanEndEntry.Text)
		interval, _ := strconv.Atoi(scanIntervalEntry.Text)

		newCfg := config.Get()
		newCfg.Enabled = enabledCheck.Checked
		newCfg.HTTPPort = httpPort
		newCfg.ScanPortStart = scanStart
		newCfg.ScanPortEnd = scanEnd
		newCfg.ScanIntervalSeconds = interval
		switch gateSelect.Selected {
		case i18n.T("settings.gate_off"):
			newCfg.WriteGate = "off"
		case i18n.T("settings.gate_all"):
			newCfg.WriteGate = "all"
		default:
			newCfg.WriteGate = "destructive"
		}
		if err := config.Save(newCfg); err != nil {
			statusLabel.SetText(i18n.T("settings.save_failed", err.Error()))
			log.Errorf("设置保存失败: %v", err)
			return
		}
		scanMin, scanMax := newCfg.ScanPortStart, newCfg.ScanPortEnd
		if scanMin > scanMax {
			scanMin, scanMax = scanMax, scanMin
		}
		if newCfg.HTTPPort >= scanMin && newCfg.HTTPPort <= scanMax {
			msg := fmt.Sprintf("MCP 端口 %d 与 UE 扫描区间 [%d, %d] 重叠，可能导致代理端口被误当 UE 实例探测",
				newCfg.HTTPPort, scanMin, scanMax)
			statusLabel.SetText(msg)
			log.Warn(msg)
		} else {
			statusLabel.SetText(i18n.T("settings.saved"))
		}
		sw.manager.Hub.SetWriteGate(proxy.ParseWriteGate(newCfg.WriteGate))
		if sw.tray != nil {
			sw.tray.Refresh()
		}
	})
	saveBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton(i18n.T("settings.close"), func() {
		sw.win.Hide()
	})

	form := container.NewVBox(
		widget.NewLabelWithStyle(i18n.T("settings.server_section"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		enabledCheck,
		container.NewGridWithColumns(2,
			widget.NewLabel(i18n.T("settings.http_port")),
			httpPortEntry,
			widget.NewLabel(i18n.T("settings.scan_start")),
			scanStartEntry,
			widget.NewLabel(i18n.T("settings.scan_end")),
			scanEndEntry,
			widget.NewLabel(i18n.T("settings.scan_interval")),
			scanIntervalEntry,
			widget.NewLabel(i18n.T("settings.write_gate")),
			gateSelect,
			widget.NewLabel(i18n.T("settings.language")),
			langSelect,
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
