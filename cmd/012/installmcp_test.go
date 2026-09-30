package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mcpHome gives the test a home folder of its own and this 012 a path.
func mcpHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	old := executable
	executable = func() (string, error) { return "/opt/012/bin/012", nil }
	t.Cleanup(func() { executable = old })
	return home
}

func backups(t *testing.T, path string) int {
	t.Helper()
	found, _ := filepath.Glob(path + ".bak-*")
	return len(found)
}

func TestInstallMCPClaude(t *testing.T) {
	home := mcpHome(t)
	e, out, _ := scriptEnv(t)
	path := filepath.Join(home, ".claude.json")
	os.WriteFile(path, []byte(`{"numStartups": 3, "mcpServers": {"other": {"command": "x"}}, "projects": {"/p": {"allowedTools": []}}}`), 0o600)
	if code, err := status(e, "agent", "--install-mcp", "claude"); code != 0 {
		t.Fatal(err)
	}
	var got struct {
		NumStartups int                        `json:"numStartups"`
		Projects    map[string]any             `json:"projects"`
		MCPServers  map[string]json.RawMessage `json:"mcpServers"`
	}
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.NumStartups != 3 || got.Projects["/p"] == nil || got.MCPServers["other"] == nil ||
		!sameJSON(got.MCPServers["012"], []byte(`{"type":"stdio","command":"/opt/012/bin/012","args":["mcp"],"env":{}}`)) {
		t.Errorf("~/.claude.json: %s", data)
	}
	if backups(t, path) != 1 || !strings.Contains(out.String(), "Restart Claude Code") {
		t.Errorf("backups %d, said %q", backups(t, path), out)
	}
	out.Reset()
	if status(e, "agent", "--install-mcp", "claude"); !strings.Contains(out.String(), "is up to date") || backups(t, path) != 1 {
		t.Errorf("again: %q, %d backups", out, backups(t, path))
	}
}

func TestInstallMCPCodex(t *testing.T) {
	home := mcpHome(t)
	e, out, _ := scriptEnv(t)
	path := filepath.Join(home, ".codex", "config.toml")
	os.MkdirAll(filepath.Dir(path), 0o755)
	old := `model = "gpt-5" # a comment

[mcp_servers.012-demo]
command = "/opt/012/bin/012"
args = ["mcp", "/tmp/demo.012"]

[mcp_servers.other.http_headers]
Authorization = "Bearer secret"
`
	os.WriteFile(path, []byte(old), 0o600)
	if code, err := status(e, "agent", "--install-mcp", "codex"); code != 0 {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	want := old + "\n[mcp_servers.012]\ncommand = \"/opt/012/bin/012\"\nargs = [\"mcp\"]\n"
	if string(data) != want {
		t.Errorf("config.toml:\n%s\nwant\n%s", data, want)
	}
	if !strings.Contains(out.String(), `also starts 012 as "012-demo"`) || backups(t, path) != 1 {
		t.Errorf("said %q, %d backups", out, backups(t, path))
	}
	out.Reset()
	if status(e, "agent", "--install-mcp", "codex"); !strings.Contains(out.String(), "is up to date") {
		t.Errorf("again: %q", out)
	}

	// A table already there keeps its other keys; its command and args
	// (an array over several lines) are replaced.
	os.WriteFile(path, []byte("[mcp_servers.\"012\"]\ncommand = \"012\"\nargs = [\n  \"mcp\",\n  \"a.012\",\n]\nstartup_timeout_sec = 20\n\n[mcp_servers.\"012\".env]\nX = \"1\"\n"), 0o600)
	if code, err := status(e, "agent", "--install-mcp", "codex", "--root", home); code != 0 {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	want = "[mcp_servers.\"012\"]\ncommand = \"/opt/012/bin/012\"\nargs = [\"mcp\", \"--root\", \"" + home + "\"]\nstartup_timeout_sec = 20\n\n[mcp_servers.\"012\".env]\nX = \"1\"\n"
	if string(data) != want {
		t.Errorf("replaced:\n%s\nwant\n%s", data, want)
	}
}

func TestInstallMCPDesktopAndPrint(t *testing.T) {
	mcpHome(t)
	e, out, _ := scriptEnv(t)
	path, err := desktopConfig(e)
	if err != nil {
		t.Fatal(err)
	}
	if code, err := status(e, "agent", "--install-mcp", "desktop", "--print"); code != 0 || !strings.Contains(out.String(), `"command": "/opt/012/bin/012"`) {
		t.Fatalf("--print: %v %q", err, out)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("--print wrote the file")
	}
	if code, err := status(e, "agent", "--install-mcp", "desktop"); code != 0 {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var got map[string]map[string]json.RawMessage
	if json.Unmarshal(data, &got) != nil || !sameJSON(got["mcpServers"]["012"], []byte(`{"command":"/opt/012/bin/012","args":["mcp"]}`)) || backups(t, path) != 0 {
		t.Errorf("claude_desktop_config.json: %s", data)
	}
	out.Reset()
	if status(e, "agent", "--install-mcp", "codex", "--print"); !strings.Contains(out.String(), "[mcp_servers.012]\ncommand = \"/opt/012/bin/012\"\nargs = [\"mcp\"]\n") {
		t.Errorf("codex --print: %q", out)
	}
	for _, args := range [][]string{{"--install-mcp", "vscode"}, {"--install-mcp", "codex", "--skill"}, {"--print"}} {
		if code, _ := status(e, append([]string{"agent"}, args...)...); code != 2 {
			t.Errorf("%v: status %d", args, code)
		}
	}
}
