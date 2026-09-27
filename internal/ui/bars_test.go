package ui

import (
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// mustFormat adds a conditional format written as a line of the file.
func mustFormat(t *testing.T, m *Model, line string) {
	t.Helper()
	f, err := sheet.ParseCondFormat(line)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.sheet.AddCondFormat(f); err != nil {
		t.Fatal(err)
	}
}

// Data bars draw in eighth blocks as long as their numbers, icons at
// the cells' left, and a dropdown shown as a chip holds its value
// between rounded ends; each reads in characters.
func TestBarsIconsAndChips(t *testing.T) {
	m := newModel()
	for i, v := range []string{"25", "50", "100"} {
		m.sheet.Set(sheet.Addr{Row: i}, v)
		m.sheet.Set(sheet.Addr{Col: 1, Row: i}, v)
	}
	m.sheet.Set(addr("C1"), "Ann")
	mustFormat(t, m, `{"ranges":"A1:A3","dataBar":{"color":"blue","min":{"type":"min"},"max":{"type":"max"}}}`)
	mustFormat(t, m, `{"ranges":"B1:B3","iconSet":{"icons":"arrows","points":[{"type":"percent","value":"33"},{"type":"percent","value":"67"}]}}`)
	mustValidate(t, m, `{"ranges":"C1:C3","criteria":"list","items":["Ann","Bo"],"display":"chip"}`)
	m.cur = addr("E5")
	g := gridLines(m)
	colOf := func(line string, col int) string {
		r := []rune(line)
		from := m.hdrW() + col*sheet.DefaultWidth
		if from >= len(r) {
			return ""
		}
		return string(r[from:min(from+sheet.DefaultWidth, len(r))])
	}
	// A1 is a quarter of the way: two of its nine columns and a quarter.
	if a1 := colOf(g[0], 0); !strings.HasPrefix(a1, "██▎") || !strings.HasSuffix(strings.TrimRight(a1, " "), "25") {
		t.Errorf("A1's bar %q", a1)
	}
	if a3 := colOf(g[2], 0); strings.Count(a3, "█") < 5 || !strings.Contains(a3, "100") {
		t.Errorf("A3's bar %q", a3)
	}
	for row, icon := range []string{"↓", "→", "↑"} {
		if b := colOf(g[row], 1); !strings.HasPrefix(b, " "+icon) {
			t.Errorf("B%d's icon %q", row+1, b)
		}
	}
	if c1 := colOf(g[0], 2); !strings.HasPrefix(c1, " ▐Ann ▾▌") {
		t.Errorf("C1's chip %q", c1)
	}
	if c2 := colOf(g[1], 2); !strings.Contains(c2, "▾") {
		t.Errorf("a blank chip cell shows its ▾: %q", c2)
	}
	// Text under a bar is in reverse video, the bar's color as its
	// background; without color that still reads.
	m.sheet.SetColWidth(0, 4)
	m.sheet.Set(addr("A3"), "999")
	if cells := cellAt(m, addr("A3")); !cells[1].reverse {
		t.Errorf("text under the bar: %+v", cells)
	}
}
