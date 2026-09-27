package config

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var (
	// inlineSetting is a setting quoted in prose: `theme = Dracula`.
	inlineSetting = regexp.MustCompile("`([a-z][a-z0-9-]*) = [^`]*`")
	// fileSetting is a line of a config example.
	fileSetting = regexp.MustCompile(`^([a-z][a-z0-9-]*)\s*=`)
	// envVar is one of 012's own variables, or the JEV service's.
	envVar = regexp.MustCompile(`\b(?:O12|TYPESAFE)_[A-Z_]+\b`)
)

// notOptions are variables the docs name that aren't options: the JEV key
// never goes through the config (see docs/jev.md).
var notOptions = []string{"TYPESAFE_API_KEY"}

// TestDocsNameOptions checks the settings and variables the docs name are
// options. A config example is a fenced block whose first line is a
// comment naming the file, `# ~/.config/012/config`; other blocks (theme
// files, shell) aren't read.
func TestDocsNameOptions(t *testing.T) {
	paths, err := filepath.Glob("../../docs/*.md")
	if err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, o := range Options {
		env = append(env, o.Env...)
	}
	for _, p := range append(paths, "../../README.md") {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		name := strings.TrimPrefix(p, "../../")
		check := func(key string) {
			if _, ok := Lookup(key); !ok && key != "key" {
				t.Errorf("%s names setting %q, which isn't an option", name, key)
			}
		}
		for _, m := range inlineSetting.FindAllStringSubmatch(string(data), -1) {
			check(m[1])
		}
		for _, block := range configBlocks(string(data)) {
			for _, l := range block {
				if m := fileSetting.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
					check(m[1])
				}
			}
		}
		for _, v := range envVar.FindAllString(string(data), -1) {
			if !slices.Contains(env, v) && !slices.Contains(notOptions, v) {
				t.Errorf("%s names %s, which no option reads", name, v)
			}
		}
	}
}

// configBlocks returns the lines of the fenced blocks that are config
// examples.
func configBlocks(doc string) [][]string {
	var out [][]string
	var cur []string
	in := false
	for l := range strings.SplitSeq(doc, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			if in && len(cur) > 0 && strings.HasPrefix(cur[0], "#") && strings.Contains(cur[0], "012/config") {
				out = append(out, cur)
			}
			in, cur = !in, nil
			continue
		}
		if in {
			cur = append(cur, strings.TrimSpace(l))
		}
	}
	return out
}
