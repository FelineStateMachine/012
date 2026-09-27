package jev

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"012/internal/sheet"
)

// Key identifies a question by its content. JEV cells are volatile, so
// every recalculation looks each one up: with thousands of them,
// encoding the key with encoding/json took most of the time an answer
// spent in recalc. The shapes a RemoteCall holds are encoded by hand,
// each value tagged and each string length-prefixed so different calls
// never share a key; anything else falls back to JSON.
func Key(c sheet.RemoteCall) string {
	var b strings.Builder
	b.Grow(64 + len(c.Instructions))
	str(&b, c.Kind)
	str(&b, c.Instructions)
	if appendValue(&b, c.State) && appendValue(&b, c.Criteria) {
		return b.String()
	}
	j, err := json.Marshal(c)
	if err != nil {
		return fmt.Sprintf("%#v", c)
	}
	return "j" + string(j)
}

func str(b *strings.Builder, s string) {
	b.WriteString(strconv.Itoa(len(s)))
	b.WriteByte(':')
	b.WriteString(s)
}

// appendValue encodes v and reports whether it is a shape it knows.
func appendValue(b *strings.Builder, v any) bool {
	switch v := v.(type) {
	case nil:
		b.WriteByte('n')
	case string:
		b.WriteByte('s')
		str(b, v)
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
		str(b, v[0])
		str(b, v[1])
	case []string:
		b.WriteByte('l')
		b.WriteString(strconv.Itoa(len(v)))
		for _, s := range v {
			str(b, s)
		}
	case map[string]string:
		b.WriteByte('m')
		b.WriteString(strconv.Itoa(len(v)))
		for _, k := range slices.Sorted(maps.Keys(v)) {
			str(b, k)
			str(b, v[k])
		}
	case []any:
		b.WriteByte('a')
		b.WriteString(strconv.Itoa(len(v)))
		for _, e := range v {
			if !appendValue(b, e) {
				return false
			}
		}
	case [][]any:
		b.WriteByte('r')
		b.WriteString(strconv.Itoa(len(v)))
		for _, row := range v {
			if !appendValue(b, row) {
				return false
			}
		}
	default:
		return false
	}
	return true
}
