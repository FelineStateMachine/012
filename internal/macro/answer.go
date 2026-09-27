package macro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// Structured answers. A dialog's choices (the columns of a sort, the
// values a filter hides, a chart's options) don't fit one string, so a
// recording writes them as a dict, run("data.sort_range", answer={"by":
// [{"column": "B"}]}), and run hands a dict or list answer to the Host
// as JSON text, keys in the order written.

// JSON is JSON text an Action writes as a Starlark dict or list literal,
// keys in the order they come.
type JSON string

// starlarkLiteral writes JSON text as a Starlark literal: objects as
// dicts, arrays as lists, and null, true and false as None, True and
// False.
func starlarkLiteral(text string) (string, error) {
	d := json.NewDecoder(strings.NewReader(text))
	d.UseNumber()
	var b strings.Builder
	if err := writeLiteral(&b, d); err != nil {
		return "", err
	}
	if _, err := d.Token(); err != io.EOF {
		return "", fmt.Errorf("more than one value in %q", text)
	}
	return b.String(), nil
}

// writeLiteral writes the next JSON value of d.
func writeLiteral(b *strings.Builder, d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	switch t := t.(type) {
	case json.Delim:
		return writeContainer(b, d, t)
	case string:
		b.WriteString(syntax.Quote(t, false))
	case json.Number:
		b.WriteString(t.String())
	case bool:
		b.WriteString(map[bool]string{true: "True", false: "False"}[t])
	case nil:
		b.WriteString("None")
	}
	return nil
}

// writeContainer writes an object or array whose opening delimiter was
// open.
func writeContainer(b *strings.Builder, d *json.Decoder, open json.Delim) error {
	obj := open == '{'
	if obj {
		b.WriteByte('{')
	} else {
		b.WriteByte('[')
	}
	for i := 0; d.More(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		if obj {
			k, err := d.Token()
			if err != nil {
				return err
			}
			b.WriteString(syntax.Quote(k.(string), false) + ": ")
		}
		if err := writeLiteral(b, d); err != nil {
			return err
		}
	}
	if _, err := d.Token(); err != nil { // the closing delimiter
		return err
	}
	if obj {
		b.WriteByte('}')
	} else {
		b.WriteByte(']')
	}
	return nil
}

// answerJSON writes a script's dict or list answer as JSON text, keys in
// the order the dict holds them.
func answerJSON(v starlark.Value) (string, error) {
	var b bytes.Buffer
	if err := writeJSON(&b, v); err != nil {
		return "", err
	}
	return b.String(), nil
}

func writeJSON(b *bytes.Buffer, v starlark.Value) error {
	switch v := v.(type) {
	case starlark.NoneType:
		b.WriteString("null")
	case starlark.Bool:
		b.WriteString(strconv.FormatBool(bool(v)))
	case starlark.Int:
		b.WriteString(v.String())
	case starlark.Float:
		raw, err := json.Marshal(float64(v))
		if err != nil {
			return fmt.Errorf("run: answer: %v", err)
		}
		b.Write(raw)
	case starlark.String:
		raw, _ := json.Marshal(string(v))
		b.Write(raw)
	case *starlark.Dict:
		return writeDict(b, v)
	case starlark.Indexable: // lists and tuples
		b.WriteByte('[')
		for i := range v.Len() {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeJSON(b, v.Index(i)); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	default:
		return fmt.Errorf("run: an answer can't hold a %s", v.Type())
	}
	return nil
}

func writeDict(b *bytes.Buffer, d *starlark.Dict) error {
	b.WriteByte('{')
	for i, kv := range d.Items() {
		k, ok := kv[0].(starlark.String)
		if !ok {
			return fmt.Errorf("run: an answer's keys are strings, not %s", kv[0].Type())
		}
		if i > 0 {
			b.WriteByte(',')
		}
		raw, _ := json.Marshal(string(k))
		b.Write(raw)
		b.WriteByte(':')
		if err := writeJSON(b, kv[1]); err != nil {
			return err
		}
	}
	b.WriteByte('}')
	return nil
}
