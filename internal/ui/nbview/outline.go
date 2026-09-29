package nbview

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// Entry is a place in the notebook to jump to: a heading of a note, or
// a cell.
type Entry struct {
	Cell  int    // the cell's index
	Level int    // a heading's level, 1 for #; 0 for a cell
	Title string // the heading's text, or the cell's first line
}

// Outline is the notebook's headings in order, from its notes'
// Markdown: its table of contents. With cells, every cell is there too,
// a code cell by its first line and a note by its first heading or line.
func (v *View) Outline(cells bool) []Entry {
	var out []Entry
	for i, c := range v.h.Cells() {
		heads := headings(c)
		for _, h := range heads {
			out = append(out, Entry{Cell: i, Level: h.Level, Title: h.Title})
		}
		if cells && (c.Kind == notebook.Code || len(heads) == 0) {
			out = append(out, Entry{Cell: i, Title: firstLine(c.Source)})
		}
	}
	return out
}

// headings are a note's Markdown headings, outside code fences.
func headings(c notebook.Cell) []Entry {
	if c.Kind != notebook.Note {
		return nil
	}
	var out []Entry
	fenced := false
	for line := range strings.SplitSeq(c.Source, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			fenced = !fenced
		}
		level := len(t) - len(strings.TrimLeft(t, "#"))
		if fenced || level == 0 || level > 6 || len(t) > level && t[level] != ' ' {
			continue
		}
		if title := strings.TrimSpace(t[level:]); title != "" {
			out = append(out, Entry{Level: level, Title: title})
		}
	}
	return out
}

// firstLine is a source's first line that isn't blank, shortened.
func firstLine(src string) string {
	for line := range strings.SplitSeq(src, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return ansi.Truncate(t, 56, "…")
		}
	}
	return "(empty)"
}
