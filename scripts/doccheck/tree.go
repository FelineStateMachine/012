package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// The docs are a Docusaurus tree that also reads on GitHub: each folder
// under docs/ has a README.md that indexes it and a _category_.json that
// names it in the sidebar, and each page has front matter with its title
// and sidebar position.

// checkTree reports pages their folder's README.md doesn't link, folders
// their parent's README.md doesn't link, and missing or invalid category
// files and front matter.
func checkTree(docs map[string]*doc) []string {
	var out []string
	folders := map[string]bool{"docs": true}
	for p := range docs {
		if strings.HasPrefix(p, "docs/") {
			folders[path.Dir(p)] = true
		}
	}
	for _, dir := range sortedKeys(folders) {
		index := dir + "/README.md"
		if docs[index] == nil {
			out = append(out, index+": missing; every folder of docs has one")
			continue
		}
		linked := linkedFrom(docs[index])
		for _, p := range sortedKeys(docs) {
			if p != index && path.Dir(p) == dir && !linked[p] {
				out = append(out, index+": doesn't link "+p)
			}
		}
		for sub := range folders {
			if path.Dir(sub) == dir && sub != dir && !linked[sub+"/README.md"] {
				out = append(out, index+": doesn't link "+sub+"/README.md")
			}
		}
		if dir != "docs" {
			out = append(out, checkCategory(dir, folders)...)
		}
	}
	out = append(out, checkFrontMatter(docs)...)
	return out
}

func linkedFrom(dc *doc) map[string]bool {
	linked := map[string]bool{}
	for _, l := range dc.links {
		file, _, _ := strings.Cut(l.target, "#")
		if file != "" && !strings.Contains(file, "://") {
			linked[path.Clean(path.Join(path.Dir(dc.path), file))] = true
		}
	}
	return linked
}

// category is a folder's _category_.json, as Docusaurus reads it.
type category struct {
	Label    string `json:"label"`
	Position *int   `json:"position"`
	Link     *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"link"`
}

// checkCategory reports a folder's missing or invalid _category_.json,
// and positions its siblings share.
func checkCategory(dir string, folders map[string]bool) []string {
	file := dir + "/_category_.json"
	c, err := readCategory(file)
	if err != nil {
		return []string{file + ": " + err.Error()}
	}
	var out []string
	if c.Label == "" || c.Position == nil {
		out = append(out, file+": needs a label and a position")
	}
	if c.Link != nil && c.Link.ID != strings.TrimPrefix(dir, "docs/")+"/README" {
		out = append(out, fmt.Sprintf("%s: link id %q isn't the folder's README", file, c.Link.ID))
	}
	for sib := range folders {
		if sib == dir || path.Dir(sib) != path.Dir(dir) || sib == "docs" || c.Position == nil {
			continue
		}
		if o, err := readCategory(sib + "/_category_.json"); err == nil && o.Position != nil && *o.Position == *c.Position && sib < dir {
			out = append(out, fmt.Sprintf("%s: position %d is also %s's", file, *c.Position, sib))
		}
	}
	return out
}

func readCategory(file string) (category, error) {
	var c category
	data, err := os.ReadFile(file)
	if err != nil {
		return c, fmt.Errorf("missing; every folder of docs has one")
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("not valid JSON: %v", err)
	}
	return c, nil
}

var frontMatter = regexp.MustCompile(`\A---\n((?:.*\n)*?)---\n`)

// checkFrontMatter reports pages under docs/ without a title and a
// sidebar position, and positions pages of one folder share.
func checkFrontMatter(docs map[string]*doc) []string {
	var out []string
	seen := map[string]string{} // folder and position -> page
	for _, p := range sortedKeys(docs) {
		if !strings.HasPrefix(p, "docs/") {
			continue
		}
		data, err := os.ReadFile(filepath.FromSlash(p))
		if err != nil {
			continue
		}
		m := frontMatter.FindSubmatch(data)
		if m == nil {
			out = append(out, p+": no front matter (title, sidebar_position)")
			continue
		}
		fields := map[string]string{}
		for l := range strings.SplitSeq(string(m[1]), "\n") {
			if k, v, ok := strings.Cut(l, ":"); ok {
				fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
		pos, err := strconv.Atoi(fields["sidebar_position"])
		if fields["title"] == "" || err != nil {
			out = append(out, p+": front matter needs a title and a whole-number sidebar_position")
			continue
		}
		key := fmt.Sprintf("%s#%d", path.Dir(p), pos)
		if other, ok := seen[key]; ok {
			out = append(out, fmt.Sprintf("%s: sidebar_position %d is also %s's", p, pos, other))
		}
		seen[key] = p
	}
	return out
}

var goDocPath = regexp.MustCompile(`\bdocs/[a-z0-9_/-]+\.md\b`)

// checkGoPaths reports doc paths named outside the docs, in Go code
// (messages and comments), the Makefile, scripts and deploy configs,
// that don't exist.
func checkGoPaths() []string {
	var out []string
	for _, f := range []string{"Makefile", "CLAUDE.md"} {
		out = append(out, missingDocPaths(f)...)
	}
	for _, root := range []string{"cmd", "internal", "e2e", "oracle", "demos", "scripts", "deploy"} {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && pathBearing[filepath.Ext(p)] {
				out = append(out, missingDocPaths(p)...)
			}
			return nil
		})
	}
	return out
}

// pathBearing are the kinds of file outside docs/ that name doc pages.
var pathBearing = map[string]bool{".go": true, ".sh": true, ".yml": true, ".yaml": true, ".tape": true, ".sql": true, ".json": true}

// missingDocPaths reports the doc paths file p names that don't exist.
func missingDocPaths(p string) []string {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var out []string
	for i, l := range strings.Split(string(data), "\n") {
		for _, m := range goDocPath.FindAllString(l, -1) {
			if _, err := os.Stat(filepath.FromSlash(m)); err != nil {
				out = append(out, fmt.Sprintf("%s:%d: %s: no such doc", p, i+1, m))
			}
		}
	}
	return out
}
