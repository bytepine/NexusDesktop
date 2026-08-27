// Copyright byteyang. All Rights Reserved.

package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/bytepine/NexusDesktop/internal/config"
	"github.com/bytepine/NexusDesktop/internal/i18n"
	"github.com/bytepine/NexusDesktop/internal/unreal"
)

// configWindow 展示 MCP 客户端配置片段，提供 Streamable HTTP / SSE 切换与一键复制。
// 参考 NexusRider NexusLinkConfigurable 的设计：先选类型，再复制。
type configWindow struct {
	app          fyne.App
	win          fyne.Window
	area         *widget.Entry
	configText   string
	lastPort     int
	kind         string // "" | "stream" | "sse"
	selectedHost string
}

func newConfigWindow(app fyne.App) *configWindow {
	cw := &configWindow{app: app}

	w := app.NewWindow(i18n.T("mcp.title"))
	w.Resize(fyne.NewSize(540, 440))
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
	cw.selectedHost = ""
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
		cw.setConfigText(buildStreamConfig(cw.lastPort, config.Get().ProxyToken, cw.copyHost()))
	case "sse":
		cw.setConfigText(buildSseConfig(cw.lastPort, config.Get().ProxyToken, cw.copyHost()))
	default:
		cw.setConfigText(i18n.T("mcp.placeholder"))
	}
	cw.win.SetContent(cw.buildContentWithPort(cw.lastPort))
}

func (cw *configWindow) copyHost() string {
	if cw.selectedHost != "" {
		return cw.selectedHost
	}
	auto, _ := unreal.CopyHostChoices(config.Get().ListenLan)
	if auto != "" {
		return auto
	}
	return unreal.LoopbackHost
}

func (cw *configWindow) buildContent() fyne.CanvasObject {
	return cw.buildContentWithPort(0)
}

func (cw *configWindow) buildContentWithPort(port int) fyne.CanvasObject {
	token := config.Get().ProxyToken
	streamBtn := widget.NewButton(i18n.T("mcp.stream"), func() {
		cw.kind = "stream"
		cw.lastPort = port
		cw.setConfigText(buildStreamConfig(port, token, cw.copyHost()))
	})

	sseBtn := widget.NewButton(i18n.T("mcp.sse"), func() {
		cw.kind = "sse"
		cw.lastPort = port
		cw.setConfigText(buildSseConfig(port, token, cw.copyHost()))
	})

	copyBtn := widget.NewButton(i18n.T("mcp.copy"), func() {
		ph := i18n.T("mcp.placeholder")
		if cw.configText != "" && cw.configText != ph {
			cw.app.Clipboard().SetContent(cw.configText)
		}
	})
	copyBtn.Importance = widget.HighImportance

	copyTokenBtn := widget.NewButton(i18n.T("mcp.copy_token"), func() {
		if token != "" {
			cw.app.Clipboard().SetContent(token)
		}
	})

	topBar := container.NewBorder(nil, nil, nil, container.NewHBox(copyTokenBtn, copyBtn),
		container.NewHBox(streamBtn, sseBtn),
	)

	header := container.NewVBox(topBar)
	if hostRow := cw.buildHostSelect(); hostRow != nil {
		header.Add(hostRow)
	}
	header.Add(widget.NewSeparator())

	return container.NewBorder(
		header,
		nil, nil, nil,
		container.NewScroll(cw.area),
	)
}

func (cw *configWindow) buildHostSelect() fyne.CanvasObject {
	auto, choices := unreal.CopyHostChoices(config.Get().ListenLan)
	if len(choices) == 0 {
		cw.selectedHost = auto
		return nil
	}
	labels := make([]string, len(choices))
	labelToAddr := make(map[string]string, len(choices))
	for i, c := range choices {
		labels[i] = unreal.LanAddrLabel(c)
		labelToAddr[labels[i]] = c.Address
	}
	sel := widget.NewSelect(labels, func(s string) {
		cw.selectedHost = labelToAddr[s]
		if cw.kind == "stream" {
			cw.setConfigText(buildStreamConfig(cw.lastPort, config.Get().ProxyToken, cw.selectedHost))
		} else if cw.kind == "sse" {
			cw.setConfigText(buildSseConfig(cw.lastPort, config.Get().ProxyToken, cw.selectedHost))
		}
	})
	if cw.selectedHost != "" {
		for _, c := range choices {
			if c.Address == cw.selectedHost {
				sel.SetSelected(unreal.LanAddrLabel(c))
				break
			}
		}
	}
	if sel.Selected == "" {
		sel.SetSelected(labels[0])
		cw.selectedHost = choices[0].Address
	}
	return container.NewBorder(nil, nil, widget.NewLabel(i18n.T("mcp.select_ip")), nil, sel)
}

func (cw *configWindow) setConfigText(text string) {
	cw.configText = text
	cw.area.SetText(text)
}

func buildStreamConfig(port int, token string, host string) string {
	headers := mcpAuthHeadersJSON(token)
	return fmt.Sprintf(
		i18n.T("mcp.comment_cursor")+"\n"+
			"\"nexus-unreal\": {\n"+
			"  \"url\": \"http://%s:%d/stream\"%s\n"+
			"}\n\n"+
			"# CodeBuddy / Windsurf\n"+
			"\"Nexus\": {\n"+
			"  \"url\": \"http://%s:%d/stream\",\n"+
			"  \"transportType\": \"streamable-http\"%s\n"+
			"}",
		host, port, headers, host, port, headers,
	)
}

func buildSseConfig(port int, token string, host string) string {
	headers := mcpAuthHeadersJSON(token)
	return fmt.Sprintf(
		i18n.T("mcp.comment_cursor")+"\n"+
			"\"nexus-unreal\": {\n"+
			"  \"url\": \"http://%s:%d/sse\"%s\n"+
			"}\n\n"+
			"# CodeBuddy / Windsurf\n"+
			"\"Nexus\": {\n"+
			"  \"url\": \"http://%s:%d/sse\"%s\n"+
			"}",
		host, port, headers, host, port, headers,
	)
}

func mcpAuthHeadersJSON(token string) string {
	if !config.Get().RequireAuth || token == "" {
		return ""
	}
	return ",\n" +
		"  \"headers\": {\n" +
		"    \"Authorization\": \"Bearer " + token + "\"\n" +
		"  }"
}
