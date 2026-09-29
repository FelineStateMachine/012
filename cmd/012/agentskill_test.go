package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentSkill(t *testing.T) {
	e, out, _ := scriptEnv(t)
	if code, err := status(e, "agent", "--skill"); code != 0 || !strings.HasPrefix(out.String(), "---\nname: 012\n") {
		t.Fatalf("--skill: %d %v %.40q", code, err, out)
	}
	home := filepath.Join(t.TempDir(), "skills", "012")
	old := skillHome
	skillHome = func() (string, error) { return home, nil }
	t.Cleanup(func() { skillHome = old })

	out.Reset()
	if code, err := status(e, "agent", "--install-skill"); code != 0 || !strings.Contains(out.String(), "Wrote "+filepath.Join(home, "SKILL.md")) {
		t.Fatalf("install: %d %v %q", code, err, out)
	}
	out.Reset()
	if status(e, "agent", "--install-skill"); !strings.Contains(out.String(), "is up to date") {
		t.Errorf("again: %q", out)
	}
	os.WriteFile(filepath.Join(home, "SKILL.md"), []byte("mine"), 0o644)
	if _, err := status(e, "agent", "--install-skill"); err == nil || !strings.Contains(err.Error(), "--force replaces it") {
		t.Errorf("over another file: %v", err)
	}
	if code, err := status(e, "agent", "--install-skill", "--force"); code != 0 {
		t.Error(err)
	}
	dir := filepath.Join(t.TempDir(), "project", ".claude", "skills", "012")
	if code, err := status(e, "agent", "--install-skill", dir); code != 0 {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "SKILL.md")); string(data) != skillSource {
		t.Errorf("project skill not written")
	}
	if code, _ := status(e, "agent"); code != 2 {
		t.Errorf("agent alone: status %d", code)
	}
}
