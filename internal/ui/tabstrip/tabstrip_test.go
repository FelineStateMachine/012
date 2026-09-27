package tabstrip

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// book returns a workbook of n sheets named Sheet1, Sheet2, ...
func book(t *testing.T, n int) []*sheet.Sheet {
	t.Helper()
	s := sheet.New()
	for i := 2; i <= n; i++ {
		if _, err := s.Book().AddSheet("Sheet"+strconv.Itoa(i), i-1); err != nil {
			t.Fatal(err)
		}
	}
	return s.Book().Sheets()
}

func kinds(spans []Span) string {
	names := map[Kind]string{Tab: "tab", Add: "+", Prev: "‹", Next: "›"}
	var out []string
	for _, sp := range spans {
		out = append(out, names[sp.Kind])
	}
	return strings.Join(out, " ")
}

func TestEveryTabFits(t *testing.T) {
	th := theme.New(true)
	var s Strip
	v := View{Sheets: book(t, 3), Active: 1}
	line, spans := s.Layout(&th, v, 80)
	if got := ansi.Strip(line); got != " Sheet1   Sheet2   Sheet3   + " {
		t.Errorf("strip %q", got)
	}
	if kinds(spans) != "tab tab tab +" || ansi.StringWidth(line) != v.FullWidth() {
		t.Errorf("spans %s, width %d of %d", kinds(spans), ansi.StringWidth(line), v.FullWidth())
	}
	for _, sp := range spans {
		if got := ansi.Strip(line)[sp.X : sp.X+sp.W]; sp.Kind == Tab && got != Label(v.Sheets[sp.Index]) {
			t.Errorf("span %+v covers %q", sp, got)
		}
	}
}

func TestScrollKeepsTheActiveTab(t *testing.T) {
	th := theme.New(true)
	var s Strip
	v := View{Sheets: book(t, 8), Active: 0}
	room := v.MinWidth() + 12
	_, spans := s.Layout(&th, v, room)
	if k := kinds(spans); !strings.HasPrefix(k, "tab") || !strings.Contains(k, "›") {
		t.Errorf("first tab shown: %s", k)
	}
	v.Active = 7
	line, spans := s.Layout(&th, v, room)
	if k := kinds(spans); !strings.HasPrefix(k, "‹") || strings.Contains(k, "›") {
		t.Errorf("last tab shown: %s", k)
	}
	if !strings.Contains(ansi.Strip(line), "Sheet8") || ansi.StringWidth(line) > room {
		t.Errorf("strip %q in %d", ansi.Strip(line), room)
	}
	// Going back one tab keeps the strip where it is.
	v.Active = 6
	again, _ := s.Layout(&th, v, room)
	if ansi.Strip(again) != ansi.Strip(line) {
		t.Errorf("strip moved: %q then %q", ansi.Strip(line), ansi.Strip(again))
	}
}

func TestHoverStyles(t *testing.T) {
	th := theme.New(true)
	var s Strip
	v := View{Sheets: book(t, 2), Active: 0, Hover: Tab, HoverIndex: 1}
	hovered, _ := s.Layout(&th, v, 80)
	v.Busy = true
	busy, _ := s.Layout(&th, v, 80)
	if hovered == busy {
		t.Error("hovering a tab looks the same while dragging something else")
	}
	if want := th.TabHover.Render(Label(v.Sheets[1])); !strings.Contains(hovered, want) {
		t.Errorf("hovered tab not highlighted: %q", hovered)
	}
}
