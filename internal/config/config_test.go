package config

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func write(t *testing.T, path, text string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func noEnv(string) string { return "" }

func warnings(c *Config) string {
	var out []string
	for _, w := range c.Warnings {
		out = append(out, w.String())
	}
	return strings.Join(out, "\n")
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	path := write(t, filepath.Join(dir, "config"), `# a comment
theme = Dracula
  # indented comment

chart-images=false
jev-model = "  spaced  "
bogus = 1
not a setting
log-level = loud
`)
	c := Load(path, noEnv, nil)
	if got := c.String("theme"); got != "Dracula" {
		t.Errorf("theme %q", got)
	}
	if c.Bool("chart-images") {
		t.Error("chart-images should be false")
	}
	if got := c.String("jev-model"); got != "  spaced  " {
		t.Errorf("quoted value %q", got)
	}
	if v := c.Get("log-level"); v.Raw != "info" || v.Src.Kind != FromDefault {
		t.Errorf("an invalid value keeps the default: %+v", v)
	}
	w := warnings(c)
	for _, want := range []string{`config:7: unknown key "bogus"`, `config:8: expected key = value`, `config:9: log-level: "loud" isn't one of`} {
		if !strings.Contains(w, want) {
			t.Errorf("warnings lack %q:\n%s", want, w)
		}
	}
	if v := c.Get("theme"); v.Src.Kind != FromFile || v.Src.Line != 2 {
		t.Errorf("theme source %+v", v.Src)
	}
}

func TestMissingFileIsFine(t *testing.T) {
	c := Load(filepath.Join(t.TempDir(), "config"), noEnv, nil)
	if len(c.Warnings) != 0 || c.String("theme") != "terminal" || !c.Bool("chart-images") {
		t.Errorf("defaults: %v %q", c.Warnings, c.String("theme"))
	}
}

func TestIncludes(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "themes.conf"), "theme = Nord\n")
	write(t, filepath.Join(dir, "sub", "more"), "jev-model = included\nconfig-file = ../config\n")
	path := write(t, filepath.Join(dir, "config"), `config-file = themes.conf
theme = before-include-wins-not
config-file = ?optional-missing
config-file = required-missing
config-file = sub/more
`)
	c := Load(path, noEnv, nil)
	if got := c.String("theme"); got != "Nord" {
		t.Errorf("includes are read after the file, so they win: %q", got)
	}
	if got := c.String("jev-model"); got != "included" {
		t.Errorf("nested include: %q", got)
	}
	w := warnings(c)
	if strings.Contains(w, "optional-missing") {
		t.Errorf("an optional include warned:\n%s", w)
	}
	if !strings.Contains(w, "required-missing doesn't exist") {
		t.Errorf("no warning for a missing include:\n%s", w)
	}
	if !strings.Contains(w, "includes itself") {
		t.Errorf("no warning for the cycle:\n%s", w)
	}
	if n := len(c.List("config-file")); n != 5 {
		t.Errorf("config-file is repeatable: %d values", n)
	}
}

func TestSelfInclude(t *testing.T) {
	dir := t.TempDir()
	path := write(t, filepath.Join(dir, "config"), "config-file = config\ntheme = x\n")
	c := Load(path, noEnv, nil)
	if c.String("theme") != "x" || !strings.Contains(warnings(c), "includes itself") {
		t.Errorf("self include: %q %v", c.String("theme"), c.Warnings)
	}
}

func TestPrecedence(t *testing.T) {
	path := write(t, filepath.Join(t.TempDir(), "config"), "log-file = from-file.jsonl\njev-model = file-model\ntheme = Nord\n")
	env := map[string]string{"O12_LOG": "from-env.jsonl", "TYPESAFE_DEFAULT_MODEL": "env-model"}
	c := Load(path, func(k string) string { return env[k] }, map[string]string{"log-file": "from-flag.jsonl"})
	if got := c.Get("log-file"); got.Raw != "from-flag.jsonl" || got.Src.String() != "--log" {
		t.Errorf("flags win: %+v", got)
	}
	if got := c.Get("jev-model"); got.Raw != "env-model" || got.Src.String() != "$TYPESAFE_DEFAULT_MODEL" {
		t.Errorf("environment beats the file: %+v", got)
	}
	if got := c.Get("theme"); got.Raw != "Nord" {
		t.Errorf("file beats the default: %+v", got)
	}
	if got := c.Get("log-level"); got.Raw != "info" || got.Src.String() != "default" {
		t.Errorf("default: %+v", got)
	}
}

