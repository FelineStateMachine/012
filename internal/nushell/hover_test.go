package nushell

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// hoverAsked are the bytes of testdata/ide/hover.nu that hover-*.json
// record nu's hover at.
var hoverAsked = map[string]int{"sort-by": 17, "flag": 30, "into-string": 45, "variable": 76, "none": 15}

func TestParseHover(t *testing.T) {
	src := string(recorded(t, "hover.nu"))
	for name, want := range map[string]string{"sort-by": "sort-by", "flag": "--reverse", "into-string": "into string", "variable": "$n"} {
		h, ok, err := ParseHover(recorded(t, "hover-"+name+".json"))
		if err != nil || !ok || src[h.From:h.To] != want {
			t.Errorf("%s: %+v %v %v", name, h, ok, err)
		}
	}
	if h, ok, err := ParseHover(recorded(t, "hover-variable.json")); !ok || err != nil || h.Text != "int" {
		t.Errorf("a variable's type: %q", h.Text)
	}
	if _, ok, err := ParseHover(recorded(t, "hover-none.json")); ok || err != nil {
		t.Errorf("no word: %v %v", ok, err)
	}
	if _, _, err := ParseHover([]byte(`{"other": 1}`)); !errors.Is(err, ErrOld) {
		t.Errorf("unreadable answer: %v", err)
	}
}

func TestParseHelp(t *testing.T) {
	h, _, _ := ParseHover(recorded(t, "hover-sort-by.json"))
	help, ok := ParseHelp(h.Text)
	if !ok || help.Name != "sort-by" || help.Summary() != "Sort by the given cell path or closure." {
		t.Fatalf("help %+v", help)
	}
	if got := help.Signature(); got != "sort-by <...comparator: cell-path|closure> --reverse --ignore-case --natural --custom" {
		t.Errorf("signature %q", got)
	}
	if f, ok := help.Flag("-r"); !ok || f.Long != "--reverse" || f.Desc != "Sort in reverse order." {
		t.Errorf("-r: %+v", f)
	}
	if _, ok := help.Flag("--help"); ok {
		t.Error("--help is every command's")
	}
	if !slices.Equal(help.Types, []string{"list<any> | list<any>", "record | table", "table | table"}) {
		t.Errorf("types %q", help.Types)
	}
	if len(help.Examples) != 6 || help.Examples[0] != (Example{"Sort files by modified date.", "ls | sort-by modified"}) {
		t.Errorf("examples %q", help.Examples)
	}
	h, _, _ = ParseHover(recorded(t, "hover-into-string.json"))
	help, _ = ParseHelp(h.Text)
	if got := help.Signature(); got != "into string <...rest: cell-path> --group-digits --decimals <int>" {
		t.Errorf("signature %q", got)
	}
	if help.Name != "into string" || DocsURL(help.Name) != "https://www.nushell.sh/commands/docs/into_string.html" {
		t.Errorf("docs of %q: %s", help.Name, DocsURL(help.Name))
	}
	if _, ok := ParseHelp("int"); ok {
		t.Error("a variable's type read as help")
	}
}

// A command of the script's own, without a description.
func TestParseHelpOfADef(t *testing.T) {
	help, ok := ParseHelp("\n### Usage\n```\n  greet {flags} <name>\n```\n\n### Flags\n\n  `--loud`\\\n  `-h`, `--help` - Display the help message for this command\n\n### Parameters\n\n  `name: string`\n\n")
	if !ok || help.Desc != "" || help.Signature() != "greet <name: string> --loud" {
		t.Errorf("help %+v %q", help, help.Signature())
	}
}

// The real nu, when it's the version recorded, says what hover-*.json
// record.
func TestNuHover(t *testing.T) {
	if _, err := exec.LookPath("nu"); err != nil {
		t.Skip("nu isn't installed")
	}
	if v, err := Version(context.Background(), Nu{}); err != nil || v != strings.TrimSpace(string(recorded(t, "version.txt"))) {
		t.Skipf("nu %s %v isn't the version recorded", v, err)
	}
	src := string(recorded(t, "hover.nu"))
	for name, at := range hoverAsked {
		got, gotOK, err := HoverAt(context.Background(), Nu{}, src, at)
		want, wantOK, _ := ParseHover(recorded(t, "hover-"+name+".json"))
		if err != nil || got != want || gotOK != wantOK {
			t.Errorf("%s: nu says %+v %v %v, recorded %+v", name, got, gotOK, err, want)
		}
	}
}
