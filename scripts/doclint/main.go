// Command doclint keeps documentation about the code as it is. It reads
// Go comments and Markdown prose (not code, strings or code blocks) and
// reports wording that narrates how the code got here: what changed,
// what it did before, what's temporary, who did it and when. History
// belongs in commit messages; docs and comments say what is and why.
//
// Saying why something stays compatible with a planned change is fine
// ("kept a pointer so the store can change shape"), since it explains
// the code today. A line that must use a flagged word says so with
// "doclint:allow" and a reason, in a comment on that line (or an HTML
// comment in Markdown).
//
// Usage: go run ./scripts/doclint [dirs or files...]
package main

import (
	"bufio"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// rules are the phrasings that describe development rather than the code.
var rules = []struct {
	re       *regexp.Regexp
	why      string
	markdown bool // only in Markdown, where the phrase can't be about code
	notTests bool // not in tests, which narrate the steps they take
}{
	{re: re(`\bno longer (read|supported|used|needed|exists?|works?|possible|available|goes|runs)\b`), why: "describes a past state"},
	{re: re(`\bused to (be|have|take|go|cost|read|write|live|run|do|make|scan|walk|say|show)\b|\bpreviously\b|\bformerly\b|\boriginally\b`), why: "describes a past state"},
	{re: regexp.MustCompile(`\b(it|this|that|they|which|012|[a-z]+s) now (uses?|takes?|goes|lives?|sits?|keeps?|returns?|comes?|reads?|writes?|has|have|is|are|gets?|works?)\b|\b(is|are) now\b`), why: `"now" narrates a change`, notTests: true},
	{re: re(`\b(was|were|has been|have been) (moved|renamed|replaced|rewritten|split|merged)\b|\b(moved|renamed) (from|out of|into)\b`), why: "narrates a change"},
	{re: re(`\bfor now\b|\btemporarily\b|\bat the moment\b|\bfor the time being\b|\buntil (it|this|that) lands\b`), why: "describes a transient state"},
	{re: regexp.MustCompile(`\b(TODO|FIXME|XXX|HACK)\b`), why: "a work note; put it in ROADMAP.md instead"},
	{re: regexp.MustCompile(`\(new\)|\bnew in v?\d|\bas of v\d|\bsince v\d`), why: "dates the text"},
	{re: re(`\bbefore (and|->|→|/) after\b|\bthis (commit|change|PR)\b|\bin the commit that\b`), why: "narrates a change", markdown: true},
	{re: re(`\b(the lead|an? agent|merged with main|merge of main)\b`), why: "narrates who changed it"},
}

func re(s string) *regexp.Regexp { return regexp.MustCompile(`(?i)` + s) }

// skip lists paths whose purpose is history or plans: the roadmap tracks
// what's done and ahead, NOTICE and LICENSE are legal text.
var skip = map[string]bool{"ROADMAP.md": true, "NOTICE": true, "LICENSE": true}

type finding struct {
	file string
	line int
	text string
	why  string
}

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{"."}
	}
	var found []finding
	for _, root := range roots {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := d.Name()
			if d.IsDir() {
				if path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "node_modules") {
					return filepath.SkipDir
				}
				return nil
			}
			if skip[filepath.ToSlash(path)] || skip[name] {
				return nil
			}
			switch filepath.Ext(name) {
			case ".go":
				found = append(found, goComments(path)...)
			case ".md":
				found = append(found, markdown(path)...)
			}
			return nil
		})
	}
	for _, f := range found {
		fmt.Printf("%s:%d: %s: %s\n", f.file, f.line, f.why, strings.TrimSpace(f.text))
	}
	if len(found) > 0 {
		fmt.Printf("doclint: %d lines describe development rather than the code\n", len(found))
		os.Exit(1)
	}
}

// check reports the rules text breaks, unless it's allowed.
func check(file string, line int, text string) []finding {
	if strings.Contains(text, "doclint:allow") {
		return nil
	}
	md := strings.HasSuffix(file, ".md")
	test := strings.HasSuffix(file, "_test.go")
	var out []finding
	for _, r := range rules {
		if r.markdown && !md || r.notTests && test {
			continue
		}
		if r.re.MatchString(text) {
			out = append(out, finding{file, line, text, r.why})
			break
		}
	}
	return out
}

func goComments(path string) []finding {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil // not ours to judge; the compiler reports it
	}
	var out []finding
	for _, g := range f.Comments {
		for _, c := range g.List {
			line := fset.Position(c.Pos()).Line
			for i, l := range strings.Split(c.Text, "\n") {
				out = append(out, check(path, line+i, l)...)
			}
		}
	}
	return out
}

func markdown(path string) []finding {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []finding
	fenced := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 1; sc.Scan(); n++ {
		l := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		out = append(out, check(path, n, stripCode(l))...)
	}
	return out
}

var inlineCode = regexp.MustCompile("`[^`]*`")

// stripCode drops inline code spans, which quote code rather than narrate.
func stripCode(l string) string { return inlineCode.ReplaceAllString(l, "``") }
