// Copyright byteyang. All Rights Reserved.

package ui

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/bytepine/NexusDesktop/internal/config"
	"github.com/bytepine/NexusDesktop/internal/i18n"
	"github.com/bytepine/NexusDesktop/internal/unreal"
)

func visitUI(o fyne.CanvasObject, fn func(fyne.CanvasObject)) {
	fn(o)
	switch v := o.(type) {
	case *fyne.Container:
		for _, child := range v.Objects {
			visitUI(child, fn)
		}
	case *container.Scroll:
		visitUI(v.Content, fn)
	case *container.AppTabs:
		for _, item := range v.Items {
			visitUI(item.Content, fn)
		}
	case *widget.Form:
		for _, item := range v.Items {
			visitUI(item.Widget, fn)
		}
	case *widget.Accordion:
		for _, item := range v.Items {
			visitUI(item.Detail, fn)
		}
	}
}

func uiButton(t *testing.T, o fyne.CanvasObject, label string) *widget.Button {
	t.Helper()
	var found *widget.Button
	visitUI(o, func(o fyne.CanvasObject) {
		if b, ok := o.(*widget.Button); ok && b.Text == label {
			found = b
		}
	})
	if found == nil {
		t.Fatalf("button %q missing", label)
	}
	return found
}

// 可选输出由真实控件渲染的预览，不使用用户的配置或凭证。
func captureUI(t *testing.T, w fyne.Window, name string) {
	t.Helper()
	dir := os.Getenv("NEXUS_UI_PREVIEW_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, w.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
}

func assertActionVisible(t *testing.T, app fyne.App, w fyne.Window, action *widget.Button) {
	t.Helper()
	p := app.Driver().AbsolutePositionForObject(action)
	s := w.Canvas().Size()
	if p.Y < 0 || p.X < 0 || p.Y+action.Size().Height > s.Height || p.X+action.Size().Width > s.Width {
		t.Fatalf("action clipped: pos=%v size=%v canvas=%v", p, action.Size(), s)
	}
}

func TestWindowFlows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("此保存流程使用 Windows 的隔离配置目录")
	}
	// config 包缓存配置目录，因此整组保存流程共用一个临时目录。
	t.Setenv("APPDATA", t.TempDir())
	localData := t.TempDir()
	t.Setenv("LOCALAPPDATA", localData)
	if err := os.MkdirAll(filepath.Join(localData, "NexusLink"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localData, "NexusLink", "mcp-auth-token"), []byte(strings.Repeat("0", 64)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Language = i18n.LangZhCN
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	i18n.Apply(cfg.Language)
	app := test.NewTempApp(t)
	app.Settings().SetTheme(theme.DarkTheme())
	sw := NewSettingsWindow(app, unreal.NewManager())
	sw.Show()
	var tabs *container.AppTabs
	var port *widget.Entry
	var language *widget.Select
	visitUI(sw.win.Content(), func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *container.AppTabs:
			tabs = v
		case *widget.Entry:
			if v.Text == "6700" {
				port = v
			}
		case *widget.Select:
			if len(v.Options) == 3 && v.Options[1] == "简体中文" {
				language = v
			}
		}
	})
	if tabs == nil || port == nil || language == nil {
		t.Fatal("settings controls missing")
	}
	if len(tabs.Items) != 3 {
		t.Fatal("settings sections missing")
	}
	for i, name := range []string{"settings-general", "settings-connection", "settings-security"} {
		tabs.SelectIndex(i)
		captureUI(t, sw.win, name)
		assertActionVisible(t, app, sw.win, uiButton(t, sw.win.Content(), "保存"))
	}
	port.SetText("6701")
	language.SetSelected("English")
	tabs.SelectIndex(0)
	sw.Show()
	if port.Text != "6701" || sw.status.Text != "" || config.Get().Language != i18n.LangZhCN {
		t.Fatal("draft changed before Save")
	}
	port.SetText("invalid")
	test.Tap(uiButton(t, sw.win.Content(), "保存"))
	if config.Get().HTTPPort != 6700 || sw.status.Text != i18n.T("settings.invalid_port") {
		t.Fatal("invalid port saved or missing feedback")
	}
	if tabs.SelectedIndex() != 1 || sw.win.Canvas().Focused() != port {
		t.Fatal("invalid field not revealed")
	}
	port.SetText("6701")
	test.Tap(uiButton(t, sw.win.Content(), "保存"))
	if config.Get().HTTPPort != 6701 || config.Get().Language != i18n.LangEn || i18n.Current() != i18n.LangEn || sw.status.Text != "Saved" {
		t.Fatal("language and draft did not save together")
	}
	sw.win.Resize(fyne.NewSize(520, 360))
	captureUI(t, sw.win, "settings-english-small")
	assertActionVisible(t, app, sw.win, uiButton(t, sw.win.Content(), "Save"))

	box, collect := newExtraTokenList("")
	if len(box.(*fyne.Container).Objects) != 1 {
		t.Fatal("empty token row shown")
	}
	test.Tap(uiButton(t, box, "Add token"))
	visitUI(box, func(o fyne.CanvasObject) {
		if e, ok := o.(*widget.Entry); ok {
			e.SetText(" example ")
		}
	})
	if collect() != "example" {
		t.Fatal("token not collected")
	}
	test.Tap(uiButton(t, box, "Remove"))
	if collect() != "" || len(box.(*fyne.Container).Objects) != 1 {
		t.Fatal("last token not removed")
	}

	cw := newConfigWindow(app)
	cw.show(6702)
	for _, protocol := range []string{protoStream, protoSSE} {
		for _, client := range []string{clientCursor, clientHTTP, clientCodeBuddy, clientCline, clientRoo} {
			// 经实际下拉回调切换，覆盖选项到配置格式的映射。
			visitUI(cw.win.Content(), func(o fyne.CanvasObject) {
				if selectWidget, ok := o.(*widget.Select); ok {
					for _, option := range selectWidget.Options {
						if option == i18n.T("mcp."+client) {
							selectWidget.SetSelected(option)
						}
					}
				}
			})
			cw.protocol = protocol
			cw.refreshPreview()
			if cw.client != client {
				t.Fatal("client selection not applied")
			}
			if cw.help.Text != i18n.T("mcp.comment_"+client) {
				t.Fatal("incorrect placement instructions")
			}
			if !json.Valid([]byte("{" + cw.configText + "}")) {
				t.Fatal("copied fragment is not a valid JSON member")
			}
			if !strings.Contains(cw.configText, ":6702/"+protocol) {
				t.Fatal("actual port or protocol missing")
			}
			if strings.Contains(cw.configText, "transportType") != (client == clientCodeBuddy && protocol == protoStream) {
				t.Fatal("incorrect transport type")
			}
			var fragment map[string]struct {
				Type    string            `json:"type"`
				Headers map[string]string `json:"headers"`
			}
			if err := json.Unmarshal([]byte("{"+cw.configText+"}"), &fragment); err != nil {
				t.Fatal(err)
			}
			wantType := map[string]string{clientHTTP: "http", clientCline: "streamableHttp", clientRoo: "streamable-http"}[client]
			if protocol == protoSSE && wantType != "" {
				wantType = "sse"
			}
			for _, entry := range fragment {
				if entry.Type != wantType {
					t.Fatalf("%s/%s: type=%q want %q", client, protocol, entry.Type, wantType)
				}
				if entry.Headers["Authorization"] != "Bearer "+config.Get().ProxyToken {
					t.Fatal("auth header missing")
				}
			}
			test.Tap(uiButton(t, cw.win.Content(), "Copy configuration"))
			if app.Clipboard().Content() != cw.configText || cw.status.Text != "Configuration copied" {
				t.Fatal("copy or feedback failed")
			}
		}
	}
	cw.show(6702)
	captureUI(t, cw.win, "client-config")
	cw.win.Resize(fyne.NewSize(520, 360))
	assertActionVisible(t, app, cw.win, uiButton(t, cw.win.Content(), "Copy configuration"))
	captureUI(t, cw.win, "client-config-small")

	var clientSelect, protocolSelect *widget.Select
	visitUI(cw.win.Content(), func(o fyne.CanvasObject) {
		if s, ok := o.(*widget.Select); ok {
			for _, option := range s.Options {
				if option == "Codex" {
					clientSelect = s
				}
				if option == "SSE" {
					protocolSelect = s
				}
			}
		}
	})
	if clientSelect == nil || protocolSelect == nil {
		t.Fatal("Codex selector missing")
	}
	protocolSelect.SetSelected("SSE")
	clientSelect.SetSelected("Codex")
	if cw.protocol != protoStream || !protocolSelect.Disabled() {
		t.Fatal("Codex must select Streamable HTTP")
	}
	var codexConfig struct {
		Servers map[string]struct {
			URL     string            `toml:"url"`
			Headers map[string]string `toml:"http_headers"`
		} `toml:"mcp_servers"`
	}
	if _, err := toml.Decode(cw.configText, &codexConfig); err != nil {
		t.Fatal(err)
	}
	server := codexConfig.Servers["nexus-unreal"]
	if server.URL != "http://127.0.0.1:6702/stream" || server.Headers["Authorization"] != "Bearer "+config.Get().ProxyToken {
		t.Fatal("invalid Codex endpoint or authentication")
	}
	test.Tap(uiButton(t, cw.win.Content(), "Copy configuration"))
	if app.Clipboard().Content() != cw.configText {
		t.Fatal("Codex copy failed")
	}
	assertActionVisible(t, app, cw.win, uiButton(t, cw.win.Content(), "Copy configuration"))
	captureUI(t, cw.win, "client-config-codex")
	clientSelect.SetSelected(i18n.T("mcp.cursor"))
	if protocolSelect.Disabled() {
		t.Fatal("protocol selector remains disabled after leaving Codex")
	}
	cfg = config.Get()
	cfg.RequireAuth = false
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buildMcpConfig(protoSSE, clientCodex, 6702, cfg.ProxyToken, "127.0.0.1"), "http_headers") {
		t.Fatal("Codex includes authentication while disabled")
	}
}
