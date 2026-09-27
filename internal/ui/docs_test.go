package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// The docs name menus, commands and keys; these tests keep what they name
// true of the registry. scripts/doccheck checks links, the index and media.

// userDocs reads README.md and docs/*.md with fenced code blocks and
// inline code removed and each paragraph on one line, so a menu path
// wrapped across lines reads whole. The generated function reference is
// left out: it names no menus.
func userDocs(t *testing.T) map[string]string {
	t.Helper()
	paths, err := filepath.Glob("../../docs/*.md")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, "../../README.md")
	out := map[string]string{}
	for _, p := range paths {
		if filepath.Base(p) == "functions.md" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		fenced := false
		for l := range strings.SplitSeq(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "```") {
				fenced = !fenced
				continue
			}
			if !fenced {
				b.WriteString(l)
				b.WriteByte(' ')
			}
		}
		out[strings.TrimPrefix(p, "../../")] = strings.Join(strings.Fields(b.String()), " ")
	}
	return out
}

var menuPath = regexp.MustCompile(`\b(File|Edit|View|Insert|Format|Data|Help) > `)

// TestDocsMenuPaths checks every menu path the docs name ("Data > Macros >
// Record macro") leads to an item of 012's menu bar. A path after a
// possessive ("Sheets' Insert > Pivot table") is another program's.
func TestDocsMenuPaths(t *testing.T) {
	for name, text := range userDocs(t) {
		for _, loc := range menuPath.FindAllStringSubmatchIndex(text, -1) {
			before := strings.TrimRight(text[:loc[0]], " *")
			if strings.HasSuffix(before, "'") || strings.HasSuffix(before, "'s") || strings.HasSuffix(before, "’") {
				continue
			}
			title := text[loc[2]:loc[3]]
			if err := resolveMenuPath(title, text[loc[1]:]); err != "" {
				end := min(len(text), loc[1]+40)
				t.Errorf("%s: %q: %s", name, text[loc[0]:end], err)
			}
		}
	}
}

func TestResolveMenuPath(t *testing.T) {
	for _, c := range []struct{ title, rest, err string }{
		{"Data", "Macros > Record macro, then", ""},
		{"File", "Settings > Vim keys turns", ""},
		{"Data", "Frequency table (column stats), or", ""},
		{"Data", "Frequency table, or", ""},
		{"Data", "Pivots > Edit", "no such item in the Data menu"},
		{"Data", "Macros > Play macro", "no such item in the Macros menu"},
		{"Insert", "Chart > Pie", "Chart has no submenu"},
	} {
		if got := resolveMenuPath(c.title, c.rest); got != c.err {
			t.Errorf("%s > %s: %q, want %q", c.title, c.rest, got, c.err)
		}
	}
}

// resolveMenuPath follows rest, the text after "Title > ", through the
// menu's items, and says what it couldn't find.
func resolveMenuPath(title, rest string) string {
	var items []menuItem
	for _, d := range menuBar {
		if d.title == title {
			items = visibleItems(d.items)
		}
	}
	for {
		found := menuItemAt(items, rest)
		if found == nil {
			return "no such item in the " + title + " menu"
		}
		label := found.label()
		if !labelPrefix(rest, label) {
			label, _, _ = strings.Cut(label, " (")
		}
		rest = rest[len(label):]
		if !strings.HasPrefix(rest, " > ") {
			return ""
		}
		if found.items == nil {
			return label + " has no submenu"
		}
		title, items, rest = label, found.items, rest[len(" > "):]
	}
}

// menuItemAt is the item of items whose label, or its part before a
// parenthesis, text starts with; the longest when several do.
func menuItemAt(items []menuItem, text string) *menuItem {
	var found *menuItem
	for i, it := range items {
		if it.sep {
			continue
		}
		label := it.label()
		short, _, _ := strings.Cut(label, " (")
		for _, l := range []string{label, short} {
			if labelPrefix(text, l) && (found == nil || len(l) > len(found.label())) {
				found = &items[i]
			}
		}
	}
	return found
}

// labelPrefix reports whether text starts with label as whole words.
func labelPrefix(text, label string) bool {
	if !strings.HasPrefix(strings.ToLower(text), strings.ToLower(label)) {
		return false
	}
	rest := text[len(label):]
	return rest == "" || !unicode.IsLetter(rune(rest[0])) && !unicode.IsDigit(rune(rest[0]))
}

var (
	colonCommand = regexp.MustCompile("`:([a-z_]+\\.[a-z_.0-9]+)`")
	runCommandID = regexp.MustCompile(`run\("([a-z_.0-9]+)"`)
)

// TestDocsCommandIDs checks the command ids the docs name, on the vim
// command line (`:settings.vim`) or in scripts (run("format.bold")),
// are registered.
func TestDocsCommandIDs(t *testing.T) {
	for _, p := range []string{"../../docs/keys.md", "../../docs/macros.md"} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, re := range []*regexp.Regexp{colonCommand, runCommandID} {
			for _, m := range re.FindAllStringSubmatch(string(data), -1) {
				if commands[m[1]] == nil {
					t.Errorf("%s names command %q, which isn't registered", p, m[1])
				}
			}
		}
	}
}

// TestKeysDocCoversKeymap checks docs/keys.md lists every key bound to a
// command, written as the menus and help show it.
func TestKeysDocCoversKeymap(t *testing.T) {
	data, err := os.ReadFile("../../docs/keys.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	for k, id := range keymap {
		if commands[id] == nil {
			continue
		}
		label := keyLabel(k)
		re := regexp.MustCompile(`(^|[\s,|(/])` + regexp.QuoteMeta(label) + `($|[\s,|)/.;:])`)
		if !re.MatchString(doc) {
			t.Errorf("docs/keys.md doesn't list %s (%s, %s)", label, id, commands[id].title)
		}
	}
	var vimKeys []string
	for _, table := range []map[string]vimBinding{vimNormal, vimVisual} {
		for k := range table {
			vimKeys = append(vimKeys, k)
		}
	}
	for k := range vimMotions {
		if !isMoveKey(k) { // Sheets' own keys, listed above
			vimKeys = append(vimKeys, k)
		}
	}
	for _, k := range vimKeys {
		if strings.Contains(k, "+") || len(k) > 2 {
			if !strings.Contains(doc, keyLabel(k)) && !strings.Contains(doc, theme.KeyLabel(k)) {
				t.Errorf("docs/keys.md doesn't list the vim key %s", k)
			}
		} else if !strings.Contains(doc, "`"+k+"`") {
			t.Errorf("docs/keys.md doesn't list the vim key `%s`", k)
		}
	}
}
