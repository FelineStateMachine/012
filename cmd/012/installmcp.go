package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
)

// 012 agent --install-mcp claude|codex|desktop: the entry that starts
// 012 mcp, written into the host's configuration (see
// docs/agents/mcp.md#adding-it-to-a-host). The entry runs this binary
// by its absolute path with no workbook, so one server works on every
// workbook the host's roots or --root open to it. A file edited is
// copied beside itself first; an entry already the same is left alone.

// mcpHost is a host whose configuration holds MCP servers.
type mcpHost struct {
	name string // as --install-mcp takes it
	app  string // as people call it
	path func(e env) (string, error)
	// entry is the configuration's text with 012's entry set, old being
	// what's there ("" for no file); same is set when it was already.
	entry func(old []byte, command string, args []string) (text []byte, same bool, err error)
	// show is the entry alone, for --print.
	show func(command string, args []string) string
}

var mcpHosts = []mcpHost{
	{name: "claude", app: "Claude Code", path: claudeConfig, entry: jsonEntry(true), show: jsonShow(true)},
	{name: "codex", app: "Codex", path: codexConfig, entry: tomlEntry, show: tomlShow},
	{name: "desktop", app: "Claude Desktop", path: desktopConfig, entry: jsonEntry(false), show: jsonShow(false)},
}

// executable is this binary's path; tests replace it.
var executable = os.Executable

// installMCP writes the entry for host, or prints it with print.
func installMCP(e env, host string, roots []string, print bool) error {
	var h *mcpHost
	for i := range mcpHosts {
		if mcpHosts[i].name == host {
			h = &mcpHosts[i]
		}
	}
	if h == nil {
		return usageError(fmt.Sprintf("--install-mcp takes claude, codex or desktop, not %q", host), agentUsage)
	}
	command, err := executable()
	if err != nil {
		return err
	}
	if command, err = filepath.Abs(command); err != nil {
		return err
	}
	args := []string{"mcp"}
	for _, r := range roots {
		abs, err := filepath.Abs(r)
		if err != nil {
			return err
		}
		args = append(args, "--root", abs)
	}
	path, err := h.path(e)
	if err != nil {
		return err
	}
	if print {
		_, err := fmt.Fprintf(e.stdout, "%s, in %s:\n\n%s", h.app, path, h.show(command, args))
		return err
	}
	return writeEntry(e, h, path, command, args)
}

// writeEntry sets 012's entry in the configuration at path, backing the
// file up first.
func writeEntry(e env, h *mcpHost, path, command string, args []string) error {
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	text, same, err := h.entry(old, command, args)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if same {
		fmt.Fprintf(e.stdout, "%s already runs %s mcp: %s is up to date\n", h.app, command, path)
		return nil
	}
	mode := fs.FileMode(0o600)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
		backup := path + ".bak-" + time.Now().Format("20060102-150405")
		if err := os.WriteFile(backup, old, mode); err != nil {
			return err
		}
		fmt.Fprintln(e.stdout, "Backed up", path, "to", backup)
	} else if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, text, mode); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "Wrote the 012 entry to %s: %s %s\n", path, command, strings.Join(args, " "))
	fmt.Fprintf(e.stdout, "Restart %s to start it; it works on the workbooks in the folders it's given, without naming one.\n", h.app)
	for _, other := range otherEntries(h, text, command) {
		fmt.Fprintf(e.stdout, "%s also starts 012 as %q; remove that entry if it was for one workbook.\n", path, other)
	}
	return nil
}

// claudeConfig is Claude Code's user configuration, where claude mcp add
// --scope user puts servers.
func claudeConfig(e env) (string, error) {
	if dir := e.getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json"), nil
	}
	home, err := os.UserHomeDir()
	return filepath.Join(home, ".claude.json"), err
}

// codexConfig is Codex's configuration, in CODEX_HOME or ~/.codex.
func codexConfig(e env) (string, error) {
	if dir := e.getenv("CODEX_HOME"); dir != "" {
		return filepath.Join(dir, "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	return filepath.Join(home, ".codex", "config.toml"), err
}

// desktopConfig is Claude Desktop's configuration, in the folder the
// system keeps applications' settings in.
func desktopConfig(env) (string, error) {
	dir, err := os.UserConfigDir()
	return filepath.Join(dir, "Claude", "claude_desktop_config.json"), err
}

// jsonServer is an entry of mcpServers; Claude Code's says its type.
func jsonServer(typed bool, command string, args []string) map[string]any {
	s := map[string]any{"command": command, "args": args}
	if typed {
		s["type"], s["env"] = "stdio", map[string]any{}
	}
	return s
}

func jsonShow(typed bool) func(string, []string) string {
	return func(command string, args []string) string {
		data, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"012": jsonServer(typed, command, args)}}, "", "  ")
		return string(data) + "\n"
	}
}

