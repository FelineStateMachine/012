package locale_test

import (
	"testing"

	"github.com/FelineStateMachine/012/internal/locale"
)

func TestLookup(t *testing.T) {
	for in, want := range map[string]string{
		"de-DE": "de-DE", "de_de": "de-DE", "DE-de": "de-DE", " en-GB ": "en-GB",
		"de": "de-DE", "fr": "fr-FR", "xx-YY": "", "": "", "de-AT": "",
	} {
		l, ok := locale.Lookup(in)
		if got := ""; ok {
			got = l.Tag
			if got != want {
				t.Errorf("Lookup(%q) = %s, want %q", in, got, want)
			}
		} else if want != "" {
			t.Errorf("Lookup(%q) found nothing, want %s", in, want)
		}
	}
}

func TestFromPOSIX(t *testing.T) {
	for in, want := range map[string]string{
		"de_DE.UTF-8": "de-DE", "fr_FR@euro": "fr-FR", "en_US.UTF-8": "en-US", "C": "", "POSIX": "",
		"C.UTF-8": "", "": "", "xx_YY.UTF-8": "", "pt_BR": "pt-BR",
	} {
		if got := locale.FromPOSIX(in); got != want {
			t.Errorf("FromPOSIX(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestTable checks every locale is whole: a separator a formula can tell
// from its arguments, a date pattern in its own order, and tags that
// look themselves up.
func TestTable(t *testing.T) {
	seen := map[string]bool{}
	for _, l := range locale.All() {
		if seen[l.Tag] {
			t.Errorf("%s twice", l.Tag)
		}
		seen[l.Tag] = true
		if got, ok := locale.Lookup(l.Tag); !ok || got != l {
			t.Errorf("%s doesn't look itself up", l.Tag)
		}
		if l.Decimal != '.' && l.Decimal != ',' {
			t.Errorf("%s: decimal %q", l.Tag, l.Decimal)
		}
		if l.ArgSep() == l.Decimal || l.ColSep() == l.Decimal || l.Group == string(l.Decimal) {
			t.Errorf("%s: separators clash with the decimal %q", l.Tag, l.Decimal)
		}
		if l.Name == "" || l.Currency == "" || l.Date == "" || l.Time == "" {
			t.Errorf("%s is missing a field", l.Tag)
		}
	}
	if !locale.Canonical.IsCanonical() || locale.Canonical.Tag != "en-US" {
		t.Error("en-US isn't canonical")
	}
	if got := locale.Canonical.DateTime(); got != "m/d/yyyy h:mm:ss" {
		t.Errorf("en-US date time %q", got)
	}
}
