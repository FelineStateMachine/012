package macro

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"go.starlark.net/starlark"

	"github.com/FelineStateMachine/012/internal/formula"
)

// The functions scripts call. Each checks its arguments, then makes one
// call to the Host through Env.Do. docs/macros.md documents them; keep
// the two in step.

type builtinFn func(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)

func (r *Run) builtins() starlark.StringDict {
	fns := map[string]builtinFn{
		"get": r.get, "get_formula": r.getFormula, "set": r.set, "set_formula": r.setFormula,
		"clear": r.clear, "number_format": r.numberFormat, "get_number_format": r.getNumberFormat,
		"selection": r.selection, "active_cell": r.activeCell, "select": r.selectRange,
		"move": r.move, "extend": r.extend, "jump": r.jump, "enter": r.enter, "paste_text": r.pasteText,
		"sheets": r.sheets, "active_sheet": r.activeSheet, "activate_sheet": r.activateSheet,
		"add_sheet": r.addSheet, "move_sheet": r.moveSheet,
		"run": r.run, "set_width": r.setWidth, "fill": r.fill, "offset": offset,
	}
	out := make(starlark.StringDict, len(fns))
	for name, fn := range fns {
		out[name] = starlark.NewBuiltin(name, func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			return fn(args, kwargs)
		})
	}
	return out
}

// host makes one call to the Host where it may be used.
func (r *Run) host(fn func(h Host) error) error {
	r.calls++
	var err error
	r.env.Do(func() { err = fn(r.env.Host) })
	return err
}

// unpack reads arguments for the function whose name leads pairs'
// error messages; see starlark.UnpackArgs.
func unpack(fn string, args starlark.Tuple, kwargs []starlark.Tuple, pairs ...any) error {
	return starlark.UnpackArgs(fn, args, kwargs, pairs...)
}

func (r *Run) get(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ref string
	if err := unpack("get", args, kwargs, "ref", &ref); err != nil {
		return nil, err
	}
	var rows [][]any
	if err := r.host(func(h Host) (err error) { rows, err = h.Values(ref); return }); err != nil {
		return nil, err
	}
	return shape(ref, rows, toValue), nil
}

func (r *Run) getFormula(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ref string
	if err := unpack("get_formula", args, kwargs, "ref", &ref); err != nil {
		return nil, err
	}
	var rows [][]string
	if err := r.host(func(h Host) (err error) { rows, err = h.Formulas(ref); return }); err != nil {
		return nil, err
	}
	return shape(ref, rows, func(s string) starlark.Value { return starlark.String(s) }), nil
}

// shape returns one value for a single cell, or a list of rows.
func shape[T any](ref string, rows [][]T, conv func(T) starlark.Value) starlark.Value {
	if len(rows) == 1 && len(rows[0]) == 1 && !strings.Contains(ref, ":") {
		return conv(rows[0][0])
	}
	out := make([]starlark.Value, len(rows))
	for i, row := range rows {
		vals := make([]starlark.Value, len(row))
		for j, v := range row {
			vals[j] = conv(v)
		}
		out[i] = starlark.NewList(vals)
	}
	return starlark.NewList(out)
}

// toValue converts a cell's value: whole numbers become ints, as they
// read in a sheet.
func toValue(v any) starlark.Value {
	switch v := v.(type) {
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1<<53 {
			return starlark.MakeInt64(int64(v))
		}
		return starlark.Float(v)
	case string:
		return starlark.String(v)
	case bool:
		return starlark.Bool(v)
	}
	return starlark.None
}

// input is a value as it would be typed into a cell.
func input(fn string, v starlark.Value) (string, error) {
	switch v := v.(type) {
	case starlark.NoneType:
		return "", nil
	case starlark.String:
		return string(v), nil
	case starlark.Bool:
		if v {
			return "TRUE", nil
		}
		return "FALSE", nil
	case starlark.Int:
		return v.String(), nil
	case starlark.Float:
		f := float64(v)
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return "", fmt.Errorf("%s: %v can't go in a cell", fn, v)
		}
		return strconv.FormatFloat(f, 'g', -1, 64), nil
	}
	return "", fmt.Errorf("%s: a cell takes a number, text, True, False or None, not %s", fn, v.Type())
}

