package sheet

import (
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/functions"
)

// Link returns the address a cell links to: its text when that is a URL
// (http, https or mailto), or the target of a HYPERLINK formula. It is
// empty for other cells.
func (s *Sheet) Link(a Addr) string {
	c := s.cells.get(a)
	if c == nil || c.Value.Kind != Text {
		return ""
	}
	if call, ok := c.expr.(formula.Call); ok && funcOf(call).Name == "HYPERLINK" && c.IsFormula() {
		v := functions.Eval(call.Args[0], s.wb.values(s).lib)
		if v.Kind == Error {
			return ""
		}
		return linkTarget(text(v), true)
	}
	return linkTarget(c.Value.Str, false)
}

// linkTarget returns s as a link, or "" if it isn't one. Text in a cell
// must be a whole URL with a scheme; HYPERLINK also takes a bare domain,
// as Sheets does, and links it over https.
func linkTarget(s string, bare bool) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, " \t\n\x1b\x07") {
		return ""
	}
	lower := strings.ToLower(s)
	for _, scheme := range []string{"https://", "http://", "mailto:"} {
		if strings.HasPrefix(lower, scheme) {
			if len(s) == len(scheme) {
				return ""
			}
			return s
		}
	}
	if bare && strings.Contains(s, ".") && !strings.Contains(s, ":") {
		return "https://" + s
	}
	return ""
}
