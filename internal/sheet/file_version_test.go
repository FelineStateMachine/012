package sheet

import (
	"bytes"
	"strings"
	"testing"
)

// A sheet is written as version 3 only when it uses something version 3
// added (named ranges, frozen panes, a filter), so older builds can still
// open everything else.
func TestFileVersionFollowsFeatures(t *testing.T) {
	version := func(s *Sheet) string {
		var b bytes.Buffer
		if err := s.Write(&b); err != nil {
			t.Fatal(err)
		}
		first := strings.SplitN(b.String(), "\n", 3)[1]
		return strings.TrimSpace(first)
	}
	base := func() *Sheet {
		s := New()
		s.Set(at("A1"), "Item")
		s.Set(at("A2"), "Rent")
		return s
	}
	if v := version(base()); v != `"version": 2,` {
		t.Errorf("plain sheet: %s", v)
	}
	named := base()
	if err := named.DefineName("Items", NewRect(at("A1"), at("A2"))); err != nil {
		t.Fatal(err)
	}
	filtered := base()
	filtered.CreateFilter(NewRect(at("A1"), at("A2")))
	frozen := base()
	frozen.SetFrozen(1, 0)
	for name, s := range map[string]*Sheet{"names": named, "filter": filtered, "freeze": frozen} {
		if v := version(s); v != `"version": 3,` {
			t.Errorf("%s: %s", name, v)
		}
		var b bytes.Buffer
		s.Write(&b)
		if _, err := Read(&b); err != nil {
			t.Errorf("%s: reading back: %v", name, err)
		}
	}
}
