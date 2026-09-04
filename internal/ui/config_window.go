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

const (
	protoStream     = "stream"
	protoSSE        = "sse"
	clientCursor    = "cursor"
	clientCodeBuddy = "codebuddy"
)

// configWindow 展示 MCP 客户端配置片段：协议 × 客户端各选一项，复制只写入当前一份。
type configWindow struct {
	app          fyne.App
	win          fyne.Window
	area         *widget.Entry
	configText   string
	lastPort     int
	protocol     string // stream | sse
	client       string // cursor | codebuddy
	selectedHost string

	streamBtn    *widget.Button
	sseBtn       *widget.Button
	cursorBtn    *widget.Button
	codebuddyBtn *widget.Button
}

func newConfigWindow(app fyne.App) *configWindow {
	cw := &configWindow{app: app, protocol: protoStream, client: clientCursor}

	w := app.NewWindow(i18n.T("mcp.title"))
	w.Resize(fyne.NewSize(540, 480))
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
	cw.win.SetContent(cw.buildContentWithPort(0))
	cw.refreshPreview()
	return cw
}

func (cw *configWindow) show(port int) {
	cw.lastPort = port
	cw.protocol = protoStream
	cw.client = clientCursor
	cw.selectedHost = ""
	cw.win.SetTitle(i18n.T("mcp.title"))
	cw.win.SetContent(cw.buildContentWithPort(port))
	cw.refreshPreview()
	cw.win.Show()
	cw.win.RequestFocus()
}

func (cw *configWindow) retranslate() {
	cw.win.SetTitle(i18n.T("mcp.title"))
	cw.win.SetContent(cw.buildContentWithPort(cw.lastPort))
	cw.refreshPreview()
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

func (cw *configWindow) refreshPreview() {
	token := config.Get().ProxyToken
	cw.setConfigText(buildMcpConfig(cw.protocol, cw.client, cw.lastPort, token, cw.copyHost()))
	cw.highlightButtons()
}

func (cw *configWindow) highlightButtons() {
	setOn := func(b *widget.Button, on bool) {
		if b == nil {
			return
		}
		if on {
			b.Importance = widget.HighImportance
		} else {
			b.Importance = widget.MediumImportance
		}
		b.Refresh()
	}
	setOn(cw.streamBtn, cw.protocol == protoStream)
	setOn(cw.sseBtn, cw.protocol == protoSSE)
	setOn(cw.cursorBtn, cw.client == clientCursor)
	setOn(cw.codebuddyBtn, cw.client == clientCodeBuddy)
}

func (cw *configWindow) buildContent() fyne.CanvasObject {
	return cw.buildContentWithPort(0)
}

func (cw *configWindow) buildContentWithPort(port int) fyne.CanvasObject {
	token := config.Get().ProxyToken
	cw.lastPort = port

	cw.streamBtn = widget.NewButton(i18n.T("mcp.stream"), func() {
		cw.protocol = protoStream
		cw.refreshPreview()
	})
	cw.sseBtn = widget.NewButton(i18n.T("mcp.sse"), func() {
		cw.protocol = protoSSE
		cw.refreshPreview()
	})
	cw.cursorBtn = widget.NewButton(i18n.T("mcp.cursor"), func() {
		cw.client = clientCursor
		cw.refreshPreview()
	})
	cw.codebuddyBtn = widget.NewButton(i18n.T("mcp.codebuddy"), func() {
		cw.client = clientCodeBuddy
		cw.refreshPreview()
	})

	copyBtn := widget.NewButton(i18n.T("mcp.copy"), func() {
		if cw.configText != "" {
			cw.app.Clipboard().SetContent(cw.configText)
		}
	})
	copyBtn.Importance = widget.HighImportance

	copyTokenBtn := widget.NewButton(i18n.T("mcp.copy_token"), func() {
		if token != "" {
			cw.app.Clipboard().SetContent(token)
		}
	})

	protoRow := container.NewBorder(nil, nil, nil, container.NewHBox(copyTokenBtn, copyBtn),
		container.NewHBox(cw.streamBtn, cw.sseBtn),
	)
	clientRow := container.NewHBox(cw.cursorBtn, cw.codebuddyBtn)

	header := container.NewVBox(protoRow, clientRow)
	if hostRow := cw.buildHostSelect(); hostRow != nil {
		header.Add(hostRow)
	}
	header.Add(widget.NewSeparator())

	cw.highlightButtons()
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
		cw.refreshPreview()
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

func buildMcpConfig(protocol, client string, port int, token, host string) string {
	headers := mcpAuthHeadersJSON(token)
	path := "/stream"
	if protocol == protoSSE {
		path = "/sse"
	}
	if client == clientCodeBuddy {
		if protocol == protoStream {
			return fmt.Sprintf(
				i18n.T("mcp.comment_codebuddy")+"\n"+
					"\"Nexus\": {\n"+
					"  \"url\": \"http://%s:%d%s\",\n"+
					"  \"transportType\": \"streamable-http\"%s\n"+
					"}",
				host, port, path, headers,
			)
		}
		return fmt.Sprintf(
			i18n.T("mcp.comment_codebuddy")+"\n"+
				"\"Nexus\": {\n"+
				"  \"url\": \"http://%s:%d%s\"%s\n"+
				"}",
			host, port, path, headers,
		)
	}
	return fmt.Sprintf(
		i18n.T("mcp.comment_cursor")+"\n"+
			"\"nexus-unreal\": {\n"+
			"  \"url\": \"http://%s:%d%s\"%s\n"+
			"}",
		host, port, path, headers,
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