func (r *Run) set(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ref string
	var v starlark.Value
	if err := unpack("set", args, kwargs, "ref", &ref, "value", &v); err != nil {
		return nil, err
	}
	if list, ok := v.(*starlark.List); ok {
		rows, err := inputRows(list)
		if err != nil {
			return nil, err
		}
		return starlark.None, r.host(func(h Host) error { return h.SetInputs(ref, rows) })
	}
	in, err := input("set", v)
	if err != nil {
		return nil, err
	}
	return starlark.None, r.host(func(h Host) error { return h.SetInput(ref, in) })
}

// inputRows reads a list of rows, each a list of values.
func inputRows(list *starlark.List) ([][]string, error) {
	rows := make([][]string, list.Len())
	for i := range list.Len() {
		row, ok := list.Index(i).(*starlark.List)
		if !ok {
			return nil, fmt.Errorf("set: a range takes a list of rows, e.g. [[1, 2], [3, 4]]")
		}
		for j := range row.Len() {
			in, err := input("set", row.Index(j))
			if err != nil {
				return nil, err
			}
			rows[i] = append(rows[i], in)
		}
	}
	return rows, nil
}

func (r *Run) setFormula(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ref, f string
	if err := unpack("set_formula", args, kwargs, "ref", &ref, "formula", &f); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(f, "=") {
		f = "=" + f
	}
	return starlark.None, r.host(func(h Host) error { return h.SetFormula(ref, f) })
}

func (r *Run) clear(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ref string
	if err := unpack("clear", args, kwargs, "ref?", &ref); err != nil {
		return nil, err
	}
	return starlark.None, r.host(func(h Host) error { return h.Clear(ref) })
}

func (r *Run) numberFormat(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ref, kind, pattern string
	var decimals starlark.Value = starlark.None
	if err := unpack("number_format", args, kwargs, "ref", &ref, "kind", &kind, "decimals?", &decimals, "pattern?", &pattern); err != nil {
		return nil, err
	}
	d := -1
	if decimals != starlark.None {
		var err error
		if d, err = starlark.AsInt32(decimals); err != nil || d < 0 {
			return nil, fmt.Errorf("number_format: decimals is a whole number from 0, not %s", decimals)
		}
	}
	return starlark.None, r.host(func(h Host) error { return h.SetNumberFormat(ref, kind, d, pattern) })
}

func (r *Run) getNumberFormat(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ref, kind string
	if err := unpack("get_number_format", args, kwargs, "ref", &ref); err != nil {
		return nil, err
	}
	err := r.host(func(h Host) (err error) { kind, _, err = h.NumberFormat(ref); return })
	return starlark.String(kind), err
}

func (r *Run) selection(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := unpack("selection", args, kwargs); err != nil {
		return nil, err
	}
	var sel string
	r.host(func(h Host) error { sel, _ = h.Selection(); return nil })
	return starlark.String(sel), nil
}

func (r *Run) activeCell(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := unpack("active_cell", args, kwargs); err != nil {
		return nil, err
	}
	var active string
	r.host(func(h Host) error { _, active = h.Selection(); return nil })
	return starlark.String(active), nil
}

func (r *Run) selectRange(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ref, active string
	if err := unpack("select", args, kwargs, "ref", &ref, "active?", &active); err != nil {
		return nil, err
	}
	return starlark.None, r.host(func(h Host) error { return h.Select(ref, active) })
}

func (r *Run) move(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var cols, rows int
	if err := unpack("move", args, kwargs, "cols?", &cols, "rows?", &rows); err != nil {
		return nil, err
	}
	return starlark.None, r.host(func(h Host) error { return h.Move(cols, rows) })
}

func (r *Run) extend(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var cols, rows int
	var whole string
	if err := unpack("extend", args, kwargs, "cols?", &cols, "rows?", &rows, "whole?", &whole); err != nil {
		return nil, err
	}
	if whole != "" && whole != "columns" && whole != "rows" {
		return nil, fmt.Errorf("extend: whole is \"columns\" or \"rows\", not %q", whole)
	}
	return starlark.None, r.host(func(h Host) error { return h.Extend(cols, rows, whole) })
}

func (r *Run) jump(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var to string
	var ext bool
	if err := unpack("jump", args, kwargs, "to", &to, "extend?", &ext); err != nil {
		return nil, err
	}
	if !slices.Contains(JumpTargets, to) {
		return nil, fmt.Errorf("jump: to is one of %s, not %q", strings.Join(JumpTargets, ", "), to)
	}
	return starlark.None, r.host(func(h Host) error { return h.Jump(to, ext) })
}

