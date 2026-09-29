package nuon

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// maxDepth is how deeply lists and records may nest.
const maxDepth = 256

// SyntaxError is text that isn't NUON, with where it went wrong.
type SyntaxError struct {
	Line, Col int // 1-based
	Msg       string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Col, e.Msg)
}

// scanner reads NUON text a byte at a time from a buffered reader,
// counting lines and columns for errors, and noting whether everything
// read so far was also JSON.
type scanner struct {
	r         *bufio.Reader
	line, col int
	notJSON   bool
	depth     int
	started   bool // past a byte order mark
}

func newScanner(r io.Reader) *scanner {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReaderSize(r, 64<<10)
	}
	return &scanner{r: br, line: 1, col: 1}
}

// skipBOM drops a UTF-8 byte order mark at the start, once. It isn't
// done in newScanner, which mustn't wait for a pipe's first bytes.
func (s *scanner) skipBOM() {
	if s.started {
		return
	}
	s.started = true
	if s.peekAt(0) == 0xEF {
		if b, _ := s.r.Peek(3); string(b) == "\xEF\xBB\xBF" {
			s.r.Discard(3)
		}
	}
}

func (s *scanner) fail(format string, args ...any) error {
	return &SyntaxError{Line: s.line, Col: s.col, Msg: fmt.Sprintf(format, args...)}
}

// peek returns the next byte without taking it; ok is false at the end.
func (s *scanner) peek() (byte, bool, error) {
	b, err := s.r.Peek(1)
	if len(b) == 1 {
		return b[0], true, nil
	}
	if errors.Is(err, io.EOF) {
		return 0, false, nil
	}
	return 0, false, err
}

// peekAt returns the byte n ahead (0 is the next), or 0.
func (s *scanner) peekAt(n int) byte {
	b, _ := s.r.Peek(n + 1)
	if len(b) == n+1 {
		return b[n]
	}
	return 0
}

// take consumes the next byte.
func (s *scanner) take() (byte, error) {
	c, err := s.r.ReadByte()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return 0, s.fail("unexpected end of input")
		}
		return 0, err
	}
	if c == '\n' {
		s.line, s.col = s.line+1, 1
	} else if c&0xC0 != 0x80 {
		s.col++
	}
	return c, nil
}

// expect consumes c or fails.
func (s *scanner) expect(c byte) error {
	got, ok, err := s.peek()
	switch {
	case err != nil:
		return err
	case !ok:
		return s.fail("expected %q, found the end", c)
	case got != c:
		return s.fail("expected %q, found %q", c, got)
	}
	_, err = s.take()
	return err
}

// valid replaces bytes that aren't UTF-8 with U+FFFD, so strings read
// are text and write back as they read.
func valid(s string) string { return strings.ToValidUTF8(s, "�") }

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// space skips whitespace and comments; with commas, also the commas
// that may separate items of lists and records. It returns the next
// byte, ok false at the end.
func (s *scanner) space(commas bool) (byte, bool, error) {
	s.skipBOM()
	for {
		c, ok, err := s.peek()
		if err != nil || !ok {
			return 0, false, err
		}
		switch {
		case isSpace(c), commas && c == ',':
			s.take()
		case c == '#':
			s.notJSON = true
			if err := s.skipLine(); err != nil {
				return 0, false, err
			}
		default:
			return c, true, nil
		}
	}
}

func (s *scanner) skipLine() error {
	for {
		c, ok, err := s.peek()
		if err != nil || !ok || c == '\n' {
			return err
		}
		s.take()
	}
}

// delimits reports whether c ends a bare word. In a key, a colon does.
func delimits(c byte, key bool) bool {
	switch c {
	case ' ', '\t', '\n', '\r', ',', '[', ']', '{', '}', '(', ')', ';', '"', '\'', '`':
		return true
	case ':':
		return key
	}
	return false
}

