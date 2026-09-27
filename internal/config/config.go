package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SourceKind says where a value came from.
type SourceKind int

const (
	FromDefault SourceKind = iota
	FromFile
	FromEnv
	FromFlag
)

// Source is where a value came from: the default, a line of a config
// file, an environment variable or a flag.
type Source struct {
	Kind SourceKind
	Name string // the file, variable or flag
	Line int    // for files
}

func (s Source) String() string {
	switch s.Kind {
	case FromFile:
		return fmt.Sprintf("%s:%d", s.Name, s.Line)
	case FromEnv:
		return "$" + s.Name
	case FromFlag:
		return s.Name
	}
	return "default"
}

// Value is one setting's value and where it came from.
type Value struct {
	Raw string
	Src Source
}

// Warning is a problem with the config that didn't stop 012: an unknown
// key, a bad value, a missing include. The option keeps its default.
type Warning struct {
	Src Source
	Msg string
}

func (w Warning) String() string {
	if w.Src.Kind == FromDefault {
		return w.Msg
	}
	return w.Src.String() + ": " + w.Msg
}

// Config is the effective configuration.
type Config struct {
	// Path is the main config file, which may not exist.
	Path     string
	Warnings []Warning
	vals     map[string][]Value // file values, then env, then flags override
}

// Dir is 012's config directory: $XDG_CONFIG_HOME/012 when that's set
// (on every OS, so tests and dotfile setups can point it anywhere),
// otherwise os.UserConfigDir()/012 (~/.config/012 on Linux,
// ~/Library/Application Support/012 on macOS, %AppData%\012 on Windows).
func Dir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" && filepath.IsAbs(x) {
		return filepath.Join(x, "012"), nil
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "012"), nil
}

// DefaultPath is the main config file, <Dir>/config.
func DefaultPath() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config"), nil
}

// ThemesDir is where theme files go, <Dir>/themes.
func ThemesDir() string {
	d, err := Dir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "themes")
}

// Load reads the config file at path (a missing file is fine), then
// applies environment variables (from getenv) and flags, which win in
// that order: flags > environment > config file > defaults.
func Load(path string, getenv func(string) string, flags map[string]string) *Config {
	c := &Config{Path: path, vals: map[string][]Value{}}
	if path != "" {
		c.readFile(path, false, map[string]bool{})
	}
	if getenv != nil {
		for i := range Options {
			o := &Options[i]
			for _, env := range o.Env {
				if v := strings.TrimSpace(getenv(env)); v != "" {
					c.set(o, v, Source{Kind: FromEnv, Name: env})
					break
				}
			}
		}
	}
	for name, v := range flags {
		if o, ok := byName[name]; ok {
			c.set(o, v, Source{Kind: FromFlag, Name: o.Flag})
		}
	}
	return c
}

// readFile reads one file and then the files it includes. seen holds the
// files being read, to stop include cycles.
func (c *Config) readFile(path string, optional bool, seen map[string]bool) {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	if seen[path] {
		c.warn(Source{}, "config-file "+path+" includes itself; skipped")
		return
	}
	f, err := os.Open(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if !optional && len(seen) > 0 {
			c.warn(Source{}, "config-file "+path+" doesn't exist")
		}
		return
	case err != nil:
		c.warn(Source{}, err.Error())
		return
	}
	defer f.Close()
	seen[path] = true
	defer delete(seen, path)
	// Includes are read after the whole file, as in Ghostty, so they
	// override what comes before and after them.
	for _, inc := range c.parse(f, path) {
		c.readFile(inc.path, inc.optional, seen)
	}
}

type include struct {
	path     string
	optional bool
}

