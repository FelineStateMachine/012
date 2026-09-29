package nuon

import (
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"time"
)

func TestParseValues(t *testing.T) {
	utc := func(s string) Value {
		tm, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatal(err)
		}
		return DateValue(tm)
	}
	for in, want := range map[string]Value{
		`null`:                                NullValue(),
		`true`:                                BoolValue(true),
		`TRUE`:                                StringValue("TRUE"),
		`42`:                                  IntValue(42),
		`-7`:                                  IntValue(-7),
		`+3`:                                  IntValue(3),
		`1_000`:                               IntValue(1000),
		`0x2a`:                                IntValue(42),
		`0o17`:                                IntValue(15),
		`0b101`:                               IntValue(5),
		`-0x10`:                               StringValue("-0x10"),
		`1.5`:                                 FloatValue(1.5),
		`.5`:                                  FloatValue(0.5),
		`5.`:                                  FloatValue(5),
		`1e3`:                                 FloatValue(1000),
		`-1.5e-3`:                             FloatValue(-0.0015),
		`inf`:                                 FloatValue(math.Inf(1)),
		`-inf`:                                FloatValue(math.Inf(-1)),
		`1646b`:                               FilesizeValue(1646),
		`1.5kb`:                               FilesizeValue(1500),
		`1.5kB`:                               FilesizeValue(1500),
		`1KiB`:                                FilesizeValue(1024),
		`0.5b`:                                FilesizeValue(0),
		`-1.5kb`:                              FilesizeValue(-1500),
		`90sec`:                               DurationValue(90 * time.Second),
		`1.5sec`:                              DurationValue(1500 * time.Millisecond),
		`3us`:                                 DurationValue(3 * time.Microsecond),
		`3µs`:                                 DurationValue(3 * time.Microsecond),
		`2wk`:                                 DurationValue(14 * 24 * time.Hour),
		`-3ms`:                                DurationValue(-3 * time.Millisecond),
		`1500000000ns`:                        DurationValue(1500 * time.Millisecond),
		`2026-09-27`:                          utc("2026-09-27T00:00:00Z"),
		`2026-09-27T11:27:31`:                 utc("2026-09-27T11:27:31Z"),
		`2026-09-27T11:27:31.351890163-06:00`: utc("2026-09-27T11:27:31.351890163-06:00"),
		`2026-09-27T11:27:31Z`:                utc("2026-09-27T11:27:31Z"),
		`0x[DEAD]`:                            BinaryValue([]byte{0xde, 0xad}),
		`0x[de ad be]`:                        BinaryValue([]byte{0xde, 0xad, 0xbe}),
		`0b[1010 0101]`:                       BinaryValue([]byte{0xa5}),
		`abc`:                                 StringValue("abc"),
		`-`:                                   StringValue("-"),
		`1..3`:                                StringValue("1..3"),
		`"a\"b\n\u{41}B\t"`:                   StringValue("a\"b\nAB\t"),
		`"😀"`:                                 StringValue("😀"),
		`'it\'`:                               StringValue(`it\`),
		"`b c`":                               StringValue("b c"),
		`r#'say 'hi''#`:                       StringValue("say 'hi'"),
		`r##'a'#b'##`:                         StringValue("a'#b"),
		`[1 2, 3,]`:                           ListValue(IntValue(1), IntValue(2), IntValue(3)),
		`[]`:                                  ListValue(),
		`{}`:                                  RecordValue(),
		`{a: 1, "b c": [x], 'd': {e: null} f:2}`: RecordValue(
			Field{"a", IntValue(1)}, Field{"b c", ListValue(StringValue("x"))},
			Field{"d", RecordValue(Field{"e", NullValue()})}, Field{"f", IntValue(2)}),
		`[[a, b]; [1, 2], [3, 4]]`: ListValue(
			RecordValue(Field{"a", IntValue(1)}, Field{"b", IntValue(2)}),
			RecordValue(Field{"a", IntValue(3)}, Field{"b", IntValue(4)})),
		`[[1, 2], [3]]`:               ListValue(ListValue(IntValue(1), IntValue(2)), ListValue(IntValue(3))),
		"# a comment\n[1, # one\n 2]": ListValue(IntValue(1), IntValue(2)),
	} {
		got, err := Parse([]byte(in))
		if err != nil {
			t.Errorf("Parse(%s): %v", in, err)
			continue
		}
		if !Equal(got, want) {
			t.Errorf("Parse(%s) = %s (%s), want %s (%s)", in, got, got.Kind, want, want.Kind)
		}
	}
	if v, err := Parse([]byte("NaN")); err != nil || !math.IsNaN(v.Float) {
		t.Errorf("NaN: %v %v", v, err)
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{``, `[1, 2`, `{a 1}`, `{a: }`, `"abc`, `"\q"`, `[[a, b]; [1]]`, `[[a]; x]`,
		`1 2`, `(1 + 2)`, `$x`, `0x[zz]`, `[[{}]; [1]]`, strings.Repeat("[", 1000)} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("Parse(%q) took it", in)
		}
	}
	_, err := Parse([]byte("[1,\n  2 }"))
	var se *SyntaxError
	if !errors.As(err, &se) || se.Line != 2 || se.Col != 5 {
		t.Errorf("error %v, want line 2 column 5", err)
	}
}

