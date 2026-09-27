package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/config"
	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/keyring"
)

// configured returns a model with settings from a config file in a
// temporary directory and a credential store in memory.
func configured(t *testing.T, file string) (*Model, string, *keyring.Memory) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if file != "" {
		os.WriteFile(path, []byte(file), 0o600)
	}
	load := func() *config.Config { return config.Load(path, func(string) string { return "" }, nil) }
	store := &keyring.Memory{}
	m := newModel()
	m.Configure(Settings{Config: load(), Reload: load, Keys: store,
		Connect: func(key string) (jev.Client, error) { return &fakeJEV{}, nil }})
	return m, path, store
}

func TestThemeFromConfig(t *testing.T) {
	m, _, _ := configured(t, "theme = Dracula\nbogus = 1\n")
	if m.th.Name != "Dracula" {
		t.Errorf("theme %q", m.th.Name)
	}
	m, _, _ = configured(t, "theme = light:Builtin Solarized Light,dark:Dracula\n")
	if m.th.Name != "Dracula" {
		t.Errorf("dark terminal: %q", m.th.Name)
	}
	send(m, tea.BackgroundColorMsg{Color: ansi.White})
	if m.th.Name != "Builtin Solarized Light" {
		t.Errorf("light terminal: %q", m.th.Name)
	}
	m, _, _ = configured(t, "theme = No Such Theme\n")
	if m.th.Name != "terminal" || !strings.Contains(line(m, contextLine), `no theme "No Such Theme"`) {
		t.Errorf("unknown theme: %q, %q", m.th.Name, line(m, contextLine))
	}
}

// With a color scheme every line is drawn out to the terminal's edge,
// at odd widths too, and with an overlay open.
func TestBarsFillTheWidth(t *testing.T) {
	m, _, _ := configured(t, "theme = 1-2-3 Classic\n")
	for _, w := range []int{61, 80, 97} {
		send(m, tea.WindowSizeMsg{Width: w, Height: 17})
		checkFilled(t, m, "no overlay")
		press(t, m, "<alt+f>")
		checkFilled(t, m, "menu open")
		press(t, m, "<esc>")
	}
}

// checkFilled fails unless every line is the terminal's width and every
// cell has a background: the bars' or the scheme's.
func checkFilled(t *testing.T, m *Model, when string) {
	t.Helper()
	content := m.View().Content
	lines := strings.Split(content, "\n")
	if len(lines) != m.height {
		t.Fatalf("%s: %d lines, height %d", when, len(lines), m.height)
	}
	for i, l := range lines {
		if got := ansi.StringWidth(l); got != m.width {
			t.Errorf("%s, width %d: line %d is %d wide", when, m.width, i, got)
		}
	}
	scr := uv.NewScreenBuffer(m.width, m.height)
	uv.NewStyledString(content).Draw(scr, scr.Bounds())
	for y := range m.height {
		for x := range m.width {
			if c := scr.CellAt(x, y); c != nil && c.Width > 0 && c.Style.Bg == nil {
				t.Fatalf("%s, width %d: no background at %d,%d", when, m.width, x, y)
			}
		}
	}
}

func TestTerminalThemeUnchanged(t *testing.T) {
	m, _, _ := configured(t, "")
	if strings.Contains(m.View().Content, "48;2;") {
		t.Error("the terminal theme uses true color")
	}
}

func TestThemePicker(t *testing.T) {
	m, path, _ := configured(t, "# mine\ntheme = Nord\n")
	press(t, m, "<alt+f>")
	openSettingsItem(t, m, "Theme")
	if m.th.Name != "nord" || !strings.Contains(screen(m), "│ nord") {
		t.Fatalf("picker opens on the current theme: %q\n%s", m.th.Name, screen(m))
	}
	press(t, m, "Dracula")
	if m.th.Name != "Dracula" {
		t.Errorf("preview: %q", m.th.Name)
	}
	press(t, m, "<esc>")
	if m.th.Name != "nord" || m.overlay != nil {
		t.Errorf("Esc restores: %q", m.th.Name)
	}
	press(t, m, "<alt+f>")
	openSettingsItem(t, m, "Theme")
	press(t, m, "1-2-3 Classic", "<enter>")
	if m.th.Name != "1-2-3 Classic" || !strings.Contains(line(m, contextLine), "saved in") {
		t.Errorf("Enter keeps it: %q, %q", m.th.Name, line(m, contextLine))
	}
	data, _ := os.ReadFile(path)
	if string(data) != "# mine\ntheme = 1-2-3 Classic\n" {
		t.Errorf("config file: %q", data)
	}
}

