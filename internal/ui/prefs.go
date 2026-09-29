package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/config"
	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/keyring"
	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
	"github.com/FelineStateMachine/012/internal/ui/themepicker"
)

// Settings are what the app takes from outside the workbook: the config
// file's options, and the credential store the JEV API key lives in.
// cmd/012 passes them to Configure; tests pass fakes, so the user's own
// config and keychain are never touched.
type Settings struct {
	Config    *config.Config
	Reload    func() *config.Config // reads the config file again; nil disables Reload config
	ThemesDir string                // the user's theme files
	Keys      keyring.Store         // where JEV API key stores the key; nil disables it
	// Connect makes a JEV client for the configured service with key.
	Connect func(key string) (jev.Client, error)
	Notes   []string // said on the context line at startup, e.g. config warnings
	// RecoveryDir is where a local session keeps its unsaved work when
	// it stops on an internal error, and finds it again: recovery.go.
	// "" keeps none; 012 serve keeps them in the served directory.
	RecoveryDir string
}

// prefs is the settings in effect: the component behind File >
// Settings, the theme picker and the API key prompt.
type prefs struct {
	Settings
	light   bool   // the terminal's background is light (dark until it says)
	preview string // a theme being previewed by the picker
	vim     bool   // vim keys in READY mode (keymap = vim): see vim.go
}

// Configure applies settings: the theme, chart images, notifications,
// and notes for the context line.
func (m *Model) Configure(s Settings) {
	m.prefs.Settings = s
	notes := s.Notes
	if p := m.applyConfig(); p != "" {
		notes = append(notes, p)
	}
	if len(notes) > 0 {
		m.note = m.th.Warning.Render(strings.Join(notes, "   "))
	}
}

// applyConfig applies the options that take effect live and reports
// problems it finds, such as an unknown theme.
func (m *Model) applyConfig() string {
	c := m.prefs.Config
	m.term.noImages = !c.Bool("chart-images")
	m.term.noNotify = !c.Bool("notifications")
	m.SetVimKeys(c.String("keymap") == "vim")
	sheet.SetMaxCells(c.Int("max-cells"))
	if l, ok := locale.Lookup(c.String("locale")); ok {
		sheet.SetDefaultLocale(l)
	}
	return m.applyTheme()
}

// applyTheme draws with the theme chosen for the terminal's background,
// or the one being previewed. It returns why a theme couldn't be used.
func (m *Model) applyTheme() string {
	name := m.prefs.preview
	if name == "" {
		name = m.prefs.Config.Theme().Pick(!m.prefs.light)
	}
	th, err := theme.Resolve(name, !m.prefs.light, m.prefs.ThemesDir)
	m.th = th
	if err != nil {
		return err.Error()
	}
	return ""
}

func init() {
	register(
		&command{id: "settings.theme", macro: macroNever, title: "Theme",
			desc: "Pick a color theme, previewing each as you move; Esc keeps the one you had",
			run:  func(m *Model) tea.Cmd { m.openOverlay(themepicker.New(m.host())); return nil }},
		&command{id: "settings.config_edit", macro: macroNever, title: "Open config file",
			desc:    "Edit the settings file in $VISUAL or $EDITOR; 012 reloads it when you close the editor",
			enabled: func(m *Model) bool { return m.prefs.Config != nil && m.prefs.Config.Path != "" },
			run:     (*Model).editConfig},
		&command{id: "settings.config_reload", macro: macroNever, title: "Reload config",
			desc:    "Read the settings file again and apply the theme and display options",
			enabled: func(m *Model) bool { return m.prefs.Reload != nil },
			run:     func(m *Model) tea.Cmd { m.reloadConfig(); return nil }},
		&command{id: "settings.jev_key", macro: macroNever, title: "JEV API key",
			desc:    "Store the TypeSafe API key for JEV functions in the OS credential store",
			enabled: func(m *Model) bool { return m.prefs.Keys != nil },
			run:     (*Model).askKey},
	)
}

// configEditedMsg says the editor opened by Open config file exited.
type configEditedMsg struct{ err error }

