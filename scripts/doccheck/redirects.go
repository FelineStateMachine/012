package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
)

// The docs site keeps its old addresses: a page that moves or goes
// leaves a redirect in website/redirects.json, from its old address to
// where its reader goes now. The site's build checks that each redirect
// leads to a page; checkRedirects checks that none is missing, by
// finding every docs page the site has had since it began (in git's
// history) that isn't a page any more.

// siteConfig is the file whose first commit began the site.
const siteConfig = "website/docusaurus.config.ts"

// redirectsFile lists the site's redirects.
const redirectsFile = "website/redirects.json"

// checkRedirects reports each docs page the site has published that is
// gone without a redirect from its address. Without git or the history
// back to the site's beginning (a shallow clone), there is nothing to
// compare with, and it checks nothing.
func checkRedirects(docs map[string]*doc) []string {
	published, ok := publishedPages()
	if !ok {
		return nil
	}
	b, err := os.ReadFile(redirectsFile)
	if err != nil {
		return []string{redirectsFile + ": " + err.Error()}
	}
	var redirects []struct{ From, To string }
	if err := json.Unmarshal(b, &redirects); err != nil {
		return []string{redirectsFile + ": " + err.Error()}
	}
	from := map[string]bool{}
	for _, r := range redirects {
		from[r.From] = true
	}
	var problems []string
	for _, p := range published {
		if _, still := docs[p]; still {
			continue
		}
		if url := pageURL(p); !from[url] {
			problems = append(problems, fmt.Sprintf("%s: gone, but the site has published it at %s: add a redirect from there in %s", p, url, redirectsFile))
		}
	}
	return problems
}

// publishedPages lists the docs pages that existed when the site began
// or were added, changed or deleted since, sorted.
func publishedPages() ([]string, bool) {
	adds, err := gitLines("log", "--diff-filter=A", "--format=%H", "--", siteConfig)
	if err != nil || len(adds) == 0 {
		return nil, false
	}
	first := adds[len(adds)-1]
	then, err := gitLines("ls-tree", "-r", "--name-only", first, "--", "docs")
	if err != nil {
		return nil, false
	}
	since, err := gitLines("log", "--no-renames", "--format=", "--name-only", first+"..HEAD", "--", "docs")
	if err != nil {
		return nil, false
	}
	var pages []string
	for _, p := range append(then, since...) {
		if isPage(p) {
			pages = append(pages, p)
		}
	}
	slices.Sort(pages)
	return slices.Compact(pages), true
}

func gitLines(args ...string) ([]string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// isPage reports whether p, a path under docs/, is a page of the site:
// Markdown not excluded by a leading _ (see the docs preset's exclude).
func isPage(p string) bool {
	if path.Ext(p) != ".md" {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if strings.HasPrefix(part, "_") {
			return false
		}
	}
	return true
}

// pageURL is where the site serves the page at p: its path without
// .md and with a trailing slash, a folder's README.md at the folder's
// address.
func pageURL(p string) string {
	p = strings.TrimSuffix(p, ".md")
	if base := path.Base(p); base == "README" {
		p = path.Dir(p)
	}
	return "/" + p + "/"
}
