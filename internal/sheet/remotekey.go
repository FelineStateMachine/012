package sheet

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Key identifies a question by its content: the answer cache and the
// engine's list of cells waiting for answers are keyed by it. JEV cells
// are volatile, so every recalculation looks each one up: with thousands
// of them, encoding the key with encoding/json took most of the time an
// answer spent in recalc. The shapes a RemoteCall holds are encoded by
// hand, each value tagged and each string length-prefixed so different
// calls never share a key; anything else falls back to JSON.
func (c RemoteCall) Key() string {
	var b strings.Builder
	b.Grow(64 + len(c.Instructions))
	keyStr(&b, c.Kind)
	keyStr(&b, c.Instructions)
	if keyValue(&b, c.State) && keyValue(&b, c.Criteria) {
		return b.String()
	}
	j, err := json.Marshal(c)
	if err != nil {
		return fmt.Sprintf("%#v", c)
	}
	return "j" + string(j)
}

func keyStr(b *strings.Builder, s string) {
	b.WriteString(strconv.Itoa(len(s)))
	b.WriteByte(':')
	b.WriteString(s)
}

// keyValue encodes v and reports whether it is a shape it knows.
func keyValue(b *strings.Builder, v any) bool {
	switch v := v.(type) {
	case nil:
		b.WriteByte('n')
	case string:
		b.WriteByte('s')
		keyStr(b, v)
	case float64:
		b.WriteByte('f')
		b.WriteString(strconv.FormatFloat(v, 'g', -1, 64))
		b.WriteByte(';')
	case bool:
		if v {
			b.WriteByte('T')
		} else {
			b.WriteByte('F')
		}
	case [2]string:
		b.WriteByte('p')
		keyStr(b, v[0])
		keyStr(b, v[1])
	case []string:
		b.WriteByte('l')
		b.WriteString(strconv.Itoa(len(v)))
		for _, s := range v {
			keyStr(b, s)
		}
	case map[string]string:
		b.WriteByte('m')
		b.WriteString(strconv.Itoa(len(v)))
		for _, k := range slices.Sorted(maps.Keys(v)) {
			keyStr(b, k)
			keyStr(b, v[k])
		}
	case []any:
		b.WriteByte('a')
		b.WriteString(strconv.Itoa(len(v)))
		for _, e := range v {
			if !keyValue(b, e) {
				return false
			}
		}
	case [][]any:
		b.WriteByte('r')
		b.WriteString(strconv.Itoa(len(v)))
		for _, row := range v {
			if !keyValue(b, row) {
				return false
			}
		}
	default:
		return false
	}
	return true
}