func TestAppend(t *testing.T) {
	loc := time.FixedZone("", -6*3600)
	for want, v := range map[string]Value{
		`null`:                                NullValue(),
		`1.0`:                                 FloatValue(1),
		`100000000000000000000.0`:             FloatValue(1e20),
		`-0.5`:                                FloatValue(-0.5),
		`abc`:                                 StringValue("abc"),
		`"a b"`:                               StringValue("a b"),
		`""`:                                  StringValue(""),
		`"true"`:                              StringValue("true"),
		`"12"`:                                StringValue("12"),
		`"1kb"`:                               StringValue("1kb"),
		`"-"`:                                 StringValue("-"),
		`"2026-01-01"`:                        StringValue("2026-01-01"),
		`"a\"b\n\u{1b}"`:                      StringValue("a\"b\n\x1b"),
		`1646b`:                               FilesizeValue(1646),
		`90000000000ns`:                       DurationValue(90 * time.Second),
		`2026-09-27T11:27:31-06:00`:           DateValue(time.Date(2026, 9, 27, 11, 27, 31, 0, loc)),
		`2026-09-27T11:27:31.500+00:00`:       DateValue(time.Date(2026, 9, 27, 11, 27, 31, 5e8, time.UTC)),
		`2026-09-27T11:27:31.000001+00:00`:    DateValue(time.Date(2026, 9, 27, 11, 27, 31, 1e3, time.UTC)),
		`2026-09-27T11:27:31.351890163+00:00`: DateValue(time.Date(2026, 9, 27, 11, 27, 31, 351890163, time.UTC)),
		`0x[DEAD]`:                            BinaryValue([]byte{0xde, 0xad}),
		`{a: 1, "b c": [1, 2]}`:               RecordValue(Field{"a", IntValue(1)}, Field{"b c", ListValue(IntValue(1), IntValue(2))}),
		`[[a, b]; [1, 2], [3, null]]`:         ListValue(RecordValue(Field{"a", IntValue(1)}, Field{"b", IntValue(2)}), RecordValue(Field{"a", IntValue(3)}, Field{"b", NullValue()})),
		`[{a: 1}, {b: 2}]`:                    ListValue(RecordValue(Field{"a", IntValue(1)}), RecordValue(Field{"b", IntValue(2)})),
	} {
		if got := v.String(); got != want {
			t.Errorf("%v wrote %s, want %s", v, got, want)
		}
		back, err := Parse([]byte(want))
		if err != nil || !Equal(back, v) {
			t.Errorf("%s read back as %v, %v", want, back, err)
		}
	}
}