// parse applies a file's lines and returns its includes.
func (c *Config) parse(r io.Reader, path string) []include {
	var incs []include
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		src := Source{Kind: FromFile, Name: path, Line: n}
		key, val, ok := splitLine(sc.Text())
		if !ok {
			if strings.TrimSpace(sc.Text()) != "" && !strings.HasPrefix(strings.TrimSpace(sc.Text()), "#") {
				c.warn(src, fmt.Sprintf("expected key = value, got %q", strings.TrimSpace(sc.Text())))
			}
			continue
		}
		o, known := byName[key]
		if !known {
			c.warn(src, "unknown key "+strconv.Quote(key))
			continue
		}
		if o.Name == "config-file" && val != "" {
			inc := include{path: val}
			if strings.HasPrefix(val, "?") {
				inc = include{path: strings.TrimPrefix(val, "?"), optional: true}
			}
			inc.path = expandPath(inc.path, filepath.Dir(path))
			incs = append(incs, inc)
		}
		c.set(o, val, src)
	}
	return incs
}

// splitLine splits "key = value", ignoring blank lines and # comments. A
// value may be quoted to keep spaces at its ends or a #.
func splitLine(line string) (key, val string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	key, val, ok = strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key, val = strings.TrimSpace(key), strings.TrimSpace(val)
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		if u, err := strconv.Unquote(val); err == nil {
			val = u
		}
	}
	return key, val, key != ""
}

// set records a value after validating it. An empty value resets the
// option (for repeatable ones, clears the list).
func (c *Config) set(o *Option, v string, src Source) {
	if err := o.validate(v); err != nil {
		c.warn(src, o.Name+": "+err.Error())
		return
	}
	switch {
	case v == "":
		delete(c.vals, o.Name)
	case o.Repeat && src.Kind == FromFile:
		c.vals[o.Name] = append(c.vals[o.Name], Value{Raw: v, Src: src})
	default:
		c.vals[o.Name] = []Value{{Raw: v, Src: src}}
	}
}

// Override sets an option for the rest of the session, over every other
// source, e.g. a theme picked in Settings. why says who set it. An
// invalid value is ignored and returned as an error.
func (c *Config) Override(name, value, why string) error {
	o := mustLookup(name)
	if err := o.validate(value); err != nil {
		return err
	}
	c.vals[name] = []Value{{Raw: value, Src: Source{Kind: FromFlag, Name: why}}}
	return nil
}

func (c *Config) warn(src Source, msg string) {
	c.Warnings = append(c.Warnings, Warning{Src: src, Msg: msg})
}

// Get returns an option's value and where it came from.
func (c *Config) Get(name string) Value {
	o := mustLookup(name)
	if c != nil {
		if vs := c.vals[name]; len(vs) > 0 {
			return vs[len(vs)-1]
		}
	}
	return Value{Raw: o.Default}
}

// String returns an option's value, with ~ expanded for paths.
func (c *Config) String(name string) string {
	v := c.Get(name)
	if byName[name].Kind == Path && v.Raw != "" {
		return expandPath(v.Raw, "")
	}
	return v.Raw
}

// Bool returns a true-or-false option.
func (c *Config) Bool(name string) bool {
	b, _ := strconv.ParseBool(c.Get(name).Raw)
	return b
}

// Int returns a number option.
func (c *Config) Int(name string) int {
	n, _ := strconv.Atoi(c.Get(name).Raw)
	return n
}

// Duration returns a duration option.
func (c *Config) Duration(name string) time.Duration {
	d, _ := time.ParseDuration(c.Get(name).Raw)
	return d
}

// List returns every value of a repeatable option.
func (c *Config) List(name string) []Value {
	mustLookup(name)
	if c == nil {
		return nil
	}
	return c.vals[name]
}

// Theme returns the theme choice.
func (c *Config) Theme() ThemeChoice {
	t, _ := ParseThemeChoice(c.Get("theme").Raw)
	return t
}

func mustLookup(name string) *Option {
	o, ok := byName[name]
	if !ok {
		panic("config: no option " + name)
	}
	return o
}

// expandPath expands a leading ~ and makes a relative path relative to
// dir, when dir is given.
func expandPath(p, dir string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	if dir != "" && !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return p
}
