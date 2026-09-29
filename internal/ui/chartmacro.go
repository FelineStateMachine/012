package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/macro"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Charts in macros. A chart inserted is recorded as
// run("insert.chart", answer={...}) with the whole chart as a line of the
// file writes it; a chart edited, moved or resized as
// run("chart.edit", answer={"chart": 2, "at": "F3"}), its number
// (counting from 1) and the fields that changed, a field left out of the
// file's line given as its default (False, "" or None). Scripts answer
// both commands the same way, and a replay changes only the fields
// named. Moving a chart a step at a time records one call.

// jsonField is a field of a JSON object, in the order written.
type jsonField struct {
	key   string
	value json.RawMessage
}

// jsonFields reads a JSON object's fields in order.
func jsonFields(text string) ([]jsonField, error) {
	d := json.NewDecoder(bytes.NewReader([]byte(text)))
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("the answer isn't an object: give a dict")
	}
	var out []jsonField
	for d.More() {
		k, err := d.Token()
		if err != nil {
			return nil, err
		}
		var v json.RawMessage
		if err := d.Decode(&v); err != nil {
			return nil, err
		}
		out = append(out, jsonField{k.(string), v})
	}
	return out, nil
}

// jsonObject writes fields as a JSON object.
func jsonObject(fields []jsonField) string {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range fields {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(f.key)
		b.Write(k)
		b.WriteByte(':')
		b.Write(f.value)
	}
	b.WriteByte('}')
	return b.String()
}

func fieldIndex(fields []jsonField, key string) int {
	return slices.IndexFunc(fields, func(f jsonField) bool { return f.key == key })
}

// chartChanges are the fields of after's line that differ from before's.
// A field before's line has and after's leaves out, at its default, is
// given as that default: the other boolean, "", or null.
func chartChanges(before, after sheet.Chart) []jsonField {
	bf, _ := jsonFields(before.JSON())
	af, _ := jsonFields(after.JSON())
	var out []jsonField
	for _, f := range af {
		if i := fieldIndex(bf, f.key); i < 0 || !bytes.Equal(bf[i].value, f.value) {
			out = append(out, f)
		}
	}
	for _, f := range bf {
		if fieldIndex(af, f.key) >= 0 {
			continue
		}
		def := json.RawMessage("null")
		switch f.value[0] {
		case 't':
			def = json.RawMessage("false")
		case 'f':
			def = json.RawMessage("true")
		case '"':
			def = json.RawMessage(`""`)
		}
		out = append(out, jsonField{f.key, def})
	}
	return out
}

// chartWith is c with the fields of a macro's answer: null leaves a
// field at its default; "chart" says which chart, and isn't a field.
func chartWith(c sheet.Chart, answer []jsonField) (sheet.Chart, error) {
	fields, _ := jsonFields(c.JSON())
	for _, f := range answer {
		i := fieldIndex(fields, f.key)
		switch {
		case f.key == "chart":
		case string(f.value) == "null" && i >= 0:
			fields = slices.Delete(fields, i, i+1)
		case string(f.value) == "null":
		case i >= 0:
			fields[i].value = f.value
		default:
			fields = append(fields, f)
		}
	}
	return sheet.ParseChart(jsonObject(fields))
}

// recordChart records chart i inserted or changed from before to after,
// as the command id that does it answered with the chart. A chart placed
// (moved or resized while selected, not by a command) comes after the
// selection it follows.
func (m *Model) recordChart(id string, i int, before, after sheet.Chart, placed bool) {
	r := m.rec
	if r == nil || r.depth > 0 {
		return
	}
	if placed {
		r.flush(m)
	}
	if id == "insert.chart" {
		m.recordDialog(id, after.JSON())
		return
	}
	changes := chartChanges(before, after)
	if len(changes) == 0 {
		return
	}
	n := json.RawMessage(strconv.Itoa(i + 1))
	if last, ok := r.lastPlacement(i + 1); ok && placed && placement(changes) {
		for _, f := range changes {
			if k := fieldIndex(last, f.key); k >= 0 {
				last[k].value = f.value
			} else {
				last = append(last, f)
			}
		}
		r.actions[len(r.actions)-1] = macro.Call("run", id).With("answer", macro.JSON(jsonObject(last)))
		r.acted = true
		return
	}
	m.recordDialog(id, jsonObject(append([]jsonField{{"chart", n}}, changes...)))
}

// placement reports whether fields only place a chart: where it is and
// its size.
func placement(fields []jsonField) bool {
	for _, f := range fields {
		switch f.key {
		case "chart", "at", "width", "height":
		default:
			return false
		}
	}
	return true
}

// lastPlacement is the answer of the last action recorded when it placed
// chart n, so that moving a chart step by step records one call.
func (r *recorder) lastPlacement(n int) ([]jsonField, bool) {
	if len(r.actions) == 0 {
		return nil, false
	}
	a := r.actions[len(r.actions)-1]
	if a.Func != "run" || len(a.Args) != 1 || a.Args[0] != "chart.edit" || len(a.Named) != 1 {
		return nil, false
	}
	text, ok := a.Named[0].Value.(macro.JSON)
	if !ok {
		return nil, false
	}
	fields, err := jsonFields(string(text))
	if err != nil || !placement(fields) {
		return nil, false
	}
	if i := fieldIndex(fields, "chart"); i < 0 || string(fields[i].value) != strconv.Itoa(n) {
		return nil, false
	}
	return fields, true
}

// answerInsertChart inserts a chart as a script answers Insert > Chart:
// the chart the selection would make, with the answer's fields.
func (m *Model) answerInsertChart(text string) (tea.Cmd, error) {
	fields, err := jsonFields(text)
	if err != nil {
		return nil, err
	}
	m.insertChart()
	e, ok := m.overlay.(*chartEditor)
	if !ok {
		return nil, errors.New(m.note)
	}
	return nil, e.answer(fields)
}

// answerEditChart changes a chart as a script answers Edit chart: the
// chart numbered "chart", or the one the command would edit, with the
// answer's fields.
func (m *Model) answerEditChart(text string) (tea.Cmd, error) {
	fields, err := jsonFields(text)
	if err != nil {
		return nil, err
	}
	charts := m.sheet.Charts()
	i := m.targetChart()
	if k := fieldIndex(fields, "chart"); k >= 0 {
		n, err := strconv.Atoi(string(fields[k].value))
		if err != nil || n < 1 || n > len(charts) {
			return nil, fmt.Errorf("there's no chart %s on %s: it has %d", fields[k].value, m.sheet.Name(), len(charts))
		}
		i = n - 1
	}
	if i < 0 {
		return nil, fmt.Errorf("there's no chart on %s", m.sheet.Name())
	}
	return nil, m.openChartEditor(i, false, m.book().Checkpoint()).answer(fields)
}

// answer makes the chart the answer's fields say and closes the editor.
func (e *chartEditor) answer(fields []jsonField) error {
	c, err := chartWith(e.chart(), fields)
	if err != nil {
		e.m.closeOverlay()
		return err
	}
	e.set(func(x *sheet.Chart) { *x = c })
	e.m.closeOverlay()
	return nil
}