func TestEmptyValueResets(t *testing.T) {
	path := write(t, filepath.Join(t.TempDir(), "config"), "theme = Nord\ntheme =\n")
	if got := Load(path, noEnv, nil).String("theme"); got != "terminal" {
		t.Errorf("empty value should reset: %q", got)
	}
}

func TestParseFlags(t *testing.T) {
	flags, rest, err := ParseFlags([]string{"--log", "a.jsonl", "--otlp=http://localhost:4318", "b.csv", "--theme", "Nord"})
	if err != nil || flags["log-file"] != "a.jsonl" || flags["otlp-endpoint"] != "http://localhost:4318" ||
		flags["theme"] != "Nord" || !slices.Equal(rest, []string{"b.csv"}) {
		t.Errorf("got %v %v %v", flags, rest, err)
	}
	if _, _, err := ParseFlags([]string{"--otlp"}); err == nil {
		t.Error("--otlp without a value")
	}
}

func TestBaseURL(t *testing.T) {
	for v, ok := range map[string]bool{
		"https://api.example.com":   true,
		"http://127.0.0.1:8799":     true,
		"http://localhost:1234/v1":  true,
		"http://[::1]:80":           true,
		"http://api.example.com":    false,
		"http://127.evil.com":       false,
		"ftp://localhost":           false,
		"not a url":                 false,
		"https://":                  false,
		"http://localhost.evil.com": false,
	} {
		if err := CheckBaseURL(v); (err == nil) != ok {
			t.Errorf("%s: %v", v, err)
		}
	}
	path := write(t, filepath.Join(t.TempDir(), "config"), "jev-base-url = http://attacker.example\n")
	c := Load(path, noEnv, nil)
	if c.String("jev-base-url") != "" || !strings.Contains(warnings(c), "must be https") {
		t.Errorf("an http base URL was accepted: %q %v", c.String("jev-base-url"), c.Warnings)
	}
	env := func(k string) string {
		if k == "TYPESAFE_BASE_URL" {
			return "http://attacker.example"
		}
		return ""
	}
	if c := Load("", env, nil); c.String("jev-base-url") != "" {
		t.Error("an http base URL from the environment was accepted")
	}
}

