// Package config is 012's settings file: a Ghostty-style list of
// "key = value" lines in <user config dir>/012/config, with # comments and
// config-file includes. One registry (Options) says what every option is
// called, its type, default, environment variable, flag and meaning; the
// parser, `012 config`, docs/reference/config.md and the in-app Settings all come
// from it.
//
// Settings are process-wide. Settings that belong to a workbook (decimal
// arithmetic, its locale) stay in the workbook's file; the locale option is
// only the default. Secrets never go here: the JEV
// API key lives in the OS credential store (see internal/keyring).
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/FelineStateMachine/012/internal/locale"
)

// Kind is an option's type.
type Kind int

const (
	String Kind = iota
	Bool
	Enum // one of Option.Values
	Path // a file path; ~ is the home directory
	URL
	Command  // a program and its arguments, run without a shell
	Int      // a whole number, at least 1
	Duration // e.g. 30m or 1h30m; 0 for never
	Address  // host:port
)

func (k Kind) String() string {
	return [...]string{"text", "true or false", "one of", "path", "URL", "command", "number", "duration", "host:port"}[k]
}

// Option is one setting.
type Option struct {
	Name    string
	Kind    Kind
	Group   string   // heading in docs/reference/config.md and `012 config`
	Default string   // as it would be written in the file
	Values  []string // for Enum
	Env     []string // environment variables that set it, first set wins
	// Fallback are environment variables read, first set wins, when
	// nothing else sets the option, through Adopt, which turns their
	// value into one of the option's or "" to pass over it quietly (LANG
	// may name a locale 012 lacks).
	Fallback []string
	Adopt    func(string) string
	Flag     string // command-line flag, e.g. "--log"
	Repeat   bool   // may be given more than once; each adds a value
	Live     bool   // Reload config applies it without restarting
	Desc     string // what it does, one or more sentences
	// Check validates a value beyond its kind. Invalid values are
	// reported as warnings and the option keeps its previous value.
	Check func(string) error
	// Redact shows a value safely in `012 config`, e.g. without a URL's
	// password.
	Redact func(string) string
}

// Group names, in the order docs list them.
const (
	GroupAppearance = "Appearance"
	GroupJEV        = "JEV functions"
	GroupTelemetry  = "Telemetry"
	GroupServe      = "012 serve"
	GroupData       = "Data"
	GroupFiles      = "Config files"
)

// Groups is the order groups are listed in.
var Groups = []string{GroupAppearance, GroupData, GroupJEV, GroupTelemetry, GroupServe, GroupFiles}

