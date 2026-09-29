package diff

import (
	"encoding/json"
	"fmt"
	"strings"
)

// A notebook tab keeps its cells in the file as a list, each with its
// source and what its last run left (see docs/files/format.md). Cells
// have no names of their own to match them by, so comparing lines them
// up by their sources, and merging treats the list of sources as one
// part, whose outputs follow the side it's taken from.

// KindNotebook is a Change to a notebook tab's cell: its source, or
// what its last run left.
const KindNotebook = "notebook"

// nbCell is a notebook cell as the file has it.
type nbCell struct {
	raw    json.RawMessage
	fields map[string]json.RawMessage
	source string
}

func nbCells(raw json.RawMessage) []nbCell {
	var list []json.RawMessage
	json.Unmarshal(raw, &list)
	out := make([]nbCell, len(list))
	for i, e := range list {
		out[i].raw = compact(e)
		out[i].fields, _ = object(e)
		json.Unmarshal(out[i].fields["source"], &out[i].source)
	}
	return out
}

// sources is the part of a notebook merging compares: each cell's kind
// and source, without the outputs runs leave.
func sources(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	var parts []string
	for _, c := range nbCells(raw) {
		parts = append(parts, string(c.fields["kind"])+" "+c.source)
	}
	out, _ := json.Marshal(parts)
	return out
}

// notebookChanges compares two notebooks' cells: those whose sources
// match, in order, by what their runs left; the others as sources
// changed, added or removed.
func notebookChanges(sheetName string, a, b json.RawMessage) []Change {
	ca, cb := nbCells(a), nbCells(b)
	var out []Change
	i, j := 0, 0
	for _, m := range commonSources(ca, cb) {
		out = append(out, unmatched(sheetName, ca[i:m[0]], cb[j:m[1]], i, j)...)
		out = append(out, cellFields(sheetName, m[1], ca[m[0]], cb[m[1]])...)
		i, j = m[0]+1, m[1]+1
	}
	return append(out, unmatched(sheetName, ca[i:], cb[j:], i, j)...)
}

// unmatched compares the cells between two matched ones: side by side
// as sources changed, the rest added or removed. i and j are where they
// start on each side.
func unmatched(sheetName string, a, b []nbCell, i, j int) []Change {
	var out []Change
	for k := 0; k < len(a) || k < len(b); k++ {
		switch {
		case k >= len(a):
			out = append(out, nbChange(sheetName, j+k, "added", nil, b[k].fields["source"]))
		case k >= len(b):
			out = append(out, nbChange(sheetName, i+k, "removed", a[k].fields["source"], nil))
		default:
			out = append(out, cellFields(sheetName, j+k, a[k], b[k])...)
		}
	}
	return out
}

// cellFields compares two cells field by field.
func cellFields(sheetName string, at int, a, b nbCell) []Change {
	var out []Change
	for _, k := range unionKeys(a.fields, b.fields) {
		if !equal(a.fields[k], b.fields[k]) {
			out = append(out, nbChange(sheetName, at, k, a.fields[k], b.fields[k]))
		}
	}
	return out
}

func nbChange(sheetName string, at int, field string, a, b json.RawMessage) Change {
	return rawChange(KindNotebook, sheetName, fmt.Sprintf("cell %d", at+1), field, a, b)
}

// commonSources pairs the cells of a and b whose sources are the same,
// as many as keep their order (a longest common subsequence), by index.
func commonSources(a, b []nbCell) [][2]int {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i].source == b[j].source {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out [][2]int
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case a[i].source == b[j].source:
			out = append(out, [2]int{i, j})
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			i++
		default:
			j++
		}
	}
	return out
}

// dumpNotebook writes a notebook's cells as Dump's lines: the source,
// and the first line of what the last run printed or said.
func dumpNotebook(sheetName string, raw json.RawMessage) []string {
	var out []string
	for i, c := range nbCells(raw) {
		line := fmt.Sprintf("%s cell %d  %s", sheetName, i+1, quoted(c.source))
		var s string
		switch {
		case json.Unmarshal(c.fields["error"], &s) == nil && s != "":
			line += "  error: " + firstLine(s)
		case json.Unmarshal(c.fields["output"], &s) == nil && s != "":
			line += "  = " + firstLine(s)
		}
		out = append(out, line)
	}
	return out
}

// firstLine is s's first line, at most 120 characters of it.
func firstLine(s string) string {
	s, _, cut := strings.Cut(s, "\n")
	if r := []rune(s); len(r) > 120 {
		s, cut = string(r[:120]), true
	}
	if cut {
		s += "…"
	}
	return s
}