func TestThemeChoice(t *testing.T) {
	for v, want := range map[string]ThemeChoice{
		"":                           {"terminal", "terminal"},
		"Nord":                       {"Nord", "Nord"},
		"light:Nord Light,dark:Nord": {"Nord Light", "Nord"},
		"dark: B , light: A":         {"A", "B"},
	} {
		got, err := ParseThemeChoice(v)
		if err != nil || got != want {
			t.Errorf("%q: %+v %v", v, got, err)
		}
	}
	for _, bad := range []string{"light:A", "light:A,dim:B", "light:,dark:B"} {
		if _, err := ParseThemeChoice(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSplitCommand(t *testing.T) {
	for in, want := range map[string][]string{
		`op read op://Private/TypeSafe/credential`: {"op", "read", "op://Private/TypeSafe/credential"},
		`pass show "api keys/typesafe"`:            {"pass", "show", "api keys/typesafe"},
		`sh -c 'pass show x | head -1'`:            {"sh", "-c", "pass show x | head -1"},
		`echo a\ b $HOME`:                          {"echo", "a b", "$HOME"},
		`printf ''`:                                {"printf", ""},
	} {
		got, err := SplitCommand(in)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("%s: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{`echo 'open`, ``, `   `} {
		if _, err := SplitCommand(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestShowRedacts(t *testing.T) {
	path := write(t, filepath.Join(t.TempDir(), "config"), "otlp-endpoint = https://user:hunter2@collector.example/?token=abc\n")
	out := Load(path, noEnv, nil).Show()
	if strings.Contains(out, "hunter2") || strings.Contains(out, "token=abc") {
		t.Errorf("secret shown:\n%s", out)
	}
	if !strings.Contains(out, "otlp-endpoint = https://xxxxx@collector.example/?xxxxx") {
		t.Errorf("redacted URL missing:\n%s", out)
	}
}

func TestDefaultFileParses(t *testing.T) {
	path := write(t, filepath.Join(t.TempDir(), "config"), DefaultFile())
	c := Load(path, noEnv, nil)
	if len(c.Warnings) != 0 {
		t.Errorf("the default file warns: %v", c.Warnings)
	}
	for _, o := range Options {
		if !strings.Contains(DefaultFile(), "# "+o.Name+" = ") {
			t.Errorf("default file lacks %s", o.Name)
		}
	}
}

func TestSetInFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config")
	if err := SetInFile(path, "theme", "Nord"); err != nil {
		t.Fatal(err)
	}
	write(t, path, "# mine\ntheme = A\nchart-images = false\ntheme = B # later wins\n")
	if err := SetInFile(path, "theme", "Dracula"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "# mine\ntheme = A\nchart-images = false\ntheme = Dracula\n" {
		t.Errorf("got %q", data)
	}
	if err := SetInFile(path, "nope", "x"); err == nil {
		t.Error("unknown key accepted")
	}
}

func TestRegistry(t *testing.T) {
	seen := map[string]bool{}
	for _, o := range Options {
		if seen[o.Name] {
			t.Errorf("%s registered twice", o.Name)
		}
		seen[o.Name] = true
		if o.Desc == "" || !slices.Contains(Groups, o.Group) {
			t.Errorf("%s: needs a description and a known group", o.Name)
		}
		if err := o.validate(o.Default); err != nil {
			t.Errorf("%s: default %q invalid: %v", o.Name, o.Default, err)
		}
	}
}

func TestServeOptions(t *testing.T) {
	path := write(t, filepath.Join(t.TempDir(), "config"), "serve-max-sessions = 3\nserve-idle-timeout = 1h\nserve-listen = nope\nserve-max-sessions = 0\n")
	c := Load(path, noEnv, nil)
	if c.Int("serve-max-sessions") != 3 || c.Duration("serve-idle-timeout").Minutes() != 60 || c.String("serve-listen") != "127.0.0.1:2312" {
		t.Errorf("serve: %d %v %q", c.Int("serve-max-sessions"), c.Duration("serve-idle-timeout"), c.String("serve-listen"))
	}
	if w := warnings(c); !strings.Contains(w, "isn't host:port") || !strings.Contains(w, "at least 1") {
		t.Errorf("warnings:\n%s", w)
	}
	if home, _ := os.UserHomeDir(); c.String("serve-authorized-keys") != filepath.Join(home, ".ssh", "authorized_keys") {
		t.Errorf("~ not expanded: %q", c.String("serve-authorized-keys"))
	}
}

func TestDir(t *testing.T) {
	x := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", x)
	if d, _ := Dir(); d != filepath.Join(x, "012") {
		t.Errorf("XDG_CONFIG_HOME ignored: %s", d)
	}
	if p, _ := DefaultPath(); p != filepath.Join(x, "012", "config") {
		t.Errorf("path %s", p)
	}
}

var updateDocs = flag.Bool("update-docs", false, "rewrite the reference in docs/reference/config.md")

// TestConfigDoc keeps docs/reference/config.md's reference in step with the
// registry: run `go test ./internal/config -update-docs` after changing
// an option.
func TestConfigDoc(t *testing.T) {
	const path = "../../docs/reference/config.md"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	intro, _, ok := strings.Cut(string(data), DocsMarker)
	if !ok {
		t.Fatalf("%s lacks the generated-reference marker", path)
	}
	want := intro + DocsMarker + "\n" + Reference()
	if *updateDocs {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if string(data) != want {
		t.Errorf("%s is stale: run go test ./internal/config -update-docs", path)
	}
}

// TestLocaleFallsBackOnPOSIX reads the locale from LC_ALL, LC_NUMERIC or
// LANG only when nothing else sets it, and passes over POSIX locales it
// doesn't have without a warning.
func TestLocaleFallsBackOnPOSIX(t *testing.T) {
	dir := t.TempDir()
	set := write(t, filepath.Join(dir, "set"), "locale = fr-FR\n")
	unset := filepath.Join(dir, "none")
	cases := []struct {
		path string
		env  map[string]string
		want string
	}{
		{unset, map[string]string{"LANG": "de_DE.UTF-8"}, "de-DE"},
		{unset, map[string]string{"LANG": "de_DE.UTF-8", "LC_ALL": "pt_BR.UTF-8"}, "pt-BR"},
		{unset, map[string]string{"LANG": "C.UTF-8"}, "en-US"},
		{unset, map[string]string{"LANG": "xx_YY.UTF-8", "LC_NUMERIC": "sv_SE"}, "sv-SE"},
		{set, map[string]string{"LANG": "de_DE.UTF-8"}, "fr-FR"},
		{unset, map[string]string{"LANG": "de_DE.UTF-8", "O12_LOCALE": "ja-JP"}, "ja-JP"},
	}
	for _, c := range cases {
		cfg := Load(c.path, func(k string) string { return c.env[k] }, nil)
		if got := cfg.String("locale"); got != c.want || len(cfg.Warnings) > 0 {
			t.Errorf("%v: locale %q, want %q; warnings %s", c.env, got, c.want, warnings(cfg))
		}
	}
}
