// Command doccheck keeps the documentation's structure sound, where a
// machine can tell:
//
//   - every relative link and image in Markdown points at a file that
//     exists, and every #anchor at a heading of that file;
//   - docs/ is a Docusaurus tree: every folder has a README.md that links
//     its pages and subfolders, and a valid _category_.json, and every
//     page front matter with a title and a sidebar_position (tree.go);
//   - every docs/...md path Go code names exists;
//   - every file in docs/media is shown by some doc, comes from a VHS tape
//     in demos/ (its Output or a Screenshot) or is a still of a golden
//     screen (e2e/stills_test.go), and no GIF is over maxGIF;
//   - every tape in demos/ records something a doc shows, and every
//     still is shown, its dark and light pictures together;
//   - every docs page the site has published is still a page, or has a
//     redirect from its address in website/redirects.json (redirects.go).
//
// What the docs say about the program (keys, menus, commands, options) is
// checked by tests next to the code they name: internal/ui's docs_test.go
// and internal/config's.
//
// Usage: go run ./scripts/doccheck (from the repository root)
package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// maxGIF keeps the README's recordings quick to load.
const maxGIF = 1 << 20

func main() {
	docs, err := readDocs(".")
	if err != nil {
		fmt.Println("doccheck:", err)
		os.Exit(1)
	}
	var problems []string
	problems = append(problems, checkLinks(docs)...)
	problems = append(problems, checkTree(docs)...)
	problems = append(problems, checkGoPaths()...)
	problems = append(problems, checkMedia(docs)...)
	problems = append(problems, checkRedirects(docs)...)
	for _, p := range problems {
		fmt.Println(p)
	}
	if len(problems) > 0 {
		fmt.Printf("doccheck: %d problems\n", len(problems))
		os.Exit(1)
	}
}

// doc is a Markdown file: its links and the anchors of its headings.
type doc struct {
	path    string // slash-separated, relative to the root
	links   []link
	anchors map[string]bool
}

type link struct {
	line   int
	target string
}

// readDocs reads every Markdown file under root, skipping hidden
// directories, test data and generated output.
func readDocs(root string) (map[string]*doc, error) {
	docs := map[string]*doc{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "node_modules" || name == "out") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(name) != ".md" {
			return nil
		}
		dc, err := parseDoc(p)
		if err != nil {
			return err
		}
		docs[dc.path] = dc
		return nil
	})
	return docs, err
}