// Options is every setting. Add an option here and read it with
// Config.String, Bool or List; parsing, `012 config`, docs/reference/config.md
// (go test ./internal/config -update-docs) and Settings follow.
var Options = []Option{
	{Name: "theme", Group: GroupAppearance, Default: "terminal", Env: []string{"O12_THEME"}, Flag: "--theme", Live: true,
		Desc: "Colors. `terminal` uses the terminal's own 16-color palette. `high-contrast` draws white on " +
			"black or black on white by the terminal's background, with WCAG AAA contrast. Any other name is a " +
			"terminal color scheme, built in (`012 config themes` lists them) or a file in the themes " +
			"directory, drawn in its own colors with solid menu and status bars. " +
			"`light:NAME,dark:NAME` picks one by the terminal's background and follows it when it changes.",
		Check: checkTheme},
	{Name: "chart-images", Kind: Bool, Group: GroupAppearance, Default: "true", Env: []string{"O12_CHART_IMAGES"}, Live: true,
		Desc: "Draw charts as images on terminals with kitty graphics (kitty, Ghostty, WezTerm). " +
			"When false, charts are always text."},
	{Name: "notifications", Kind: Bool, Group: GroupAppearance, Default: "true", Env: []string{"O12_NOTIFICATIONS"}, Live: true,
		Desc: "Send a desktop notification (OSC 9) when JEV answers or an import finishes while the " +
			"terminal window is in the background."},
	{Name: "keymap", Kind: Enum, Group: GroupAppearance, Default: "default", Values: []string{"default", "vim"}, Env: []string{"O12_KEYMAP"}, Live: true,
		Desc: "Keys in the grid. `default` works like Google Sheets; `vim` adds hjkl, counts, operators, " +
			"visual selection and a : command line (File > Settings > Vim keys)."},

	{Name: "locale", Kind: Enum, Group: GroupData, Default: "en-US", Values: locale.Tags(), Env: []string{"O12_LOCALE"},
		Fallback: []string{"LC_ALL", "LC_NUMERIC", "LANG"}, Adopt: locale.FromPOSIX, Live: true,
		Desc: "How new sheets and files without a locale of their own are typed and shown: decimal and " +
			"thousands separators, date order, the currency symbol and the formula argument separator " +
			"(`;` where the decimal separator is a comma), as in Sheets' File > Settings > Locale, " +
			"which sets a file's own. Files store the same thing in every locale. " +
			"When unset, the POSIX locale (`LC_ALL`, `LC_NUMERIC`, then `LANG`, e.g. `de_DE.UTF-8`) picks it if it's one of these."},

	{Name: "max-cells", Kind: Int, Group: GroupData, Default: "10000000", Env: []string{"O12_MAX_CELLS"}, Live: true,
		Desc: "The most cells an import keeps, and a paste or fill writes at once. Numbers and text " +
			"take 20 to 60 bytes a cell and formulas about 750, so the default of ten million cells of " +
			"data is 200 to 600 MB. Imports keep whole rows " +
			"up to the budget and say how many they left out; larger pastes and fills are refused. " +
			"The grid itself is 1,048,576 rows by 16,384 columns (A to XFD) whatever this is."},

	{Name: "jev-api-key-command", Kind: Command, Group: GroupJEV,
		Desc: "A command that prints the TypeSafe API key, used when TYPESAFE_API_KEY isn't set and the " +
			"credential store has no key, e.g. `op read op://Private/TypeSafe/credential` or " +
			"`pass show typesafe`. It runs the first time a sheet asks JEV something, without a shell, for up to 10 seconds, and its first line of output is the key; for pipes, write " +
			"`sh -c '...'` yourself. The key itself never goes in this file."},
	{Name: "jev-credential-store", Kind: Bool, Group: GroupJEV, Default: "true", Env: []string{"O12_JEV_CREDENTIAL_STORE"},
		Desc: "Look for the API key in the OS credential store (macOS Keychain, Windows Credential Manager, " +
			"the Secret Service on Linux), where `012 config set-key` and Settings put it."},
	{Name: "jev-base-url", Kind: URL, Group: GroupJEV, Env: []string{"TYPESAFE_BASE_URL"},
		Desc:  "The TypeSafe service to ask; the SDK's default when empty. Must be https, except on localhost.",
		Check: CheckBaseURL, Redact: redactURL},
	{Name: "jev-model", Group: GroupJEV, Env: []string{"TYPESAFE_DEFAULT_MODEL"},
		Desc: "The JEV model to ask; the service's default when empty."},

	{Name: "log-file", Kind: Path, Group: GroupTelemetry, Env: []string{"O12_LOG"}, Flag: "--log",
		Desc: "Append telemetry events to this JSON log file. See docs/contributing/observability.md."},
	{Name: "log-level", Kind: Enum, Group: GroupTelemetry, Default: "info", Values: []string{"debug", "info", "warn", "error"},
		Env:  []string{"O12_LOG_LEVEL"},
		Desc: "The least severe telemetry event recorded; debug adds every frame and command."},
	{Name: "otlp-endpoint", Kind: URL, Group: GroupTelemetry, Env: []string{"OTEL_EXPORTER_OTLP_ENDPOINT"}, Flag: "--otlp",
		Desc: "Send telemetry to this OTLP/HTTP collector, e.g. http://localhost:4318. " +
			"The other OTEL_* variables still apply.",
		Redact: redactURL},

	{Name: "serve-listen", Kind: Address, Group: GroupServe, Default: "127.0.0.1:2312",
		Desc: "The address 012 serve listens on. Anything but the loopback address lets other machines " +
			"reach it (with an authorized key). See docs/terminal/ssh.md."},
	{Name: "serve-authorized-keys", Kind: Path, Group: GroupServe, Default: "~/.ssh/authorized_keys",
		Desc: "The public keys allowed to log in to 012 serve, in OpenSSH's authorized_keys format."},
	{Name: "serve-host-key", Kind: Path, Group: GroupServe,
		Desc: "012 serve's private host key, generated when missing; " +
			"ssh_host_ed25519_key in the config directory when empty."},
	{Name: "serve-idle-timeout", Kind: Duration, Group: GroupServe, Default: "30m",
		Desc: "End a 012 serve session that has had no input for this long; 0 never does."},
	{Name: "serve-max-sessions", Kind: Int, Group: GroupServe, Default: "8",
		Desc: "How many 012 serve sessions may run at once; more are turned away."},

	{Name: "config-file", Kind: Path, Group: GroupFiles, Repeat: true,
		Desc: "Read another config file after this one, relative to this file's directory. " +
			"A leading `?` makes it optional: no warning when it doesn't exist."},
}

// byName indexes Options.
var byName = func() map[string]*Option {
	m := make(map[string]*Option, len(Options))
	for i := range Options {
		m[Options[i].Name] = &Options[i]
	}
	return m
}()