// jsonEntry sets mcpServers["012"] in a JSON configuration, keeping the
// rest of it as it was, keys in order.
func jsonEntry(typed bool) func([]byte, string, []string) ([]byte, bool, error) {
	return func(old []byte, command string, args []string) ([]byte, bool, error) {
		top := map[string]json.RawMessage{}
		if len(bytes.TrimSpace(old)) > 0 {
			if err := json.Unmarshal(old, &top); err != nil {
				return nil, false, fmt.Errorf("not JSON 012 can add to: %w", err)
			}
		}
		servers := map[string]json.RawMessage{}
		if raw, ok := top["mcpServers"]; ok {
			if err := json.Unmarshal(raw, &servers); err != nil {
				return nil, false, fmt.Errorf("mcpServers isn't an object: %w", err)
			}
		}
		entry, _ := json.Marshal(jsonServer(typed, command, args))
		if sameJSON(servers["012"], entry) {
			return old, true, nil
		}
		servers["012"] = entry
		top["mcpServers"], _ = json.Marshal(servers)
		text, err := json.MarshalIndent(top, "", "  ")
		return append(text, '\n'), false, err
	}
}

func sameJSON(a, b []byte) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}

// otherEntries are the names of the configuration's other servers that
// start this 012.
func otherEntries(h *mcpHost, text []byte, command string) []string {
	var out []string
	if h.name == "codex" {
		for _, m := range tomlTable.FindAllSubmatchIndex(text, -1) {
			name := string(text[m[2]:m[3]])
			if unquote(name) != "012" && bytes.Contains(text[m[1]:tomlTableEnd(text, m[1])], []byte(tomlString(command))) {
				out = append(out, name)
			}
		}
		return out
	}
	var top struct {
		MCPServers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	_ = json.Unmarshal(text, &top)
	for name, s := range top.MCPServers {
		if name != "012" && s.Command == command {
			out = append(out, name)
		}
	}
	return out
}

// tomlTable finds [mcp_servers.<name>] tables' headers.
var tomlTable = regexp.MustCompile(`(?m)^[ \t]*\[[ \t]*mcp_servers[ \t]*\.[ \t]*("[^"\n]*"|'[^'\n]*'|[A-Za-z0-9_-]+)[ \t]*\][ \t]*(?:#.*)?$`)

// tomlKey finds a command or args key's line in a table.
var tomlKey = regexp.MustCompile(`^[ \t]*(command|args)[ \t]*=`)

func unquote(name string) string { return strings.Trim(name, `"'`) }

// tomlTableEnd is where the table whose header ends at i ends: the next
// table's header, or the end.
func tomlTableEnd(text []byte, i int) int {
	if m := tomlHeader.FindIndex(text[i:]); m != nil {
		return i + m[0]
	}
	return len(text)
}

// tomlHeader finds any table's header; tomlInline012 a key 012 set
// inline in [mcp_servers].
var (
	tomlHeader    = regexp.MustCompile(`(?m)^[ \t]*\[`)
	tomlInline012 = regexp.MustCompile(`(?m)^[ \t]*("012"|'012'|012)[ \t]*=`)
)

// tomlString is s as a TOML basic string.
func tomlString(s string) string {
	data, _ := json.Marshal(s) // JSON's escapes are TOML's
	return string(data)
}

func tomlLines(command string, args []string) (string, string) {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = tomlString(a)
	}
	return "command = " + tomlString(command), "args = [" + strings.Join(quoted, ", ") + "]"
}

func tomlShow(command string, args []string) string {
	c, a := tomlLines(command, args)
	return "[mcp_servers.012]\n" + c + "\n" + a + "\n"
}

// tomlEntry sets command and args in Codex's [mcp_servers.012] table,
// keeping its other keys (env, timeouts) and everything else in the
// file as written, or adds the table at the end.
func tomlEntry(old []byte, command string, args []string) ([]byte, bool, error) {
	c, a := tomlLines(command, args)
	var header []int
	for _, m := range tomlTable.FindAllSubmatchIndex(old, -1) {
		if unquote(string(old[m[2]:m[3]])) == "012" {
			header = m
		}
	}
	if header == nil {
		if tomlInline012.Match(old) {
			return nil, false, errors.New("012 is set inline in [mcp_servers]; replace it with --print's table")
		}
		text := append([]byte{}, old...)
		if len(text) > 0 && !bytes.HasSuffix(text, []byte("\n")) {
			text = append(text, '\n')
		}
		if len(text) > 0 {
			text = append(text, '\n')
		}
		return append(text, tomlShow(command, args)...), false, nil
	}
	start, end := header[1]+1, tomlTableEnd(old, min(header[1]+1, len(old)))
	if start > len(old) {
		start = len(old)
	}
	body := strings.SplitAfter(string(old[start:end]), "\n")
	var kept []string
	for i := 0; i < len(body); i++ {
		m := tomlKey.FindStringSubmatch(body[i])
		if m == nil {
			kept = append(kept, body[i])
			continue
		}
		if m[1] == "args" && strings.Count(body[i], "[") > strings.Count(body[i], "]") {
			for i+1 < len(body) && !strings.Contains(body[i], "]") {
				i++ // an array written over several lines
			}
		}
	}
	newBody := c + "\n" + a + "\n" + strings.Join(kept, "")
	if newBody == string(old[start:end]) {
		return old, true, nil
	}
	text := string(old[:start]) + newBody + string(old[end:])
	return []byte(text), false, nil
}
