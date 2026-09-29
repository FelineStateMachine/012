package diff

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Conflict is a part of a workbook both sides changed differently. The
// merge keeps ours; the report says what theirs had.
type Conflict struct {
	Where string // as a Change's: Q3!B7, sheet Q3, Q3 widths, name Sales
	Field string // what about it: input, format, note, a field's key, or ""
	// Base, Ours and Theirs are the three versions as text, "" where
	// the thing isn't there; What says what happened instead, when the
	// conflict is about a whole sheet.
	Base, Ours, Theirs string
	What               string
}

// String is the conflict as the merge report lists it.
func (c Conflict) String() string {
	s := c.Where
	if c.Field != "" {
		s += " " + c.Field
	}
	if c.What != "" {
		return s + ": " + c.What
	}
	return s + ": ours " + side3(c.Ours) + ", theirs " + side3(c.Theirs) + ", base " + side3(c.Base)
}

func side3(s string) string {
	if s == "" {
		return "(none)"
	}
	return quoted(s)
}

// Merge combines ours and theirs, two workbook files changed from base,
// and returns the merged file as 012 saves it. Each cell's input, format
// and note, each field of a sheet or the workbook (a column's width, a
// region, a named range, a macro) and each sheet's name and presence
// merge on their own: a part changed on one side only takes that side's
// version, and a part both changed alike takes it too. A part both
// changed differently is a conflict: the merge keeps ours, and a cell's
// note says what theirs had (on top of its own note), so the conflicts
// can be found in the grid. Empty contents stand for a missing file.
//
// Sheets are matched by name or, when renamed, by their cells (see
// matchSheets). Rows or columns inserted on one side shift the cells
// below or right of them, which a cell-by-cell merge can't tell from
// edits: merge such changes in 012 rather than here.
func Merge(base, ours, theirs []byte) ([]byte, []Conflict, error) {
	var books [3]*rawBook
	for i, data := range [][]byte{base, ours, theirs} {
		b, err := parseRaw(data)
		if err != nil {
			return nil, nil, err
		}
		books[i] = b
	}
	m := &merger{}
	merged := m.books(books[0], books[1], books[2])
	if len(merged.sheets) == 0 {
		return nil, m.conflicts, nil
	}
	out, err := normalize(merged)
	return out, m.conflicts, err
}

// merger collects the conflicts of a merge.
type merger struct {
	conflicts []Conflict
}

func (m *merger) conflict(where, field string, b, o, t json.RawMessage) {
	m.conflicts = append(m.conflicts, Conflict{Where: where, Field: field, Base: rawText(b), Ours: rawText(o), Theirs: rawText(t)})
}

// said records a conflict that isn't between three values, in words.
func (m *merger) said(where, field, what string) {
	m.conflicts = append(m.conflicts, Conflict{Where: where, Field: field, What: what})
}

// merge3 merges one value: what changed on one side, or both alike.
func merge3(b, o, t json.RawMessage) (json.RawMessage, bool) {
	switch {
	case equal(o, t), equal(b, t):
		return o, false
	case equal(b, o):
		return t, false
	}
	return o, true
}

// value merges one value, reporting a conflict at where.
func (m *merger) value(where, field string, b, o, t json.RawMessage) json.RawMessage {
	v, clash := merge3(b, o, t)
	if clash {
		m.conflict(where, field, b, o, t)
	}
	return v
}

// books merges the workbooks' fields, then their sheets.
func (m *merger) books(b, o, t *rawBook) *rawBook {
	out := &rawBook{top: map[string]json.RawMessage{}}
	for _, k := range unionKeys3(b.top, o.top, t.top) {
		var v json.RawMessage
		switch k {
		case "version":
			v = maxVersion(b.top[k], o.top[k], t.top[k])
		case "names":
			v = m.object(b.top[k], o.top[k], t.top[k], func(key string) (string, string) { return "name " + key, "" })
		case "macros":
			v = m.list(b.top[k], o.top[k], t.top[k], func(key string) string { return "macro " + key })
		case "active", "shellHistory", "macroOrigin":
			v, _ = merge3(b.top[k], o.top[k], t.top[k]) // what's shown or was typed: ours if both differ
		default:
			v = m.value("workbook "+k, "", b.top[k], o.top[k], t.top[k])
		}
		if v != nil {
			out.top[k] = v
		}
	}
	out.sheets = m.sheets(b, o, t)
	if n, err := strconv.Atoi(string(out.top["active"])); err == nil && n >= len(out.sheets) {
		delete(out.top, "active") // the sheet shown went: show the first
	}
	trust(out, o, t)
	return out
}

