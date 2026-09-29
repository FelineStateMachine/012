package sheet

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// saveFixtures names a release: go test -run TestFixtures -fixtures
// v1.2.3 saves the workbooks in testdata/fixtures/new with this build
// into testdata/fixtures/v1.2.3. See docs/contributing/releasing.md.
var saveFixtures = flag.String("fixtures", "", "save testdata/fixtures/new into testdata/fixtures/<release>")

var releaseDir = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// TestFixturesOpenAndSaveUnchanged opens every workbook a release saved
// (testdata/fixtures/<release>) and saves it again: the bytes must be
// the file's own. The writer puts every map in a fixed order (cells by
// row then column, widths and lines by column, names by name), so byte
// order never legitimately varies and the comparison is exact.
// v0.2.0's fixtures were saved by that tag from fixtures/new, less the
// rule and validation options it refuses (date periods, top values,
// duplicates, averages, data bars, icon sets, dropdown display and
// checkbox values).
func TestFixturesOpenAndSaveUnchanged(t *testing.T) {
	if *saveFixtures != "" {
		writeFixtures(t, *saveFixtures)
	}
	dirs, err := os.ReadDir("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range dirs {
		if !d.IsDir() || !releaseDir.MatchString(d.Name()) {
			continue
		}
		files, _ := filepath.Glob(filepath.Join("testdata/fixtures", d.Name(), "*.012"))
		for _, path := range files {
			n++
			t.Run(d.Name()+"/"+filepath.Base(path), func(t *testing.T) {
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if got := resave(t, want); !bytes.Equal(got, want) {
					t.Errorf("saved differently:\n%s", lineDiff(string(want), string(got)))
				}
			})
		}
	}
	if n == 0 {
		t.Fatal("no fixtures in testdata/fixtures/v*")
	}
}

// TestFixtureSourcesStable checks that the workbooks the next release's
// fixtures are made from open, and that what this build saves of them
// saves again unchanged, so -fixtures always makes fixtures that pass.
func TestFixtureSourcesStable(t *testing.T) {
	files, _ := filepath.Glob("testdata/fixtures/new/*.012")
	if len(files) == 0 {
		t.Fatal("no workbooks in testdata/fixtures/new")
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			once := resave(t, src)
			if twice := resave(t, once); !bytes.Equal(once, twice) {
				t.Errorf("saved differently the second time:\n%s", lineDiff(string(once), string(twice)))
			}
		})
	}
}

// resave opens a file and returns what this build saves of it.
func resave(t *testing.T, data []byte) []byte {
	t.Helper()
	wb, err := ReadBook(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	var out bytes.Buffer
	if err := wb.Write(&out); err != nil {
		t.Fatalf("save: %v", err)
	}
	return out.Bytes()
}

// writeFixtures saves testdata/fixtures/new with this build as the
// fixtures of release, refusing to replace a release's fixtures.
func writeFixtures(t *testing.T, release string) {
	if !releaseDir.MatchString(release) {
		t.Fatalf("-fixtures %q: want a release such as v1.2.3", release)
	}
	dir := filepath.Join("testdata/fixtures", release)
	if _, err := os.Stat(dir); err == nil {
		t.Fatalf("%s exists: a release's fixtures are never replaced", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob("testdata/fixtures/new/*.012")
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(path)), resave(t, src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// lineDiff lists the lines of want and got that differ, by line number.
func lineDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	var b strings.Builder
	for i := range max(len(w), len(g)) {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			fmt.Fprintf(&b, "%d:\n  want %s\n  got  %s\n", i+1, wl, gl)
		}
	}
	return b.String()
}