func TestAppendJSON(t *testing.T) {
	v := RecordValue(Field{"a", FilesizeValue(1000)}, Field{"b", DurationValue(time.Second)},
		Field{"c", DateValue(time.Date(2026, 9, 27, 11, 27, 31, 5e8, time.FixedZone("", 7200)))},
		Field{"d", BinaryValue([]byte{1, 2})}, Field{"e", FloatValue(1)}, Field{"f", FloatValue(math.Inf(1))},
		Field{"g <", StringValue("x\"y")})
	want := `{"a":1000,"b":1000000000,"c":"2026-09-27T11:27:31.500+02:00","d":[1,2],"e":1.0,"f":null,"g <":"x\"y"}`
	if got := string(AppendJSON(nil, v)); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// rows reads every row of in.
func rows(t *testing.T, in string) ([]Row, *Reader) {
	t.Helper()
	r := NewReader(strings.NewReader(in))
	var out []Row
	for {
		row, err := r.Next()
		if errors.Is(err, io.EOF) {
			return out, r
		}
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		out = append(out, row)
	}
}

func TestReaderShapes(t *testing.T) {
	for in, want := range map[string]string{
		`[[a, b]; [1, 2], [3, 4]]`: `{a: 1, b: 2}|{a: 3, b: 4}`,
		`[{a: 1}, {b: 2, a: 3}]`:   `{a: 1}|{b: 2, a: 3}`,
		`[1, x]`:                   `{value: 1}|{value: x}`,
		`[[1, 2], [3]]`:            `{value: [1, 2]}|{value: [3]}`,
		`{a: 1}`:                   `{a: 1}`,
		`42`:                       `{value: 42}`,
		"{\"a\": 1}\n{\"a\": 2}\n": `{a: 1}|{a: 2}`,
		`[[a]; [1]] [[b]; [2]]`:    `{a: 1}|{b: 2}`,
		`[]`:                       ``,
		``:                         ``,
		"  \n# nothing\n":          ``,
	} {
		got, _ := rows(t, in)
		var parts []string
		for _, r := range got {
			parts = append(parts, RecordValue(r...).String())
		}
		if s := strings.Join(parts, "|"); s != want {
			t.Errorf("%s: rows %s, want %s", in, s, want)
		}
	}
}

func TestReaderHeaderAndJSON(t *testing.T) {
	r := NewReader(strings.NewReader(`[[name, size]; [a, 1kb], [b, 2kb]]`))
	if _, err := r.Next(); err != nil {
		t.Fatal(err)
	}
	if h := r.Header(); strings.Join(h, ",") != "name,size" {
		t.Errorf("header %q", h)
	}
	if r.JSON() {
		t.Error("the table form is JSON")
	}
	for in, want := range map[string]bool{
		`[{"a": 1, "b": [true, null, "x"]}, {"a": -1.5e3}]`: true,
		`[{a: 1}]`:          false,
		`[{"a": 1kb}]`:      false,
		`[{"a": 'x'}]`:      false,
		`{"a": "\u{41}"}`:   false,
		"{\"a\": 1} # note": false,
	} {
		if _, r := rows(t, in); r.JSON() != want {
			t.Errorf("%s: JSON %v", in, r.JSON())
		}
	}
}

// TestReaderStreams reads rows before the text is complete: each row is
// handed over once it has been read, so a pipe still being written
// yields what it has.
func TestReaderStreams(t *testing.T) {
	pr, pw := io.Pipe()
	r := NewReader(pr)
	got := make(chan Row)
	go func() {
		for {
			row, err := r.Next()
			if err != nil {
				close(got)
				return
			}
			got <- row
		}
	}()
	io.WriteString(pw, "[[a]; [1], ")
	if row := <-got; RecordValue(row...).String() != "{a: 1}" {
		t.Errorf("first row %v", row)
	}
	io.WriteString(pw, "[2]]")
	if row := <-got; RecordValue(row...).String() != "{a: 2}" {
		t.Errorf("second row %v", row)
	}
	pw.Close()
	if _, ok := <-got; ok {
		t.Error("a row after the end")
	}
}

func TestTableWriter(t *testing.T) {
	var b strings.Builder
	w := NewTableWriter(&b, []string{"name", "size in"})
	w.Write([]Value{StringValue("a"), FilesizeValue(1)})
	w.Write([]Value{StringValue("b c"), NullValue()})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	want := "[[name, \"size in\"]; [a, 1b],\n[\"b c\", null]]\n"
	if b.String() != want {
		t.Errorf("wrote %q", b.String())
	}
	b.Reset()
	NewTableWriter(&b, []string{"a"}).Close()
	if b.String() != "[]\n" {
		t.Errorf("empty table %q", b.String())
	}
}

// FuzzParse holds the parser to its writer: whatever parses writes NUON
// that parses back to the same value, and nothing panics.
func FuzzParse(f *testing.F) {
	for _, s := range []string{`[[a, b]; [1, 2kb]]`, `{a: [1 2], "b": 2026-09-27T11:27:31Z}`, `0x[de ad]`,
		`r#'x'#`, `"\u{41}"`, `[1.5sec, -3, 1e9, inf, NaN]`, `# c` + "\n" + `{}`, `[{"a": null}]`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		v, err := Parse(data)
		if err != nil {
			return
		}
		text := v.String()
		back, err := Parse([]byte(text))
		if err != nil {
			t.Fatalf("%q wrote %s, which doesn't parse: %v", data, text, err)
		}
		if !Equal(v, back) {
			t.Fatalf("%q wrote %s, which reads as %s", data, text, back)
		}
		NewReader(strings.NewReader(string(data))).Next()
	})
}