func (r *Run) enter(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var v starlark.Value
	var fill bool
	if err := unpack("enter", args, kwargs, "text", &v, "fill?", &fill); err != nil {
		return nil, err
	}
	text, err := input("enter", v)
	if err != nil {
		return nil, err
	}
	return starlark.None, r.host(func(h Host) error { return h.Enter(text, fill) })
}

func (r *Run) pasteText(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var text string
	if err := unpack("paste_text", args, kwargs, "text", &text); err != nil {
		return nil, err
	}
	return starlark.None, r.host(func(h Host) error { return h.PasteText(text) })
}

func (r *Run) sheets(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := unpack("sheets", args, kwargs); err != nil {
		return nil, err
	}
	var names []string
	r.host(func(h Host) error { names = h.Sheets(); return nil })
	vals := make([]starlark.Value, len(names))
	for i, n := range names {
		vals[i] = starlark.String(n)
	}
	return starlark.NewList(vals), nil
}

func (r *Run) activeSheet(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := unpack("active_sheet", args, kwargs); err != nil {
		return nil, err
	}
	var name string
	r.host(func(h Host) error { name = h.ActiveSheet(); return nil })
	return starlark.String(name), nil
}

func (r *Run) activateSheet(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := unpack("activate_sheet", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.None, r.host(func(h Host) error { return h.ActivateSheet(name) })
}

func (r *Run) addSheet(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := unpack("add_sheet", args, kwargs, "name?", &name); err != nil {
		return nil, err
	}
	var got string
	err := r.host(func(h Host) (err error) { got, err = h.AddSheet(name); return })
	return starlark.String(got), err
}

func (r *Run) moveSheet(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var pos int
	if err := unpack("move_sheet", args, kwargs, "position", &pos); err != nil {
		return nil, err
	}
	return starlark.None, r.host(func(h Host) error { return h.MoveSheet(pos) })
}

func (r *Run) run(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var id string
	var answer starlark.Value = starlark.None
	if err := unpack("run", args, kwargs, "id", &id, "answer?", &answer); err != nil {
		return nil, err
	}
	var a *string
	if answer != starlark.None {
		text, err := input("run", answer)
		if err != nil {
			return nil, err
		}
		a = &text
	}
	return starlark.None, r.host(func(h Host) error { return h.Run(id, a) })
}

func (r *Run) setWidth(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var cols string
	var width int
	if err := unpack("set_width", args, kwargs, "cols", &cols, "width", &width); err != nil {
		return nil, err
	}
	return starlark.None, r.host(func(h Host) error { return h.SetWidth(cols, width) })
}

func (r *Run) fill(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var to string
	var rows, cols int
	if err := unpack("fill", args, kwargs, "to?", &to, "rows?", &rows, "cols?", &cols); err != nil {
		return nil, err
	}
	if (to != "") == (rows != 0 || cols != 0) || rows != 0 && cols != 0 {
		return nil, fmt.Errorf("fill: give either to, or one of rows and cols")
	}
	return starlark.None, r.host(func(h Host) error { return h.Fill(to, rows, cols) })
}

// offset is ref moved by cols and rows, keeping its sheet and size; it
// doesn't need the spreadsheet.
func offset(args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ref string
	var cols, rows int
	if err := unpack("offset", args, kwargs, "ref", &ref, "cols?", &cols, "rows?", &rows); err != nil {
		return nil, err
	}
	name, rest := formula.SplitSheet(ref)
	rng, ok := formula.ParseRange(rest)
	if !ok {
		return nil, fmt.Errorf("offset: not a cell or range: %s", ref)
	}
	rng.From.Col, rng.To.Col = rng.From.Col+cols, rng.To.Col+cols
	rng.From.Row, rng.To.Row = rng.From.Row+rows, rng.To.Row+rows
	if !rng.From.Valid() || !rng.To.Valid() {
		return nil, fmt.Errorf("offset: %s moved by %d, %d is off the sheet", ref, cols, rows)
	}
	out := rng.String()
	if rng.From == rng.To && !strings.Contains(rest, ":") {
		out = rng.From.String()
	}
	if name != "" {
		out = formula.QuoteSheet(name) + "!" + out
	}
	return starlark.String(out), nil
}
