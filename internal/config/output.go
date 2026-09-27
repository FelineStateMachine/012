package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ParseFlags takes the options that have flags ("--log path" or
// "--log=path") out of args, returning their values by option name and
// the arguments left.
func ParseFlags(args []string) (map[string]string, []string, error) {
	flags := map[string]string{}
	var rest []string
	for i := 0; i < len(args); i++ {
		name, value, hasValue := strings.Cut(args[i], "=")
		o := flagOption(name)
		switch {
		case o == nil:
			rest = append(rest, args[i])
		case hasValue:
			flags[o.Name] = value
		case i+1 < len(args):
			flags[o.Name] = args[i+1]
			i++
		default:
			return nil, nil, errors.New(name + " needs a value")
		}
	}
	return flags, rest, nil
}

func flagOption(flag string) *Option {
	if !strings.HasPrefix(flag, "--") {
		return nil
	}
	for i := range Options {
		if Options[i].Flag == flag {
			return &Options[i]
		}
	}
	return nil
}

// FlagUsage lists the flags for a usage line, e.g. "[--log file]".
func FlagUsage() string {
	var parts []string
	for _, o := range Options {
		if o.Flag != "" {
			parts = append(parts, "["+o.Flag+" "+strings.TrimPrefix(o.Flag, "--")+"]")
		}
	}
	return strings.Join(parts, " ")
}

// Show writes the effective configuration: every option with its value
// and where it came from, values that can hold secrets redacted.
func (c *Config) Show() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", c.Path)
	for _, g := range Groups {
		fmt.Fprintf(&b, "\n# %s\n", g)
		for _, o := range Options {
			if o.Group != g {
				continue
			}
			vals := c.List(o.Name)
			if !o.Repeat || len(vals) == 0 {
				vals = []Value{c.Get(o.Name)}
			}
			for _, v := range vals {
				raw := v.Raw
				if o.Redact != nil && raw != "" {
					raw = o.Redact(raw)
				}
				fmt.Fprintf(&b, "%s = %s", o.Name, raw)
				b.WriteString(strings.Repeat(" ", max(1, 40-len(o.Name)-len(raw)-3)))
				fmt.Fprintf(&b, "# %s\n", v.Src)
			}
		}
	}
	for _, w := range c.Warnings {
		fmt.Fprintf(&b, "\n# warning: %s", w)
	}
	if len(c.Warnings) > 0 {
		b.WriteByte('\n')
	}
	return b.String()
}

// DefaultFile is a config file with every option commented out at its
// default, each with its description: what `012 config default` prints
// and `012 config edit` starts a new file with.
func DefaultFile() string {
	var b strings.Builder
	b.WriteString("# 012 configuration. Lines are key = value; # starts a comment.\n")
	b.WriteString("# Flags and environment variables override this file.\n")
	b.WriteString("# See `012 config` for the values in effect and docs/config.md for more.\n")
	for _, g := range Groups {
		fmt.Fprintf(&b, "\n# ---- %s ----\n", g)
		for _, o := range Options {
			if o.Group != g {
				continue
			}
			b.WriteString("\n")
			for _, l := range wrap(stripTicks(o.Desc), 74) {
				b.WriteString("# " + l + "\n")
			}
			fmt.Fprintf(&b, "# %s = %s\n", o.Name, o.Default)
		}
	}
	return b.String()
}

func stripTicks(s string) string { return strings.ReplaceAll(s, "`", "") }

// wrap breaks s into lines of at most w columns at spaces.
func wrap(s string, w int) []string {
	var lines []string
	line := ""
	for word := range strings.FieldsSeq(s) {
		if line != "" && len(line)+1+len(word) > w {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// SetInFile sets key to value in the config file at path: the last line
// setting key is replaced, or a line is added at the end. Comments and
// every other line are kept. The file and its directory are created if
// needed.
func SetInFile(path, key, value string) error {
	if _, ok := byName[key]; !ok {
		return errors.New("no option " + key)
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(data) == 0 {
		lines = nil
	}
	newLine := key + " = " + value
	at := -1
	for i, l := range lines {
		if k, _, ok := splitLine(l); ok && k == key {
			at = i
		}
	}
	if at >= 0 {
		lines[at] = newLine
	} else {
		lines = append(lines, newLine)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Names is every option name, sorted.
func Names() []string {
	names := make([]string, len(Options))
	for i, o := range Options {
		names[i] = o.Name
	}
	slices.Sort(names)
	return names
}
