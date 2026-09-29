package diff

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/FelineStateMachine/012/internal/nuon"
)

// quietKeys are the workbook's fields that change without the workbook
// changing: the file version, the sheet shown, the computer its macros
// were trusted on and the notebook prompt's history.
var quietKeys = map[string]bool{"version": true, "active": true, "macroOrigin": true, "shellHistory": true}

// bookChanges compares the workbook's fields: named ranges, macros and
// settings.
func bookChanges(a, b *rawBook) []Change {
	var out []Change
	out = append(out, keyedChanges(KindName, "range", a.top["names"], b.top["names"])...)
	out = append(out, listChanges(KindMacro, "", a.top["macros"], b.top["macros"])...)
	keys := map[string]bool{}
	for _, t := range []map[string]json.RawMessage{a.top, b.top} {
		for k := range t {
			keys[k] = true
		}
	}
	for _, k := range slices.Sorted(maps.Keys(keys)) {
		if !quietKeys[k] && k != "names" && k != "macros" {
			out = append(out, fieldChanges(KindWorkbook, "", k, a.top[k], b.top[k])...)
		}
	}
	return out
}

// fieldChanges compares a field: key by key when it's an object (a
// column's width), else whole.
func fieldChanges(kind, sheetName, item string, a, b json.RawMessage) []Change {
	if equal(a, b) {
		return nil
	}
	oa, okA := object(a)
	ob, okB := object(b)
	if okA && okB {
		var out []Change
		for _, k := range unionKeys(oa, ob) {
			if !equal(oa[k], ob[k]) {
				out = append(out, rawChange(kind, sheetName, item, k, oa[k], ob[k]))
			}
		}
		return out
	}
	return []Change{rawChange(kind, sheetName, item, "", a, b)}
}

// keyedChanges compares an object whose keys are things (named ranges):
// each added, removed, or its value changed, as field.
func keyedChanges(kind, field string, a, b json.RawMessage) []Change {
	oa, _ := object(a)
	ob, _ := object(b)
	var out []Change
	for _, k := range unionKeys(oa, ob) {
		switch va, vb := oa[k], ob[k]; {
		case va == nil:
			out = append(out, rawChange(kind, "", k, "added", nil, vb))
		case vb == nil:
			out = append(out, rawChange(kind, "", k, "removed", va, nil))
		case !equal(va, vb):
			out = append(out, rawChange(kind, "", k, field, va, vb))
		}
	}
	return out
}

// listChanges compares lists of things with names (regions, macros):
// each added, removed, or changed field by field.
func listChanges(kind, sheetName string, a, b json.RawMessage) []Change {
	la, lb := named(a), named(b)
	var out []Change
	for _, k := range unionKeys(la, lb) {
		switch ea, eb := la[k], lb[k]; {
		case ea == nil:
			c := rawChange(kind, sheetName, k, "added", nil, eb)
			c.newText = summary(eb)
			out = append(out, c)
		case eb == nil:
			c := rawChange(kind, sheetName, k, "removed", ea, nil)
			c.oldText = summary(ea)
			out = append(out, c)
		default:
			fa, _ := object(ea)
			fb, _ := object(eb)
			for _, f := range unionKeys(fa, fb) {
				if !equal(fa[f], fb[f]) {
					out = append(out, rawChange(kind, sheetName, k, f, fa[f], fb[f]))
				}
			}
		}
	}
	return out
}

// named reads a list of objects by their "name" fields, as compact
// JSON.
func named(raw json.RawMessage) map[string]json.RawMessage {
	var list []json.RawMessage
	json.Unmarshal(raw, &list)
	out := map[string]json.RawMessage{}
	for _, e := range list {
		var n struct{ Name string }
		json.Unmarshal(e, &n)
		out[n.Name] = compact(e)
	}
	return out
}

// summary is what the text output shows of a region, table or macro
// added or removed: the file a region follows or that it's a notebook's
// output; a table's range; a macro's script's first line.
func summary(raw json.RawMessage) string {
	fields, _ := object(raw)
	var s string
	switch {
	case json.Unmarshal(fields["path"], &s) == nil && s != "":
		return s
	case string(fields["output"]) == "true":
		return "a notebook cell's output"
	case json.Unmarshal(fields["range"], &s) == nil && s != "":
		return s
	case json.Unmarshal(fields["source"], &s) == nil && s != "":
		return firstLine(s)
	}
	return string(raw)
}

// rawChange is a change between two JSON values, nil for none.
func rawChange(kind, sheetName, item, field string, a, b json.RawMessage) Change {
	return Change{Kind: kind, Sheet: sheetName, Item: item, Field: field,
		Old: jsonValue(a), New: jsonValue(b), oldText: rawText(a), newText: rawText(b)}
}

func jsonValue(raw json.RawMessage) nuon.Value {
	if raw == nil {
		return nuon.NullValue()
	}
	v, err := nuon.Parse(raw)
	if err != nil {
		return nuon.StringValue(string(raw))
	}
	return v
}

// rawText is a JSON value as the text output shows it: a string as it
// is, anything else as compact JSON.
func rawText(raw json.RawMessage) string {
	var s string
	if raw == nil {
		return ""
	}
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

func object(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var m map[string]json.RawMessage
	if raw == nil {
		return map[string]json.RawMessage{}, true
	}
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return nil, false
	}
	for k, v := range m {
		m[k] = compact(v)
	}
	return m, true
}

func unionKeys(a, b map[string]json.RawMessage) []string {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	return slices.Sorted(maps.Keys(keys))
}
