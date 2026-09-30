package main

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// 012 agent --skill and 012 agent --install-skill: the Claude Code
// skill that teaches agents with a shell to use 012 (see
// docs/agents/README.md), installed as 012 nu --install-module installs
// the nushell module. 012 agent --install-mcp adds 012 mcp to a host
// (installmcp.go).

//go:embed skill/SKILL.md
var skillSource string

const agentUsage = "usage: 012 agent --skill | --install-skill [--force] [dir] | --install-mcp claude|codex|desktop [--root dir]... [--print]"

// skillHome is where Claude Code looks for a person's own skills; tests
// replace it.
var skillHome = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding your home directory: %w; name the folder instead: 012 agent --install-skill path/to/skills/012", err)
	}
	return filepath.Join(home, ".claude", "skills", "012"), nil
}

// runAgent is 012 agent --skill (print the skill), --install-skill
// [--force] [dir], or --install-mcp host [--root dir]... [--print].
func runAgent(args []string, e env) error {
	a, err := parseArgs(args, []string{"install-mcp", "root"}, []string{"skill", "install-skill", "force", "print", "help"})
	switch {
	case err != nil:
		return usageError(err.Error(), agentUsage)
	case a.has("help"):
		fmt.Fprintln(e.stdout, agentUsage)
		return nil
	case a.has("install-mcp"):
		if a.has("skill") || a.has("install-skill") || a.has("force") || len(a.pos) > 0 {
			return usageError("", agentUsage)
		}
		return installMCP(e, a.flags["install-mcp"], a.all["root"], a.has("print"))
	case a.has("root") || a.has("print"):
		return usageError("--root and --print go with --install-mcp", agentUsage)
	case a.has("skill") && !a.has("install-skill") && len(a.pos) == 0:
		_, err := fmt.Fprint(e.stdout, skillSource)
		return err
	case !a.has("install-skill") || a.has("skill") || len(a.pos) > 1:
		return usageError("", agentUsage)
	}
	path, err := skillPath(a.pos)
	if err != nil {
		return err
	}
	wrote, err := installSkill(path, a.has("force"))
	if err != nil {
		return err
	}
	if wrote {
		fmt.Fprintln(e.stdout, "Wrote", path)
	} else {
		fmt.Fprintln(e.stdout, path, "is up to date")
	}
	fmt.Fprintln(e.stdout, "Claude Code finds it in the next session; /skills lists it.")
	return nil
}

// skillPath is the SKILL.md to write: in the folder given (a file named
// there, if it ends in .md), or in ~/.claude/skills/012.
func skillPath(given []string) (string, error) {
	dir := ""
	if len(given) == 1 {
		p, err := filepath.Abs(given[0])
		if err != nil {
			return "", err
		}
		if strings.EqualFold(filepath.Ext(p), ".md") {
			return p, nil
		}
		dir = p
	} else {
		var err error
		if dir, err = skillHome(); err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "SKILL.md"), nil
}

// installSkill writes the skill to path, creating its folder. A file
// already there is replaced only with force, unless it's this skill;
// wrote is false then.
func installSkill(path string, force bool) (wrote bool, err error) {
	old, err := os.ReadFile(path)
	switch {
	case err == nil && bytes.Equal(old, []byte(skillSource)):
		return false, nil
	case err == nil && !force:
		return false, fmt.Errorf("%s exists and isn't this 012's skill; --force replaces it", path)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(skillSource), 0o644)
}