func (m *Model) editConfig() tea.Cmd {
	path := m.prefs.Config.Path
	if err := config.EnsureFile(path); err != nil {
		m.fail("Can't create " + path + ": " + err.Error())
		return nil
	}
	cmd, err := config.Editor(path, os.Getenv)
	if err != nil {
		m.fail("Can't start the editor: " + err.Error())
		return nil
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return configEditedMsg{err} })
}

// reloadConfig reads the config file again, applies what can change
// live, and says what happened.
func (m *Model) reloadConfig() {
	c := m.prefs.Reload()
	m.prefs.Config = c
	var problems []string
	for _, w := range c.Warnings {
		problems = append(problems, w.Short())
	}
	if p := m.applyConfig(); p != "" {
		problems = append(problems, p)
	}
	if len(problems) > 0 {
		m.note = m.th.Warning.Render("Config reloaded with problems: " + strings.Join(problems, "; "))
		return
	}
	m.note = "Config reloaded from " + config.Tilde(c.Path)
}

// keySavedMsg reports storing the API key, the client made with it, and
// how the test call with it went.
type keySavedMsg struct {
	client jev.Client
	err    error  // storing it or connecting failed
	check  string // why the test call failed, "" when it worked
}

// keyCheckTimeout bounds the test call made with a new key.
const keyCheckTimeout = 15 * time.Second

// askKey asks for the API key on the context line, masked, stores it,
// and checks it with one small test call (jev.Check).
func (m *Model) askKey() tea.Cmd {
	m.openPrompt(&prompt{kind: promptText, label: "TypeSafe API key:", indicator: "KEY", secret: true,
		onText: func(key string) tea.Cmd {
			if key == "" {
				m.note = "No key entered; nothing changed"
				return nil
			}
			m.note = "Saving the key in the " + m.prefs.Keys.Name() + " and checking it…"
			return saveKeyCmd(m.prefs.Keys, m.prefs.Connect, key)
		}}, "")
	return nil
}

// saveKeyCmd stores key, connects with it and makes the test call.
func saveKeyCmd(store keyring.Store, connect func(string) (jev.Client, error), key string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := store.Set(ctx, key); err != nil {
			return keySavedMsg{err: err}
		}
		if connect == nil {
			return keySavedMsg{err: errors.New("JEV isn't available in this session")}
		}
		client, err := connect(key)
		if err != nil {
			return keySavedMsg{err: err}
		}
		cctx, ccancel := context.WithTimeout(context.Background(), keyCheckTimeout)
		defer ccancel()
		msg := keySavedMsg{client: client}
		if err := jev.Check(cctx, client); err != nil {
			// Whatever the service said, it never gets to show the key.
			msg.check = strings.ReplaceAll(err.Error(), key, "…")
		}
		return msg
	}
}

// keySaved turns JEV on with the new key and says how its check went.
// A key whose check failed is kept and used: a check can fail because
// the service can't be reached, not the key.
func (m *Model) keySaved(msg keySavedMsg) {
	if msg.err != nil {
		m.fail("Couldn't save the API key: " + msg.err.Error())
		return
	}
	if m.jev == nil {
		m.EnableJEV(msg.client, jev.NewCache())
	} else {
		m.jev.client = msg.client
	}
	m.sheet.RecalcVolatile()
	if msg.check != "" {
		m.note = m.th.Warning.Render("Key saved, but the check failed: " + msg.check)
		return
	}
	m.note = fmt.Sprintf("Key saved and checked; it's in the %s and JEV functions are on", m.prefs.Keys.Name())
}

// handlePrefs handles the messages settings get: the terminal's
// background (which picks the theme), the editor closing, the key saved.
func (m *Model) handlePrefs(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.prefs.light = !msg.IsDark()
		m.term.bg = msg.Color
		if p := m.applyTheme(); p != "" && m.note == "" {
			m.note = m.th.Warning.Render(p)
		}
	case configEditedMsg:
		if msg.err != nil {
			m.fail("The editor failed: " + msg.err.Error())
		} else if m.prefs.Reload != nil {
			m.reloadConfig()
		}
	case keySavedMsg:
		m.keySaved(msg)
	default:
		return false
	}
	return true
}