// trust keeps the computer ours trusted its commands on only when the
// merge's commands (macros, regions' commands and files) are ours', or
// theirs were trusted on the same computer, so a merge never makes
// someone else's commands run without asking.
func trust(out, o, t *rawBook) {
	if equal(o.top["macroOrigin"], t.top["macroOrigin"]) || code(out) == code(o) {
		if origin := o.top["macroOrigin"]; origin != nil {
			out.top["macroOrigin"] = origin
		}
		return
	}
	delete(out.top, "macroOrigin")
}

// code is what a workbook can make 012 run or read: its macros and its
// regions' commands and files, in one string to compare.
func code(b *rawBook) string {
	var parts []string
	parts = append(parts, string(b.top["macros"]))
	for _, s := range b.sheets {
		for _, r := range named(s.fields["regions"]) {
			f, _ := object(r)
			parts = append(parts, string(f["command"])+string(f["path"])+string(f["input"])+string(f["query"]))
		}
	}
	slices.Sort(parts)
	raw, _ := json.Marshal(parts)
	return string(raw)
}

func maxVersion(vs ...json.RawMessage) json.RawMessage {
	best, out := 0, json.RawMessage(nil)
	for _, v := range vs {
		if n, err := strconv.Atoi(string(v)); err == nil && n > best {
			best, out = n, v
		}
	}
	return out
}

// object merges an object key by key (named ranges, column widths);
// where names each key's place and field in conflicts.
func (m *merger) object(b, o, t json.RawMessage, where func(key string) (string, string)) json.RawMessage {
	ob, okB := object(b)
	oo, okO := object(o)
	ot, okT := object(t)
	if !okB || !okO || !okT {
		at, _ := where("")
		return m.value(at, "", b, o, t)
	}
	out := map[string]json.RawMessage{}
	for _, k := range unionKeys3(ob, oo, ot) {
		at, field := where(k)
		if v := m.value(at, field, ob[k], oo[k], ot[k]); v != nil {
			out[k] = v
		}
	}
	if len(out) == 0 && o == nil && t == nil {
		return nil
	}
	raw, _ := json.Marshal(out)
	return raw
}

// list merges a list of things with names (regions, macros) thing by
// thing, in ours' order with theirs' additions after.
func (m *merger) list(b, o, t json.RawMessage, where func(string) string) json.RawMessage {
	lb, lo, lt := named(b), named(o), named(t)
	var order []string
	for _, raw := range []json.RawMessage{o, t} {
		var items []struct{ Name string }
		json.Unmarshal(raw, &items)
		for _, it := range items {
			if !slices.Contains(order, it.Name) {
				order = append(order, it.Name)
			}
		}
	}
	var out []json.RawMessage
	for _, k := range order {
		if v := m.value(where(k), "", lb[k], lo[k], lt[k]); v != nil {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	raw, _ := json.Marshal(out)
	return raw
}

// cell merges a cell's input, format and note apart. Where they
// conflict, ours stays and the note says what theirs had.
func (m *merger) cell(where string, b, o, t json.RawMessage) json.RawMessage {
	pb, po, pt := splitCell(b), splitCell(o), splitCell(t)
	str := func(s string) json.RawMessage {
		if s == "" {
			return nil
		}
		raw, _ := json.Marshal(s)
		return raw
	}
	out := cellParts{}
	var said []string
	part := func(field string, b, o, t json.RawMessage) json.RawMessage {
		v, clash := merge3(b, o, t)
		if clash {
			text := rawText
			if field == "format" {
				text = formatText
			}
			m.conflicts = append(m.conflicts, Conflict{Where: where, Field: field, Base: text(b), Ours: text(o), Theirs: text(t)})
			said = append(said, field+" "+side3(text(t)))
		}
		return v
	}
	json.Unmarshal(part("input", str(pb.input), str(po.input), str(pt.input)), &out.input)
	out.format = part("format", pb.format, po.format, pt.format)
	json.Unmarshal(part("note", str(pb.note), str(po.note), str(pt.note)), &out.note)
	if len(said) > 0 {
		out.note = conflictNote(out.note, said)
	}
	return joinCell(out)
}

// conflictNote is a cell's note with what theirs had added below it.
func conflictNote(note string, said []string) string {
	text := "Merge conflict, kept ours; theirs had "
	for i, s := range said {
		if i > 0 {
			text += "; "
		}
		text += s
	}
	if note != "" {
		text = note + "\n\n" + text
	}
	return sheet.CleanNote(text)
}

func unionKeys3(a, b, c map[string]json.RawMessage) []string {
	keys := map[string]bool{}
	for _, m := range []map[string]json.RawMessage{a, b, c} {
		for k := range m {
			keys[k] = true
		}
	}
	return slices.Sorted(maps.Keys(keys))
}
