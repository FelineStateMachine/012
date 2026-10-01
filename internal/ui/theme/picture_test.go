package theme

import (
	"os"
	"regexp"
	"testing"
)

// pictureFamily is a row of the table of the docs pictures' color
// families in docs/contributing/site.md: | Family | `dark` | `light` |.
var pictureFamily = regexp.MustCompile("(?m)^\\| ([^|`]+?) \\| `([^`]+)` \\| `([^`]+)` \\|")

// The schemes the docs' recordings and stills are drawn in are built-in
// schemes under the names VHS knows them by, dark and light as the table
// says, with text that reads on their background as they are, and 012's
// theme in them readable.
func TestPictureSchemes(t *testing.T) {
	src, err := os.ReadFile("../../../docs/contributing/site.md")
	if err != nil {
		t.Fatal(err)
	}
	rows := pictureFamily.FindAllStringSubmatch(string(src), -1)
	if len(rows) == 0 {
		t.Fatal("no families table in site.md")
	}
	for _, row := range rows {
		for _, s := range []struct {
			name string
			dark bool
		}{{row[2], true}, {row[3], false}} {
			p, err := Lookup(s.name, "")
			if err != nil || p.Name != s.name {
				t.Errorf("%s: no built-in scheme named %q (%q, %v)", row[1], s.name, p.Name, err)
				continue
			}
			if p.Dark != s.dark {
				t.Errorf("%s: %s is dark %v", row[1], s.name, p.Dark)
			}
			requireContrast(t, s.name, "its text", p.Foreground, p.Background, minText)
			requireReadable(t, s.name, FromPalette(p))
		}
	}
}
