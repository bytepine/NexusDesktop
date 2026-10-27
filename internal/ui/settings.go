// Copyright byteyang. All Rights Reserved.

package ui

import (
	"fmt"
	"strings"

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
	visible bool
	status  *widget.Label
}

// NewSettingsWindow 创建设置窗口（初始隐藏）。
func NewSettingsWindow(app fyne.App, mgr *unreal.Manager) *SettingsWindow {
	sw := &SettingsWindow{manager: mgr}
	w := app.NewWindow(i18n.T("settings.title"))
	w.Resize(fyne.NewSize(640, 560))
	// 关闭按钮仅隐藏，不退出
	w.SetCloseIntercept(func() {
		sw.hide()
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
	if !sw.visible {
		sw.buildContent()
	}
	sw.visible = true
	sw.win.Show()
	sw.win.RequestFocus()
}

func (sw *SettingsWindow) hide() {
	sw.visible = false
	sw.win.Hide()
}

func (sw *SettingsWindow) buildContent() {
	cfg := config.Get()
	sw.win.SetTitle(i18n.T("settings.title"))

	// ---- 服务器开关 ----
	enabledCheck := widget.NewCheck(i18n.T("settings.enable_proxy"), nil)
	enabledCheck.SetChecked(cfg.Enabled)

	listenLanCheck := widget.NewCheck(i18n.T("settings.listen_lan"), nil)
	listenLanCheck.SetChecked(cfg.ListenLan)

	requireAuthCheck := widget.NewCheck(i18n.T("settings.require_auth"), nil)
	requireAuthCheck.SetChecked(cfg.RequireAuth)

	extraTokensBox, collectExtraTokens := newExtraTokenList(cfg.ExtraAuthTokens)

	statusLabel := wrappedLabel("")
	sw.status = statusLabel

	tokenEntry := widget.NewPasswordEntry()
	tokenEntry.SetText(cfg.ProxyToken)
	tokenEntry.Disable()
	copyTokenBtn := widget.NewButton(i18n.T("settings.copy_token"), func() {
		if cfg.ProxyToken == "" {
			return
		}
		fyne.CurrentApp().Clipboard().SetContent(cfg.ProxyToken)
		statusLabel.SetText(i18n.T("settings.token_copied"))
	})

	remoteEntry := widget.NewMultiLineEntry()
	remoteEntry.SetText(formatRemoteLines(cfg.RemoteUnreal))
	remoteEntry.SetPlaceHolder(i18n.T("settings.remote_placeholder"))
	remoteEntry.Wrapping = fyne.TextWrapOff
	remoteEntry.SetMinRowsVisible(3)

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

	channelOpts := []string{
		i18n.T("settings.update_auto"),
		i18n.T("settings.update_stable"),
		i18n.T("settings.update_pre"),
	}
	channelSelect := widget.NewSelect(channelOpts, nil)
	switch cfg.UpdateChannel {
	case config.UpdateChannelStable:
		channelSelect.SetSelected(channelOpts[1])
	case config.UpdateChannelPre:
		channelSelect.SetSelected(channelOpts[2])
	default:
		channelSelect.SetSelected(channelOpts[0])
	}

	// ---- 界面语言与其他字段一起保存，避免切换时丢失草稿 ----
	langOpts := i18n.SelectOptions()
	langSelect := widget.NewSelect(langOpts, nil)
	idx := i18n.SelectIndex(cfg.Language)
	if idx >= 0 && idx < len(langOpts) {
		langSelect.SetSelected(langOpts[idx])
	}
	var tabs *container.AppTabs
	var connectionOptions *widget.Accordion
	showFieldError := func(entry *widget.Entry, message string) {
		tabs.SelectIndex(1)
		if entry != httpPortEntry {
			connectionOptions.Open(0)
		}
		statusLabel.SetText(message)
		sw.win.Canvas().Focus(entry)
	}
	saveBtn := widget.NewButton(i18n.T("settings.save"), func() {
		httpPort, err := config.ParsePortField(httpPortEntry.Text)
		if err != nil {
			showFieldError(httpPortEntry, i18n.T("settings.invalid_port"))
			return
		}
		scanStart, err := config.ParsePortField(scanStartEntry.Text)
		if err != nil {
			showFieldError(scanStartEntry, i18n.T("settings.invalid_port"))
			return
		}
		scanEnd, err := config.ParsePortField(scanEndEntry.Text)
		if err != nil {
			showFieldError(scanEndEntry, i18n.T("settings.invalid_port"))
			return
		}
		if err := config.ValidateScanRange(scanStart, scanEnd); err != nil {
			showFieldError(scanEndEntry, i18n.T("settings.scan_span_too_wide", config.MaxScanPortSpan))
			return
		}
		interval, err := config.ParseScanIntervalField(scanIntervalEntry.Text)
		if err != nil {
			showFieldError(scanIntervalEntry, i18n.T("settings.invalid_interval"))
			return
		}

		newCfg := config.Get()
		newCfg.Language = i18n.PrefFromSelect(langSelect.Selected)
		newCfg.Enabled = enabledCheck.Checked
		newCfg.ListenLan = listenLanCheck.Checked
		newCfg.RequireAuth = requireAuthCheck.Checked
		newCfg.ExtraAuthTokens = collectExtraTokens()
		newCfg.RemoteUnreal = parseRemoteLines(remoteEntry.Text)
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
		switch channelSelect.Selected {
		case i18n.T("settings.update_stable"):
			newCfg.UpdateChannel = config.UpdateChannelStable
		case i18n.T("settings.update_pre"):
			newCfg.UpdateChannel = config.UpdateChannelPre
		default:
			newCfg.UpdateChannel = config.UpdateChannelAuto
		}
		old := config.Get()
		enteredDanger := newCfg.ListenLan && !newCfg.RequireAuth && !(old.ListenLan && !old.RequireAuth)
		enteredRemote := len(old.RemoteUnreal) == 0 && len(newCfg.RemoteUnreal) > 0
		apply := func() {
			sw.applySavedConfig(newCfg, statusLabel)
		}
		confirmRemote := func() {
			if enteredRemote {
				showConfirmation(
					i18n.T("settings.remote_plain_warn_title"),
					i18n.T("settings.remote_plain_warn"),
					func(ok bool) {
						if ok {
							apply()
						}
					},
					sw.win,
				)
				return
			}
			apply()
		}
		if enteredDanger {
			showConfirmation(
				i18n.T("settings.lan_auth_warn_title"),
				i18n.T("settings.lan_auth_warn"),
				func(ok bool) {
					if ok {
						confirmRemote()
					}
				},
				sw.win,
			)
			return
		}
		confirmRemote()
	})
	saveBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton(i18n.T("settings.close"), func() {
		sw.hide()
	})

	general := container.NewVBox(
		enabledCheck,
		widget.NewForm(
			widget.NewFormItem(i18n.T("settings.language"), langSelect),
			widget.NewFormItem(i18n.T("settings.update_channel"), channelSelect),
		),
		wrappedLabel(i18n.T("settings.save_hint")),
	)
	scan := widget.NewForm(
		widget.NewFormItem(i18n.T("settings.scan_start"), scanStartEntry),
		widget.NewFormItem(i18n.T("settings.scan_end"), scanEndEntry),
		widget.NewFormItem(i18n.T("settings.scan_interval"), scanIntervalEntry),
	)
	connectionOptions = widget.NewAccordion(
		widget.NewAccordionItem(i18n.T("settings.scan_section"), scan),
		widget.NewAccordionItem(i18n.T("settings.remote_section"), container.NewVBox(
			wrappedLabel(i18n.T("settings.remote_unreal")), remoteEntry)),
	)
	connection := container.NewVBox(
		widget.NewForm(widget.NewFormItem(i18n.T("settings.http_port"), httpPortEntry)),
		listenLanCheck,
		wrappedLabel(i18n.T("settings.instances_hint")),
		connectionOptions,
	)
	security := container.NewVBox(
		requireAuthCheck,
		widget.NewForm(
			widget.NewFormItem(i18n.T("settings.write_gate"), gateSelect),
			widget.NewFormItem(i18n.T("settings.auth_token"), container.NewBorder(nil, nil, nil, copyTokenBtn, tokenEntry)),
		),
		widget.NewAccordion(widget.NewAccordionItem(i18n.T("settings.extra_tokens"), extraTokensBox)),
	)
	tabs = container.NewAppTabs(
		container.NewTabItem(i18n.T("settings.general"), container.NewVScroll(container.NewPadded(general))),
		container.NewTabItem(i18n.T("settings.connection"), container.NewVScroll(container.NewPadded(connection))),
		container.NewTabItem(i18n.T("settings.security"), container.NewVScroll(container.NewPadded(security))),
	)
	sw.win.SetContent(windowContent(nil, tabs, statusLabel, cancelBtn, saveBtn))
}

func (sw *SettingsWindow) applySavedConfig(newCfg config.Config, statusLabel *widget.Label) {
	languageChanged := config.Get().Language != newCfg.Language
	if err := config.Save(newCfg); err != nil {
		statusLabel.SetText(i18n.T("settings.save_failed", err.Error()))
		log.Errorf("设置保存失败: %v", err)
		return
	}
	if languageChanged {
		i18n.Apply(newCfg.Language)
		sw.buildContent()
		statusLabel = sw.status
		if sw.tray != nil {
			sw.tray.ApplyLanguage()
		}
	}
	scanMin, scanMax := newCfg.ScanPortStart, newCfg.ScanPortEnd
	if scanMin > scanMax {
		scanMin, scanMax = scanMax, scanMin
	}
	if newCfg.HTTPPort >= scanMin && newCfg.HTTPPort <= scanMax {
		msg := i18n.T("settings.port_overlap",
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
}

func formatRemoteLines(list []config.RemoteUnreal) string {
	if len(list) == 0 {
		return ""
	}
	lines := make([]string, 0, len(list))
	for _, r := range list {
		lines = append(lines, fmt.Sprintf("%s:%d %s", r.Host, r.McpPort, r.AuthToken))
	}
	return strings.Join(lines, "\n")
}

func parseRemoteLines(text string) []config.RemoteUnreal {
	parsed := unreal.ParseRemoteText(text)
	out := make([]config.RemoteUnreal, 0, len(parsed))
	for _, r := range parsed {
		out = append(out, config.RemoteUnreal{Host: r.Host, McpPort: r.McpPort, AuthToken: r.AuthToken})
	}
	return out
}

// newExtraTokenList 逐条输入额外 token（+ / −），保存时换行拼接。
func newExtraTokenList(raw string) (fyne.CanvasObject, func() string) {
	values := config.ParseAuthTokens(raw)
	entries := make([]*widget.Entry, 0, len(values))
	for _, v := range values {
		e := widget.NewPasswordEntry()
		e.SetPlaceHolder(i18n.T("settings.extra_token_placeholder"))
		e.SetText(v)
		entries = append(entries, e)
	}
	box := container.NewVBox()
	var rebuild func()
	rebuild = func() {
		objs := make([]fyne.CanvasObject, 0, len(entries)+1)
		for i := range entries {
			i := i
			del := widget.NewButton(i18n.T("settings.remove_token"), func() {
				entries = append(entries[:i], entries[i+1:]...)
				rebuild()
			})
			objs = append(objs, container.NewBorder(nil, nil, nil, del, entries[i]))
		}
		add := widget.NewButton(i18n.T("settings.add_token"), func() {
			e := widget.NewPasswordEntry()
			e.SetPlaceHolder(i18n.T("settings.extra_token_placeholder"))
			entries = append(entries, e)
			rebuild()
		})
		objs = append(objs, container.NewHBox(add))
		box.Objects = objs
		box.Refresh()
	}
	rebuild()
	collect := func() string {
		var parts []string
		for _, e := range entries {
			t := strings.TrimSpace(e.Text)
			if t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "\n")
	}
	return box, collect
}
