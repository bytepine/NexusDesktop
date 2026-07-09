// Copyright byteyang. All Rights Reserved.

package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// configWindow 展示 MCP 客户端配置片段，提供 Streamable HTTP / SSE 切换与一键复制。
// 参考 NexusRider NexusLinkConfigurable 的设计：先选类型，再复制。
type configWindow struct {
	app        fyne.App
	win        fyne.Window
	area       *widget.Entry // 只读多行文本区，用于展示配置
	configText string        // 当前展示的配置片段（复制 / 防误编辑用）
}

// newConfigWindow 懒创建配置窗口（初始隐藏）。
func newConfigWindow(app fyne.App) *configWindow {
	cw := &configWindow{app: app}

	w := app.NewWindow("MCP 客户端配置")
	w.Resize(fyne.NewSize(540, 400))
	w.SetFixedSize(true)
	w.SetCloseIntercept(func() { w.Hide() })
	cw.win = w

	// 只读多行文本区：保持启用态以使用前景色（Disable 对比度过低），OnChanged 拦截误编辑
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
	cw.setConfigText(placeholder)

	cw.win.SetContent(cw.buildContent())
	return cw
}

const placeholder = "← 点击上方按钮生成对应配置"

// show 刷新内容并显示窗口，port 为当前 MCP HTTP 端口。
func (cw *configWindow) show(port int) {
	cw.setConfigText(placeholder)
	// 用新端口重建按钮行（端口可能随设置变化）
	cw.win.SetContent(cw.buildContentWithPort(port))
	cw.win.Show()
	cw.win.RequestFocus()
}

func (cw *configWindow) buildContent() fyne.CanvasObject {
	return cw.buildContentWithPort(0)
}

func (cw *configWindow) buildContentWithPort(port int) fyne.CanvasObject {
	streamBtn := widget.NewButton("Streamable HTTP 配置", func() {
		cw.setConfigText(buildStreamConfig(port))
	})

	sseBtn := widget.NewButton("SSE 配置", func() {
		cw.setConfigText(buildSseConfig(port))
	})

	copyBtn := widget.NewButton("复制", func() {
		if cw.configText != "" && cw.configText != placeholder {
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

func buildStreamConfig(port int) string {
	return fmt.Sprintf(
		"# Cursor  (~/.cursor/mcp.json → mcpServers 节点)\n"+
			"\"nexus-unreal\": {\n"+
			"  \"url\": \"http://127.0.0.1:%d/stream\"\n"+
			"}\n\n"+
			"# CodeBuddy / Windsurf\n"+
			"\"Nexus\": {\n"+
			"  \"url\": \"http://127.0.0.1:%d/stream\",\n"+
			"  \"transportType\": \"streamable-http\"\n"+
			"}",
		port, port,
	)
}

func buildSseConfig(port int) string {
	return fmt.Sprintf(
		"# Cursor  (~/.cursor/mcp.json → mcpServers 节点)\n"+
			"\"nexus-unreal\": {\n"+
			"  \"url\": \"http://127.0.0.1:%d/sse\"\n"+
			"}\n\n"+
			"# CodeBuddy / Windsurf\n"+
			"\"Nexus\": {\n"+
			"  \"url\": \"http://127.0.0.1:%d/sse\"\n"+
			"}",
		port, port,
	)
}