var (
	linkRE     = regexp.MustCompile(`!?\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	inlineCode = regexp.MustCompile("`[^`]*`")
	headingRE  = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
	anchorTag  = regexp.MustCompile(`<a\s+(?:name|id)="([^"]+)"`)
)

func parseDoc(p string) (*doc, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dc := &doc{path: filepath.ToSlash(filepath.Clean(p)), anchors: map[string]bool{}}
	seen := map[string]int{}
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
		if m := headingRE.FindStringSubmatch(l); m != nil {
			s := slug(m[1])
			if k := seen[s]; k > 0 {
				dc.anchors[fmt.Sprintf("%s-%d", s, k)] = true
			} else {
				dc.anchors[s] = true
			}
			seen[s]++
		}
		for _, m := range anchorTag.FindAllStringSubmatch(l, -1) {
			dc.anchors[m[1]] = true
		}
		for _, m := range linkRE.FindAllStringSubmatch(inlineCode.ReplaceAllString(l, "``"), -1) {
			dc.links = append(dc.links, link{n, m[1]})
		}
	}
	return dc, sc.Err()
}

// slug is the anchor GitHub gives a heading: lower case, spaces as
// hyphens, punctuation other than hyphens and underscores dropped.
func slug(heading string) string {
	heading = strings.ToLower(inlineCode.ReplaceAllStringFunc(heading, func(s string) string { return strings.Trim(s, "`") }))
	var b strings.Builder
	for _, r := range heading {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r > 127 && isLetter(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isLetter(r rune) bool { return strings.ToLower(string(r)) != strings.ToUpper(string(r)) }

// checkLinks reports relative links to missing files or anchors.
func checkLinks(docs map[string]*doc) []string {
	var out []string
	for _, p := range sortedKeys(docs) {
		dc := docs[p]
		for _, l := range dc.links {
			t := l.target
			if strings.Contains(t, "://") || strings.HasPrefix(t, "mailto:") {
				continue
			}
			file, anchor, _ := strings.Cut(t, "#")
			target := dc.path
			if file != "" {
				target = path.Clean(path.Join(path.Dir(dc.path), file))
			}
			where := fmt.Sprintf("%s:%d: %s", dc.path, l.line, t)
			if _, err := os.Stat(filepath.FromSlash(target)); err != nil {
				out = append(out, where+": no such file")
				continue
			}
			if anchor == "" || modeOnly[anchor] {
				continue // the color mode a still shows in, checked in checkMedia
			}
			td, ok := docs[target]
			if !ok {
				out = append(out, where+": an anchor into a file that isn't Markdown")
			} else if !td.anchors[anchor] {
				out = append(out, where+": no heading with that anchor in "+target)
			}
		}
	}
	return out
}

var (
	tapeOutput     = regexp.MustCompile(`(?m)^Output\s+"?out/([^"\s]+)"?`)
	tapeScreenshot = regexp.MustCompile(`(?m)^Screenshot\s+"?out/stills/([^"\s]+)"?`)
	// docScreen is an entry of e2e's docScreens, a still drawn from a
	// golden screen as <name>-dark.png and <name>-light.png.
	docScreen = regexp.MustCompile(`(?m)^\s*\{name: "([^"]+)", from:`)
)

// stillsSource lists the stills of golden screens.
const stillsSource = "e2e/stills_test.go"

// modeOnly are the fragments GitHub and the site show an image by: only
// in dark mode, or only in light.
var modeOnly = map[string]bool{"gh-dark-mode-only": true, "gh-light-mode-only": true}

// checkMedia reports media no doc shows, media nothing makes, GIFs over
// maxGIF, tapes and stills no doc shows, and stills shown without their
// other color mode.
func checkMedia(docs map[string]*doc) []string {
	// shown maps a media path to the fragment each doc shows it with.
	shown := map[string]map[string]string{}
	for _, dc := range docs {
		for _, l := range dc.links {
			file, frag, _ := strings.Cut(l.target, "#")
			if file != "" && !strings.Contains(file, "://") {
				p := path.Clean(path.Join(path.Dir(dc.path), file))
				if shown[p] == nil {
					shown[p] = map[string]string{}
				}
				shown[p][dc.path] = frag
			}
		}
	}
	madeBy := map[string]string{} // media file name -> the tape or stills list
	out := tapeMedia(shown, madeBy)
	out = append(out, stillMedia(shown, madeBy)...)
	media, _ := filepath.Glob("docs/media/*")
	for _, m := range media {
		m = filepath.ToSlash(m)
		name := path.Base(m)
		if shown[m] == nil {
			out = append(out, m+": no doc shows it")
		}
		if madeBy[name] == "" {
			out = append(out, m+": nothing makes it: no tape in demos/ (Output or Screenshot), nor docScreens in "+stillsSource)
		}
		if fi, err := os.Stat(m); err == nil && path.Ext(m) == ".gif" && fi.Size() > maxGIF {
			out = append(out, fmt.Sprintf("%s: %d KB, over %d KB", m, fi.Size()>>10, maxGIF>>10))
		}
	}
	return out
}

// tapeMedia records what each tape makes in madeBy and reports tapes
// whose recordings no doc shows.
func tapeMedia(shown map[string]map[string]string, madeBy map[string]string) []string {
	tapes, _ := filepath.Glob("demos/*.tape")
	var out []string
	for _, t := range tapes {
		src, err := os.ReadFile(t)
		if err != nil {
			out = append(out, t+": "+err.Error())
			continue
		}
		var made []string
		for _, re := range []*regexp.Regexp{tapeOutput, tapeScreenshot} {
			for _, m := range re.FindAllStringSubmatch(string(src), -1) {
				made = append(made, m[1])
				madeBy[m[1]] = t
			}
		}
		if !slices.ContainsFunc(made, func(name string) bool { return shown["docs/media/"+name] != nil }) {
			out = append(out, t+": no doc shows its recording or stills (docs/media/"+strings.Join(made, ", docs/media/")+")")
		}
	}
	return out
}

// stillMedia records the stills of golden screens in madeBy, and reports
// one no doc shows, or one shown without its other color mode: a doc
// shows <name>-dark.png#gh-dark-mode-only and
// <name>-light.png#gh-light-mode-only together.
func stillMedia(shown map[string]map[string]string, madeBy map[string]string) []string {
	src, err := os.ReadFile(stillsSource)
	if err != nil {
		return []string{stillsSource + ": " + err.Error()}
	}
	var out []string
	for _, m := range docScreen.FindAllStringSubmatch(string(src), -1) {
		dark, light := "docs/media/"+m[1]+"-dark.png", "docs/media/"+m[1]+"-light.png"
		madeBy[path.Base(dark)], madeBy[path.Base(light)] = stillsSource, stillsSource
		if shown[dark] == nil && shown[light] == nil {
			out = append(out, stillsSource+": no doc shows the still "+m[1])
		}
		for _, d := range sortedKeys(mergeKeys(shown[dark], shown[light])) {
			if shown[dark][d] != "gh-dark-mode-only" || shown[light][d] != "gh-light-mode-only" {
				out = append(out, d+": shows the still "+m[1]+" without both "+path.Base(dark)+"#gh-dark-mode-only and "+path.Base(light)+"#gh-light-mode-only")
			}
		}
	}
	return out
}

func mergeKeys(a, b map[string]string) map[string]bool {
	out := map[string]bool{}
	for k := range a {
		out[k] = true
	}
	for k := range b {
		out[k] = true
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
