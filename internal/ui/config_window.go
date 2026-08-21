// Copyright byteyang. All Rights Reserved.

package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/bytepine/NexusDesktop/internal/config"
	"github.com/bytepine/NexusDesktop/internal/i18n"
)

// configWindow 展示 MCP 客户端配置片段，提供 Streamable HTTP / SSE 切换与一键复制。
// 参考 NexusRider NexusLinkConfigurable 的设计：先选类型，再复制。
type configWindow struct {
	app        fyne.App
	win        fyne.Window
	area       *widget.Entry
	configText string
	lastPort   int
	kind       string // "" | "stream" | "sse"
}

func newConfigWindow(app fyne.App) *configWindow {
	cw := &configWindow{app: app}

	w := app.NewWindow(i18n.T("mcp.title"))
	w.Resize(fyne.NewSize(540, 400))
	w.SetFixedSize(true)
	w.SetCloseIntercept(func() { w.Hide() })
	cw.win = w

	area := widget.NewMultiLineEntry()
	area.Wrapping = fyne.TextWrapOff
	area.TextStyle = fyne.TextStyle{Monospace: true}
	area.SetMinRowsVisible(12)
	cw.area = area
	area.OnChanged = func(s string) {
		if s != cw.configText {
			area.SetText(cw.configText)
		}
	}
	cw.setConfigText(i18n.T("mcp.placeholder"))
	cw.win.SetContent(cw.buildContentWithPort(0))
	return cw
}

func (cw *configWindow) show(port int) {
	cw.lastPort = port
	cw.kind = ""
	cw.setConfigText(i18n.T("mcp.placeholder"))
	cw.win.SetTitle(i18n.T("mcp.title"))
	cw.win.SetContent(cw.buildContentWithPort(port))
	cw.win.Show()
	cw.win.RequestFocus()
}

func (cw *configWindow) retranslate() {
	cw.win.SetTitle(i18n.T("mcp.title"))
	switch cw.kind {
	case "stream":
		cw.setConfigText(buildStreamConfig(cw.lastPort, config.Get().ProxyToken))
	case "sse":
		cw.setConfigText(buildSseConfig(cw.lastPort, config.Get().ProxyToken))
	default:
		cw.setConfigText(i18n.T("mcp.placeholder"))
	}
	cw.win.SetContent(cw.buildContentWithPort(cw.lastPort))
}

func (cw *configWindow) buildContent() fyne.CanvasObject {
	return cw.buildContentWithPort(0)
}

func (cw *configWindow) buildContentWithPort(port int) fyne.CanvasObject {
	token := config.Get().ProxyToken
	streamBtn := widget.NewButton(i18n.T("mcp.stream"), func() {
		cw.kind = "stream"
		cw.lastPort = port
		cw.setConfigText(buildStreamConfig(port, token))
	})

	sseBtn := widget.NewButton(i18n.T("mcp.sse"), func() {
		cw.kind = "sse"
		cw.lastPort = port
		cw.setConfigText(buildSseConfig(port, token))
	})

	copyBtn := widget.NewButton(i18n.T("mcp.copy"), func() {
		ph := i18n.T("mcp.placeholder")
		if cw.configText != "" && cw.configText != ph {
			cw.app.Clipboard().SetContent(cw.configText)
		}
	})
	copyBtn.Importance = widget.HighImportance

	topBar := container.NewBorder(nil, nil, nil, copyBtn,
		container.NewHBox(streamBtn, sseBtn),
	)

	return container.NewBorder(
		container.NewVBox(topBar, widget.NewSeparator()),
		nil, nil, nil,
		container.NewScroll(cw.area),
	)
}

func (cw *configWindow) setConfigText(text string) {
	cw.configText = text
	cw.area.SetText(text)
}

func buildStreamConfig(port int, token string) string {
	return fmt.Sprintf(
		i18n.T("mcp.comment_cursor")+"\n"+
			"\"nexus-unreal\": {\n"+
			"  \"url\": \"http://127.0.0.1:%d/stream\",\n"+
			"  \"headers\": {\n"+
			"    \"Authorization\": \"Bearer %s\"\n"+
			"  }\n"+
			"}\n\n"+
			"# CodeBuddy / Windsurf\n"+
			"\"Nexus\": {\n"+
			"  \"url\": \"http://127.0.0.1:%d/stream\",\n"+
			"  \"transportType\": \"streamable-http\",\n"+
			"  \"headers\": {\n"+
			"    \"Authorization\": \"Bearer %s\"\n"+
			"  }\n"+
			"}",
		port, token, port, token,
	)
}

func buildSseConfig(port int, token string) string {
	return fmt.Sprintf(
		i18n.T("mcp.comment_cursor")+"\n"+
			"\"nexus-unreal\": {\n"+
			"  \"url\": \"http://127.0.0.1:%d/sse\",\n"+
			"  \"headers\": {\n"+
			"    \"Authorization\": \"Bearer %s\"\n"+
			"  }\n"+
			"}\n\n"+
			"# CodeBuddy / Windsurf\n"+
			"\"Nexus\": {\n"+
			"  \"url\": \"http://127.0.0.1:%d/sse\",\n"+
			"  \"headers\": {\n"+
			"    \"Authorization\": \"Bearer %s\"\n"+
			"  }\n"+
			"}",
		port, token, port, token,
	)
}
