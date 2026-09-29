package diff

import (
	"encoding/json"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// sheets merges the workbooks' sheets. A sheet is matched across the
// three by name or, renamed, by its cells; one removed on one side goes
// when the other left it as it was, and stays (a conflict) when ours
// changed it, or goes (a conflict) when only theirs did. Sheets added
// on both sides under one name merge cell by cell. The order is ours',
// with theirs' additions after the sheet they follow in theirs.
func (m *merger) sheets(b, o, t *rawBook) []*rawSheet {
	oOf, oBase := matchIndex(matchSheets(b, o))
	tOf, tBase := matchIndex(matchSheets(b, t))
	tDone := make([]bool, len(t.sheets))
	var out []*rawSheet
	for j, so := range o.sheets {
		i, inBase := oBase[j]
		switch {
		case !inBase:
			out = append(out, m.added(so, t, tBase, tDone))
		case hasKey(tOf, i):
			tDone[tOf[i]] = true
			out = append(out, m.sheet(b.sheets[i], so, t.sheets[tOf[i]]))
		case sameSheet(b.sheets[i], so):
			// removed in theirs, unchanged in ours: removed
		default:
			m.said("sheet "+so.name, "", "changed in ours, removed in theirs")
			out = append(out, so)
		}
	}
	for k, st := range t.sheets {
		if tDone[k] {
			continue
		}
		i, inBase := tBase[k]
		switch {
		case !inBase:
			out = insertAfter(out, st, t, k)
		case !hasKey(oOf, i) && !sameSheet(b.sheets[i], st):
			m.said("sheet "+st.name, "", "removed in ours, changed in theirs")
		}
	}
	return m.unique(out)
}

// matchIndex is a pairing as maps: a's index to b's, and back.
func matchIndex(p pairing) (toB, toA map[int]int) {
	toB, toA = map[int]int{}, map[int]int{}
	for _, pr := range p.pairs {
		toB[pr[0]], toA[pr[1]] = pr[1], pr[0]
	}
	return toB, toA
}

func hasKey(m map[int]int, k int) bool { _, ok := m[k]; return ok }

// added is a sheet ours added, merged with one theirs added under the
// same name, if any, as if both started empty.
func (m *merger) added(so *rawSheet, t *rawBook, tBase map[int]int, tDone []bool) *rawSheet {
	k := t.sheetIndex(so.name)
	if _, inBase := tBase[k]; k < 0 || inBase || tDone[k] {
		return so
	}
	tDone[k] = true
	return m.sheet(&rawSheet{name: so.name, fields: map[string]json.RawMessage{}, cells: map[string]json.RawMessage{}}, so, t.sheets[k])
}

// insertAfter puts s, theirs' k-th sheet, after the sheet before it in
// theirs, or first.
func insertAfter(out []*rawSheet, s *rawSheet, t *rawBook, k int) []*rawSheet {
	at := 0
	for p := k - 1; p >= 0; p-- {
		if i := indexNamed(out, t.sheets[p].name); i >= 0 {
			at = i + 1
			break
		}
	}
	out = append(out, nil)
	copy(out[at+1:], out[at:])
	out[at] = s
	return out
}

func indexNamed(list []*rawSheet, name string) int {
	for i, s := range list {
		if strings.EqualFold(s.name, name) {
			return i
		}
	}
	return -1
}

// unique renames sheets that came out with one name, ignoring case, as
// when ours renamed a sheet to the name of one theirs added.
func (m *merger) unique(list []*rawSheet) []*rawSheet {
	for i, s := range list {
		if indexNamed(list[:i], s.name) < 0 {
			continue
		}
		name := s.name + " (theirs)"
		for n := 2; indexNamed(list, name) >= 0; n++ {
			name = s.name + " (theirs " + itoa(n) + ")"
		}
		m.said("sheet "+s.name, "name", "a second sheet of this name, from theirs, is named "+name)
		s.name = name
	}
	return list
}

func itoa(n int) string {
	raw, _ := json.Marshal(n)
	return string(raw)
}

func str(s string) json.RawMessage {
	raw, _ := json.Marshal(s)
	return raw
}

// notebook merges a notebook tab's cells as one part: their sources,
// with the outputs of the side taken.
func (m *merger) notebook(sheetName string, b, o, t json.RawMessage) json.RawMessage {
	sb, so, st := sources(b), sources(o), sources(t)
	switch {
	case equal(so, st), equal(sb, st):
		return o
	case equal(sb, so):
		return t
	}
	m.said(sheetName+" notebook", "", "cells changed on both sides")
	return o
}

// sheet merges one sheet: its name, each cell and each field.
func (m *merger) sheet(b, o, t *rawSheet) *rawSheet {
	out := &rawSheet{fields: map[string]json.RawMessage{}, cells: map[string]json.RawMessage{}}
	json.Unmarshal(m.value("sheet "+o.name, "name", str(b.name), str(o.name), str(t.name)), &out.name)
	for _, k := range unionKeys3(b.cells, o.cells, t.cells) {
		if v := m.cell(sheet.QuoteSheet(out.name)+"!"+k, b.cells[k], o.cells[k], t.cells[k]); v != nil {
			out.cells[k] = v
		}
	}
	for _, k := range unionKeys3(b.fields, o.fields, t.fields) {
		var v json.RawMessage
		switch k {
		case "regions":
			v = m.list(b.fields[k], o.fields[k], t.fields[k], func(key string) string { return out.name + " region " + key })
		case "tables":
			v = m.list(b.fields[k], o.fields[k], t.fields[k], func(key string) string { return out.name + " table " + key })
		case "notebookCells":
			v = m.notebook(out.name, b.fields[k], o.fields[k], t.fields[k])
		case "widths", "heights", "lines":
			v = m.object(b.fields[k], o.fields[k], t.fields[k], func(key string) (string, string) { return out.name + " " + k, key })
		default:
			v = m.value(out.name+" "+k, "", b.fields[k], o.fields[k], t.fields[k])
		}
		if v != nil {
			out.fields[k] = v
		}
	}
	return out
}