// Lookup returns the option called name.
func Lookup(name string) (*Option, bool) {
	o, ok := byName[name]
	return o, ok
}

// validate checks a value against the option's kind and Check.
func (o *Option) validate(v string) error {
	if v == "" {
		return nil // empty resets to the default
	}
	switch o.Kind {
	case Bool:
		if _, err := strconv.ParseBool(v); err != nil {
			return fmt.Errorf("%q isn't true or false", v)
		}
	case Enum:
		if !contains(o.Values, v) {
			return fmt.Errorf("%q isn't one of %s", v, strings.Join(o.Values, ", "))
		}
	case URL:
		withScheme := v
		if !strings.Contains(v, "://") {
			withScheme = "http://" + v // host:port, as OTEL_EXPORTER_OTLP_ENDPOINT allows
		}
		u, err := url.Parse(withScheme)
		if err != nil || u.Host == "" {
			return fmt.Errorf("%q isn't a URL", v)
		}
	case Command:
		if _, err := SplitCommand(v); err != nil {
			return err
		}
	case Int:
		if n, err := strconv.Atoi(v); err != nil || n < 1 {
			return fmt.Errorf("%q isn't a whole number of at least 1", v)
		}
	case Duration:
		if d, err := time.ParseDuration(v); err != nil || d < 0 {
			return fmt.Errorf("%q isn't a duration like 30m or 1h", v)
		}
	case Address:
		if _, _, err := net.SplitHostPort(v); err != nil {
			return fmt.Errorf("%q isn't host:port", v)
		}
	}
	if o.Check != nil {
		return o.Check(v)
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// CheckBaseURL accepts https URLs, and http only for this machine, so a
// config can't send the API key anywhere in the clear.
func CheckBaseURL(v string) error {
	u, err := url.Parse(v)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%q isn't a URL", v)
	}
	switch {
	case u.Scheme == "https":
		return nil
	case u.Scheme == "http" && IsLocalhost(u.Hostname()):
		return nil
	}
	return errors.New("the JEV base URL must be https (http only for localhost)")
}

// IsLocalhost reports whether host names this machine.
func IsLocalhost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// checkTheme checks the shape of a theme value: a name, or light: and
// dark: names. Whether the names exist is known only when it's loaded.
func checkTheme(v string) error {
	_, err := ParseThemeChoice(v)
	return err
}

// ThemeChoice is the theme value: one theme, or one for each kind of
// terminal background.
type ThemeChoice struct{ Light, Dark string }

// Pick returns the theme for a dark or light terminal.
func (c ThemeChoice) Pick(dark bool) string {
	if dark {
		return c.Dark
	}
	return c.Light
}

// ParseThemeChoice parses "name" or "light:A,dark:B" (either order).
func ParseThemeChoice(v string) (ThemeChoice, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		v = "terminal"
	}
	if !strings.Contains(v, "light:") && !strings.Contains(v, "dark:") {
		return ThemeChoice{Light: v, Dark: v}, nil
	}
	var c ThemeChoice
	for part := range strings.SplitSeq(v, ",") {
		k, name, ok := strings.Cut(strings.TrimSpace(part), ":")
		name = strings.TrimSpace(name)
		switch {
		case !ok || name == "":
			return c, fmt.Errorf("theme %q: expected light:NAME,dark:NAME", v)
		case k == "light":
			c.Light = name
		case k == "dark":
			c.Dark = name
		default:
			return c, fmt.Errorf("theme %q: expected light:NAME,dark:NAME", v)
		}
	}
	if c.Light == "" || c.Dark == "" {
		return c, fmt.Errorf("theme %q: give both light: and dark:", v)
	}
	return c, nil
}

// String is the choice as written in the file.
func (c ThemeChoice) String() string {
	if c.Light == c.Dark {
		return c.Light
	}
	return "light:" + c.Light + ",dark:" + c.Dark
}

// redactURL hides a URL's password and query, which can carry tokens.
func redactURL(v string) string {
	u, err := url.Parse(v)
	if err != nil {
		return "(hidden)"
	}
	if u.User != nil {
		u.User = url.User("xxxxx")
	}
	if u.RawQuery != "" {
		u.RawQuery = "xxxxx"
	}
	return u.String()
}

// SplitCommand splits a command line into words, as a shell would
// without expanding anything: spaces separate words, and single or
// double quotes keep spaces inside one (a backslash escapes the next
// character outside single quotes). No variables, globs or pipes.
func SplitCommand(s string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inWord := false
	var quote rune
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped, inWord = true, true
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			cur.WriteRune(r)
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("command %q: unclosed quote", s)
	}
	if inWord {
		words = append(words, cur.String())
	}
	if len(words) == 0 {
		return nil, errors.New("empty command")
	}
	return words, nil
}
