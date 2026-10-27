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
	clientHTTP      = "http" // Claude Code / VS Code
	clientCline     = "cline"
	clientRoo       = "roo"
	clientCodex     = "codex"
)

// configWindow 展示 MCP 客户端配置片段：协议 × 客户端各选一项，复制只写入当前一份。
type configWindow struct {
	app          fyne.App
	win          fyne.Window
	area         *widget.Entry
	configText   string
	lastPort     int
	protocol     string // stream | sse
	client       string // 按配置格式分组
	selectedHost string

	status *widget.Label
	help   *widget.Label
}

func newConfigWindow(app fyne.App) *configWindow {
	cw := &configWindow{app: app, protocol: protoStream, client: clientCursor}

	w := app.NewWindow(i18n.T("mcp.title"))
	w.Resize(fyne.NewSize(640, 560))
	w.SetCloseIntercept(func() { w.Hide() })
	cw.win = w

	area := widget.NewMultiLineEntry()
	area.Wrapping = fyne.TextWrapOff
	area.TextStyle = fyne.TextStyle{Monospace: true}
	area.SetMinRowsVisible(6)
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
	if cw.help != nil {
		cw.help.SetText(i18n.T("mcp.comment_" + cw.client))
	}
	if cw.status != nil {
		cw.status.SetText("")
	}
}

func (cw *configWindow) buildContentWithPort(port int) fyne.CanvasObject {
	cw.lastPort = port
	cw.status = wrappedLabel("")
	cw.help = wrappedLabel("")
	protocol := widget.NewSelect([]string{i18n.T("mcp.stream"), i18n.T("mcp.sse")}, nil)
	if cw.protocol == protoSSE {
		protocol.SetSelectedIndex(1)
	} else {
		protocol.SetSelectedIndex(0)
	}
	protocol.OnChanged = func(s string) {
		cw.protocol = protoStream
		if s == i18n.T("mcp.sse") {
			cw.protocol = protoSSE
		}
		cw.refreshPreview()
	}
	updateProtocol := func() {
		if cw.client == clientCodex {
			protocol.SetSelectedIndex(0)
			protocol.Disable()
		} else {
			protocol.Enable()
		}
	}
	clientIDs := []string{clientCursor, clientCodex, clientHTTP, clientCodeBuddy, clientCline, clientRoo}
	clientLabels := make([]string, len(clientIDs))
	selected := 0
	for i, id := range clientIDs {
		clientLabels[i] = i18n.T("mcp." + id)
		if cw.client == id {
			selected = i
		}
	}
	client := widget.NewSelect(clientLabels, nil)
	client.SetSelectedIndex(selected)
	client.OnChanged = func(_ string) {
		cw.client = clientIDs[client.SelectedIndex()]
		updateProtocol()
		cw.refreshPreview()
	}
	updateProtocol()
	header := container.NewVBox(widget.NewForm(
		widget.NewFormItem(i18n.T("mcp.client"), client),
		widget.NewFormItem(i18n.T("mcp.protocol"), protocol),
	))
	if hostRow := cw.buildHostSelect(); hostRow != nil {
		header.Add(hostRow)
	}
	header.Add(container.NewPadded(cw.help))
	copyBtn := widget.NewButton(i18n.T("mcp.copy"), func() {
		if cw.configText != "" {
			cw.app.Clipboard().SetContent(cw.configText)
			cw.status.SetText(i18n.T("mcp.copied"))
		}
	})
	copyBtn.Importance = widget.HighImportance
	closeBtn := widget.NewButton(i18n.T("settings.close"), func() { cw.win.Hide() })
	return windowContent(header, cw.area, cw.status, closeBtn, copyBtn)
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
	// Codex 的远程连接使用 Streamable HTTP，配置格式为 TOML。
	if client == clientCodex {
		text := fmt.Sprintf("[mcp_servers.nexus-unreal]\nurl = %q\n", fmt.Sprintf("http://%s:%d/stream", host, port))
		if config.Get().RequireAuth && token != "" {
			text += fmt.Sprintf("http_headers = { Authorization = %q }\n", "Bearer "+token)
		}
		return text
	}
	path := "/stream"
	if protocol == protoSSE {
		path = "/sse"
	}
	name, typeKey, transport := "nexus-unreal", "type", ""
	switch client {
	case clientCodeBuddy:
		name, typeKey = "Nexus", "transportType"
		if protocol == protoStream {
			transport = "streamable-http"
		}
	case clientHTTP:
		transport = "http"
	case clientCline:
		transport = "streamableHttp"
	case clientRoo:
		transport = "streamable-http"
	}
	if protocol == protoSSE && transport != "" {
		transport = "sse"
	}
	typeLine := ""
	if transport != "" {
		typeLine = fmt.Sprintf("  %q: %q,\n", typeKey, transport)
	}
	return fmt.Sprintf("%q: {\n%s  \"url\": %q%s\n}",
		name, typeLine, fmt.Sprintf("http://%s:%d%s", host, port, path), mcpAuthHeadersJSON(token))
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
