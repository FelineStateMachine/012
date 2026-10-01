package main

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// The pictures' color schemes: docs/contributing/site.md has a table of
// families, each a dark and a light scheme and the pages in it. Every
// page that shows a picture is in one family, and so is every picture
// it shows: a still's docScreens entry names its family, and a tape's
// Set Theme is one of the family's two schemes.

const familiesDoc = "docs/contributing/site.md"

var (
	// familyRow is a row of the table: | Family | `dark` | `light` | pages |.
	familyRow = regexp.MustCompile("(?m)^\\| ([^|`]+?) \\| `([^`]+)` \\| `([^`]+)` \\| (.+) \\|$")
	setTheme  = regexp.MustCompile(`(?m)^Set Theme\s+"([^"]+)"`)
	// stillFamily is a docScreens entry's name and family.
	stillFamily = regexp.MustCompile(`(?m)^\s*\{name: "([^"]+)", from:.*family: "([^"]+)"\}`)
)

type family struct {
	name, dark, light string
	pages             []string // pages and folders, relative to docs/
}

// readFamilies reads the families table.
func readFamilies() ([]family, error) {
	src, err := os.ReadFile(familiesDoc)
	if err != nil {
		return nil, err
	}
	var out []family
	for _, m := range familyRow.FindAllStringSubmatch(string(src), -1) {
		f := family{name: m[1], dark: m[2], light: m[3]}
		for _, p := range strings.Split(m[4], ",") {
			f.pages = append(f.pages, strings.Trim(strings.TrimSpace(p), "`"))
		}
		out = append(out, f)
	}
	return out, nil
}

// familyOf is the family of a page under docs/: the one naming the page,
// or else the deepest folder holding it; nil when none does.
func familyOf(fams []family, page string) *family {
	rel := strings.TrimPrefix(page, "docs/")
	var best *family
	bestLen := -1
	for i, f := range fams {
		for _, p := range f.pages {
			if (p == rel || strings.HasSuffix(p, "/") && strings.HasPrefix(rel, p)) && len(p) > bestLen {
				best, bestLen = &fams[i], len(p)
			}
		}
	}
	return best
}

// checkFamilies reports pictures in a family other than the pages
// showing them, and pages showing pictures in no family. shown maps a
// media path to the docs showing it.
func checkFamilies(shown map[string]map[string]string) []string {
	fams, err := readFamilies()
	if err != nil {
		return []string{familiesDoc + ": " + err.Error()}
	}
	if len(fams) == 0 {
		return []string{familiesDoc + ": no table of the pictures' families"}
	}
	// check reports each docs page showing media whose family doesn't
	// pass ok.
	var out []string
	check := func(source string, media []string, ok func(*family) bool, want string) {
		for _, m := range media {
			for _, page := range sortedKeys(shown["docs/media/"+m]) {
				if !strings.HasPrefix(page, "docs/") {
					continue // the README and the landing page show every topic's
				}
				switch f := familyOf(fams, page); {
				case f == nil:
					out = append(out, page+": shows "+m+" but is in no family of "+familiesDoc)
				case !ok(f):
					out = append(out, source+": "+want+", but "+page+", which shows "+m+", is in the family "+f.name)
				}
			}
		}
	}
	setup, _ := os.ReadFile("demos/lib/setup.tape")
	tapes, _ := filepath.Glob("demos/*.tape")
	for _, t := range tapes {
		src, err := os.ReadFile(t)
		if err != nil {
			continue
		}
		themes := setTheme.FindAllStringSubmatch(string(setup)+string(src), -1)
		if len(themes) == 0 {
			out = append(out, t+": no Set Theme, here or in demos/lib/setup.tape")
			continue
		}
		theme := themes[len(themes)-1][1]
		var made []string
		for _, re := range []*regexp.Regexp{tapeOutput, tapeScreenshot} {
			for _, m := range re.FindAllStringSubmatch(string(src), -1) {
				made = append(made, path.Base(m[1]))
			}
		}
		check(t, made, func(f *family) bool { return theme == f.dark || theme == f.light }, "its theme is "+theme)
	}
	if src, err := os.ReadFile(stillsSource); err == nil {
		for _, m := range stillFamily.FindAllStringSubmatch(string(src), -1) {
			name, fam := m[1], m[2]
			if !slices.ContainsFunc(fams, func(f family) bool { return f.name == fam }) {
				out = append(out, stillsSource+": the still "+name+" is in the family "+fam+", which "+familiesDoc+" doesn't list")
			}
			check(stillsSource, []string{name + "-dark.png", name + "-light.png"},
				func(f *family) bool { return f.name == fam }, "the still "+name+" is in the family "+fam)
		}
	}
	return out
}