// bare reads a bare word: a number, a size, a date or an unquoted
// string.
func (s *scanner) bare(key bool) (string, error) {
	var b strings.Builder
	for {
		c, ok, err := s.peek()
		if err != nil {
			return "", err
		}
		if !ok || delimits(c, key) {
			break
		}
		s.take()
		b.WriteByte(c)
	}
	if b.Len() == 0 {
		c, _, _ := s.peek()
		return "", s.fail("unexpected %q", c)
	}
	return valid(b.String()), nil
}

// str reads a quoted string at the next byte: "..." with escapes, or
// '...' and `...` taken as written.
func (s *scanner) str() (string, error) {
	q, err := s.take()
	if err != nil {
		return "", err
	}
	if q != '"' {
		s.notJSON = true
	}
	var b strings.Builder
	for {
		c, err := s.take()
		if err != nil {
			return "", err
		}
		switch {
		case c == q:
			return valid(b.String()), nil
		case c == '\\' && q == '"':
			if err := s.escape(&b); err != nil {
				return "", err
			}
		default:
			b.WriteByte(c)
		}
	}
}

// raw reads a raw string, r#'...'# with any number of #.
func (s *scanner) raw() (string, error) {
	s.notJSON = true
	s.take() // r
	hashes := 0
	for s.peekAt(0) == '#' {
		s.take()
		hashes++
	}
	if err := s.expect('\''); err != nil {
		return "", err
	}
	end := "'" + strings.Repeat("#", hashes)
	var b strings.Builder
	for {
		c, err := s.take()
		if err != nil {
			return "", err
		}
		b.WriteByte(c)
		if c == '#' || hashes == 0 && c == '\'' {
			if text := b.String(); strings.HasSuffix(text, end) {
				return valid(strings.TrimSuffix(text, end)), nil
			}
		}
	}
}

// isRaw reports whether a raw string starts at the next byte.
func (s *scanner) isRaw() bool {
	if s.peekAt(0) != 'r' || s.peekAt(1) != '#' {
		return false
	}
	for i := 1; ; i++ {
		switch s.peekAt(i) {
		case '#':
		case '\'':
			return true
		default:
			return false
		}
	}
}

var simpleEscapes = map[byte]byte{'"': '"', '\\': '\\', '/': '/', 'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', '\'': '\'', '0': 0}

// escape reads what follows a backslash in a double-quoted string.
func (s *scanner) escape(b *strings.Builder) error {
	c, err := s.take()
	if err != nil {
		return err
	}
	if r, ok := simpleEscapes[c]; ok {
		b.WriteByte(r)
		return nil
	}
	if c != 'u' {
		return s.fail("unknown escape \\%c", c)
	}
	if s.peekAt(0) == '{' {
		s.notJSON = true
		return s.braceEscape(b)
	}
	r, err := s.hex4()
	if err != nil {
		return err
	}
	if r >= 0xD800 && r < 0xDC00 && s.peekAt(0) == '\\' && s.peekAt(1) == 'u' {
		s.take()
		s.take()
		lo, err := s.hex4()
		if err != nil {
			return err
		}
		r = 0x10000 + (r-0xD800)<<10 + (lo - 0xDC00)
	}
	b.WriteRune(r)
	return nil
}

// braceEscape reads \u{1F600}, after the u.
func (s *scanner) braceEscape(b *strings.Builder) error {
	s.take() // {
	var r rune
	for n := 0; ; n++ {
		c, err := s.take()
		if err != nil {
			return err
		}
		if c == '}' && n > 0 {
			b.WriteRune(r)
			return nil
		}
		d, ok := hexDigit(c)
		if !ok || n == 6 {
			return s.fail("bad \\u{...} escape")
		}
		r = r<<4 | rune(d)
	}
}

func (s *scanner) hex4() (rune, error) {
	var r rune
	for range 4 {
		c, err := s.take()
		if err != nil {
			return 0, err
		}
		d, ok := hexDigit(c)
		if !ok {
			return 0, s.fail("bad \\u escape")
		}
		r = r<<4 | rune(d)
	}
	return r, nil
}

func hexDigit(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