// openSettingsItem opens File > Settings and runs the item called title.
func openSettingsItem(t *testing.T, m *Model, title string) {
	t.Helper()
	for range 30 {
		if selectedTitle(m) == "Settings" {
			break
		}
		press(t, m, "<down>")
	}
	press(t, m, "<right>")
	for range 10 {
		if selectedTitle(m) == title {
			press(t, m, "<enter>")
			return
		}
		press(t, m, "<down>")
	}
	t.Fatalf("no %s in Settings:\n%s", title, screen(m))
}

// selectedTitle is the highlighted item of the open menu.
func selectedTitle(m *Model) string {
	mo, ok := m.overlay.(*menuOverlay)
	if !ok {
		return ""
	}
	l := mo.top()
	if l.sel < 0 {
		return ""
	}
	return l.items[l.sel].label()
}

func TestAPIKeyPrompt(t *testing.T) {
	const key = "ts-secret-9876"
	m, _, store := configured(t, "")
	m.runCommand("settings.jev_key")
	press(t, m, key)
	ctx := line(m, contextLine)
	if strings.Contains(screen(m), key) || !strings.Contains(ctx, "TypeSafe API key: "+strings.Repeat("•", len(key))) {
		t.Fatalf("key shown: %q", ctx)
	}
	if x, y, _ := m.cursorPos(); y != contextLine || x != len("TypeSafe API key: ")+len(key) {
		t.Errorf("cursor at %d,%d", x, y)
	}
	press(t, m, "<enter>")
	if got, _ := store.Get(context.Background()); got != key {
		t.Errorf("stored %q", got)
	}
	if m.jev == nil || !strings.Contains(line(m, contextLine), "Key saved and checked; it's in the test store") {
		t.Errorf("JEV not on: %q", line(m, contextLine))
	}
	if strings.Contains(screen(m), key) || m.line.Text() != "" {
		t.Error("the key is still around")
	}

	store.Err = errors.New("keychain locked")
	m.runCommand("settings.jev_key")
	press(t, m, "x", "<enter>")
	if !strings.Contains(screen(m), "Couldn't save the API key: keychain locked") {
		t.Errorf("failure not shown:\n%s", screen(m))
	}
}

func TestKeyNeedsAStore(t *testing.T) {
	m := newModel()
	if commands["settings.jev_key"].available(m) || commands["settings.config_reload"].available(m) || commands["settings.config_edit"].available(m) {
		t.Error("settings that need a config or store are available without one")
	}
}

func TestReloadConfig(t *testing.T) {
	m, path, _ := configured(t, "theme = nord\n")
	os.WriteFile(path, []byte("theme = Dracula\nchart-images = false\nnotifications = false\n"), 0o600)
	m.runCommand("settings.config_reload")
	if m.th.Name != "Dracula" || !m.term.noImages || !m.term.noNotify || !strings.Contains(line(m, contextLine), "Config reloaded") {
		t.Errorf("reload: %q images off %v, %q", m.th.Name, m.term.noImages, line(m, contextLine))
	}
	os.WriteFile(path, []byte("wat = 1\n"), 0o600)
	m.runCommand("settings.config_reload")
	if !strings.Contains(line(m, contextLine), "problems") {
		t.Errorf("warnings: %q", line(m, contextLine))
	}
	if m.th.Name != "terminal" {
		t.Errorf("theme back to the default: %q", m.th.Name)
	}
}

func TestChartImagesOff(t *testing.T) {
	m, _, _ := configured(t, "chart-images = false\n")
	m.term.kitty = true
	if m.term.images() || m.term.chartOptions().Image {
		t.Error("images drawn although chart-images = false")
	}
}
