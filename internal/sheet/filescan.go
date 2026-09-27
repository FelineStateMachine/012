package sheet

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

// scanner reads JSON a value at a time from a stream, for the .012
// reader (fileread.go): the cells are read one by one straight into the
// sheet, and everything else, which is small, is captured whole and
// given to encoding/json, so it decodes as it always has. Strings
// without escapes are read without encoding/json; the rest go through
// it, so text decodes exactly as it would there.
type scanner struct {
	r   *bufio.Reader
	buf []byte // scratch for a string longer than the reader's buffer or escaped
}

func newScanner(r io.Reader) *scanner { return &scanner{r: bufio.NewReaderSize(r, 64<<10)} }

// errSyntax is returned for anything that isn't the JSON expected.
var errSyntax = errors.New("invalid JSON")

// syntax describes a byte that wasn't expected.
func syntax(c byte, want string) error {
	return fmt.Errorf("%w: %q where %s was expected", errSyntax, c, want)
}

// eofErr is err, turning a clean EOF in the middle of a value into
// io.ErrUnexpectedEOF, as encoding/json does.
func eofErr(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

// peek skips white space and returns the next byte without reading it.
func (sc *scanner) peek() (byte, error) {
	for {
		c, err := sc.r.ReadByte()
		if err != nil {
			return 0, eofErr(err)
		}
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		}
		return c, sc.r.UnreadByte()
	}
}

// next skips white space and reads the next byte.
func (sc *scanner) next() (byte, error) {
	if _, err := sc.peek(); err != nil {
		return 0, err
	}
	return sc.r.ReadByte()
}

// expect reads the next byte, which must be c.
func (sc *scanner) expect(c byte) error {
	got, err := sc.next()
	if err == nil && got != c {
		err = syntax(got, fmt.Sprintf("%q", c))
	}
	return err
}

// null reads the literal null if it comes next, and reports whether it
// did.
func (sc *scanner) null() (bool, error) {
	c, err := sc.peek()
	if err != nil || c != 'n' {
		return false, err
	}
	raw, err := sc.raw()
	if err == nil && string(raw) != "null" {
		err = fmt.Errorf("%w: %q", errSyntax, raw)
	}
	return err == nil, err
}

// members reads an object, calling fn with each key once the reader is
// at its value, which fn must read.
func (sc *scanner) members(fn func(key string) error) error {
	if err := sc.expect('{'); err != nil {
		return err
	}
	return sc.list('}', func() error {
		key, err := sc.str()
		if err != nil {
			return err
		}
		if err := sc.expect(':'); err != nil {
			return err
		}
		return fn(key)
	})
}

// elements reads an array, calling fn for each element, which fn must
// read.
func (sc *scanner) elements(fn func() error) error {
	if err := sc.expect('['); err != nil {
		return err
	}
	return sc.list(']', fn)
}

// list reads the items of an object or array up to its closing byte,
// with fn reading each.
func (sc *scanner) list(end byte, fn func() error) error {
	c, err := sc.peek()
	if err != nil {
		return err
	}
	if c == end {
		_, err = sc.r.ReadByte()
		return err
	}
	for {
		if err := fn(); err != nil {
			return err
		}
		c, err := sc.next()
		switch {
		case err != nil:
			return err
		case c == end:
			return nil
		case c != ',':
			return syntax(c, fmt.Sprintf(`"," or %q`, end))
		}
	}
}

// str reads a string.
func (sc *scanner) str() (string, error) {
	body, plain, err := sc.text()
	if err != nil || plain {
		return string(body), err
	}
	var s string
	err = json.Unmarshal(quote(body), &s)
	return s, err
}

// quote is body between quotes, in a new slice.
func quote(body []byte) []byte {
	out := make([]byte, 0, len(body)+2)
	return append(append(append(out, '"'), body...), '"')
}

// text reads a string as it is written between its quotes, and reports
// whether it is plain: no escapes, control characters or invalid UTF-8,
// so those bytes are its text. They are valid until the next read.
func (sc *scanner) text() ([]byte, bool, error) {
	if err := sc.expect('"'); err != nil {
		return nil, false, err
	}
	part, err := sc.r.ReadSlice('"')
	if err == nil && !escapedQuote(part) { // the common case: no copy
		body := part[:len(part)-1]
		return body, plainText(body), nil
	}
	sc.buf = append(sc.buf[:0], part...)
	for err != nil || escapedQuote(sc.buf) {
		if err != nil && err != bufio.ErrBufferFull {
			return nil, false, eofErr(err)
		}
		part, err = sc.r.ReadSlice('"')
		sc.buf = append(sc.buf, part...)
	}
	body := sc.buf[:len(sc.buf)-1]
	return body, plainText(body), nil
}

// plainText reports whether a string's body as written is its text.
func plainText(body []byte) bool {
	for _, c := range body {
		if c < 0x20 || c == '\\' {
			return false
		}
	}
	return utf8.Valid(body)
}

// escapedQuote reports whether the quote ending s is escaped: preceded
// by an odd number of backslashes.
func escapedQuote(s []byte) bool {
	n := 0
	for i := len(s) - 2; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// raw reads the next value whole, as it is written. It checks only
// that brackets balance outside strings; encoding/json checks the rest
// when the value is decoded.
func (sc *scanner) raw() (json.RawMessage, error) {
	c, err := sc.peek()
	if err != nil {
		return nil, err
	}
	switch c {
	case '"':
		body, _, err := sc.text()
		return quote(body), err
	case '{', '[':
		return sc.nested()
	}
	var out []byte
	for {
		c, err := sc.r.ReadByte()
		if err == io.EOF && len(out) > 0 {
			return out, nil
		}
		if err != nil {
			return nil, eofErr(err)
		}
		switch c {
		case ',', '}', ']', ' ', '\t', '\n', '\r':
			return out, sc.r.UnreadByte()
		}
		out = append(out, c)
	}
}

// nested reads an object or array whole.
func (sc *scanner) nested() (json.RawMessage, error) {
	var out []byte
	depth := 0
	for {
		c, err := sc.r.ReadByte()
		if err != nil {
			return nil, eofErr(err)
		}
		switch c {
		case '"':
			if err := sc.r.UnreadByte(); err != nil {
				return nil, err
			}
			body, _, err := sc.text()
			if err != nil {
				return nil, err
			}
			out = append(append(append(out, '"'), body...), '"')
			continue
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
		out = append(out, c)
		if depth == 0 {
			return out, nil
		}
	}
}
