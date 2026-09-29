package sheet

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/locale"
)

// The reader streams the file (see scanner): cells go straight into
// their sheet as they are read, so opening a file allocates about what
// the workbook holds rather than a tree of the whole document. The
// other fields of the workbook and of each sheet are small; they are
// kept as written and decoded with encoding/json once the stream ends.
//
// Keys match fields as encoding/json matches them, case aside, and a
// field given twice takes its last value, cells adding up, except that
// the version and the list of sheets, which say where cells go, are
// refused twice once cells were read by them. Cells come
// after "version" in every file Write makes; any read before it (a file
// written by hand) are kept as written until the version is known.
type fileReader struct {
	sc      *scanner
	w       *Workbook
	version int
	// hasVersion is set once "version" is read, and streamed once cells
	// were read with it, which it may then no longer change.
	hasVersion, streamed bool
	listed               bool              // the list of sheets was read
	single               *Sheet            // versions 1 to 3: the sheet of the top-level cells
	pending              []json.RawMessage // top-level cells read before the version, in order
	sheets               []readSheet       // version 4: the sheets read
	sheetsRaw            json.RawMessage   // "sheets" read before the version
}

// readSheet is a sheet of a version 4 file, its cells read, and its
// other fields.
type readSheet struct {
	s    *Sheet
	body fileSheet
}

// field is a member of an object, its value as written.
type field struct {
	key string
	raw json.RawMessage
}

func readBook(r io.Reader, trace any) (*Workbook, error) {
	rd := &fileReader{sc: newScanner(r), w: emptyBook()}
	f, err := rd.top()
	if err != nil {
		return nil, err
	}
	if f.Version < 1 || f.Version > fileVersion {
		return nil, fmt.Errorf("unsupported file version %d", f.Version)
	}
	if err := rd.late(); err != nil {
		return nil, err
	}
	w := rd.w
	w.trace = trace
	list := rd.sheets
	if f.Version < 4 {
		if rd.single == nil {
			rd.single = w.newSheet("")
		}
		list = []readSheet{{rd.single, f.fileSheet}}
		if list[0].body.Name == "" {
			list[0].body.Name = "Sheet1"
		}
	}
	if len(list) == 0 {
		return nil, errors.New("the file has no sheets")
	}
	for _, rs := range list {
		if err := w.checkName(nil, rs.body.Name); err != nil {
			return nil, fmt.Errorf("sheet %q: %w", rs.body.Name, err)
		}
		rs.s.name = rs.body.Name
		w.insert(rs.s, len(w.sheets))
	}
	if err := w.readNames(f.Names); err != nil {
		return nil, err
	}
	var old []oldRegion
	for _, rs := range list {
		if err := rs.s.read(rs.body, &old); err != nil {
			if len(list) > 1 {
				err = fmt.Errorf("sheet %s: %w", rs.body.Name, err)
			}
			return nil, err
		}
	}
	w.active = clampInt(f.Active, 0, len(w.sheets)-1)
	w.settleHidden()
	// Anything but "decimal" (say, a mode from a later build) computes in
	// binary, as the file would in a build without the setting.
	w.decimal = f.Arithmetic == "decimal"
	// A locale this build doesn't know follows the default.
	if l, ok := locale.Lookup(f.Locale); ok {
		w.locale = l
	}
	if err := w.readMacros(f.Macros); err != nil {
		return nil, err
	}
	w.macroOrigin = f.MacroOrigin
	w.convertOld(old)
	w.nb.changed = 0 // the outputs read are the file's
	w.RecalcAll()
	return w, nil
}

// top reads the file's object, streaming the cells, and returns its
// other fields.
func (rd *fileReader) top() (fileFormat, error) {
	var f fileFormat
	if null, err := rd.sc.null(); err != nil || null {
		return f, err
	}
	var rest []field
	err := rd.sc.members(func(key string) error {
		switch {
		case strings.EqualFold(key, "version"):
			return rd.readVersion()
		case strings.EqualFold(key, "cells"):
			return rd.topCells()
		case strings.EqualFold(key, "sheets"):
			return rd.topSheets()
		}
		raw, err := rd.sc.raw()
		rest = append(rest, field{key, raw})
		return err
	})
	if err == nil {
		err = unmarshalFields(rest, &f)
	}
	f.Version = rd.version
	return f, err
}

func (rd *fileReader) readVersion() error {
	raw, err := rd.sc.raw()
	if err != nil {
		return err
	}
	v := rd.version
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	if rd.streamed && v != rd.version {
		return errTwice
	}
	rd.version, rd.hasVersion = v, true
	return nil
}

// topCells reads the top-level cells: the only sheet's, before version 4.
func (rd *fileReader) topCells() error {
	if !rd.hasVersion || len(rd.pending) > 0 {
		raw, err := rd.sc.raw()
		rd.pending = append(rd.pending, raw)
		return err
	}
	if rd.version >= 4 {
		raw, err := rd.sc.raw()
		if err == nil {
			err = checkCells(raw)
		}
		return err
	}
	return rd.cells(rd.singleSheet(), rd.sc)
}

func (rd *fileReader) singleSheet() *Sheet {
	if rd.single == nil {
		rd.single = rd.w.newSheet("")
	}
	return rd.single
}

// checkCells checks cells a file of this version ignores, as decoding
// them into the file's fields would.
func checkCells(raw json.RawMessage) error {
	var m map[string]json.RawMessage
	return json.Unmarshal(raw, &m)
}

// topSheets reads the list of sheets of version 4.
func (rd *fileReader) topSheets() error {
	if !rd.hasVersion || rd.version < 4 {
		raw, err := rd.sc.raw()
		if err != nil {
			return err
		}
		if !rd.hasVersion {
			if rd.sheetsRaw != nil {
				return errTwice
			}
			rd.sheetsRaw = raw
			return nil
		}
		var ignored []fileSheet
		return json.Unmarshal(raw, &ignored)
	}
	return rd.readSheets(rd.sc)
}

// errTwice is returned for a version or a list of sheets given twice
// once cells were read by the first.
var errTwice = errors.New("the file gives its version or its sheets twice")

// readSheets reads a list of sheets from sc.
func (rd *fileReader) readSheets(sc *scanner) error {
	if rd.listed {
		return errTwice
	}
	rd.listed, rd.sheetsRaw = true, nil
	if null, err := sc.null(); err != nil || null {
		return err
	}
	return sc.elements(func() error {
		s := rd.w.newSheet("")
		var rest []field
		if null, err := sc.null(); err != nil || null {
			rd.sheets = append(rd.sheets, readSheet{s: s})
			return err
		}
		name := ""
		err := sc.members(func(key string) error {
			if strings.EqualFold(key, "cells") {
				err := rd.cells(s, sc)
				if err != nil && name != "" {
					err = fmt.Errorf("sheet %s: %w", name, err)
				}
				return err
			}
			raw, err := sc.raw()
			if strings.EqualFold(key, "name") {
				json.Unmarshal(raw, &name) // for errors only: the body decodes it
			}
			rest = append(rest, field{key, raw})
			return err
		})
		var body fileSheet
		if err == nil {
			err = unmarshalFields(rest, &body)
		}
		rd.sheets = append(rd.sheets, readSheet{s, body})
		return err
	})
}

// late reads what came before the version, now that it is known.
func (rd *fileReader) late() error {
	for _, raw := range rd.pending {
		if rd.version >= 4 {
			if err := checkCells(raw); err != nil {
				return err
			}
			continue
		}
		if err := rd.cells(rd.singleSheet(), newScanner(bytes.NewReader(raw))); err != nil {
			return err
		}
	}
	if rd.sheetsRaw == nil {
		return nil
	}
	if rd.version < 4 {
		var ignored []fileSheet
		return json.Unmarshal(rd.sheetsRaw, &ignored)
	}
	return rd.readSheets(newScanner(bytes.NewReader(rd.sheetsRaw)))
}

// cells reads a sheet's cells from sc into s. Version 1 had no formats,
// so there entries imply them, as if typed again; since version 2 the
// stored format wins.
func (rd *fileReader) cells(s *Sheet, sc *scanner) error {
	rd.streamed = true
	if null, err := sc.null(); err != nil || null {
		return err
	}
	implied := rd.version < 2
	return sc.fields(func(key []byte, plain bool) error {
		a, name, err := cellKey(key, plain)
		if err != nil {
			return err
		}
		if err := s.readCell(sc, a, implied); err != nil {
			if name == "" {
				name = a.String()
			}
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	})
}

// cellKey is the cell a key of "cells" names, and its name when it isn't
// a's (for errors). A plain key is parsed from its bytes, which are
// valid only until the next read, without making a string of it.
func cellKey(key []byte, plain bool) (Addr, string, error) {
	if plain {
		a, ok := ParseAddr(string(key))
		if !ok {
			return a, "", fmt.Errorf("invalid cell %q", key)
		}
		return a, "", nil
	}
	var name string
	if err := json.Unmarshal(quote(key), &name); err != nil {
		return Addr{}, "", err
	}
	a, ok := ParseAddr(name)
	if !ok {
		return a, "", fmt.Errorf("invalid cell %q", name)
	}
	return a, name, nil
}

// readCell reads the cell at a from sc: a string for an entry alone, an
// object for one with formatting or a note (see fileFormat).
func (s *Sheet) readCell(sc *scanner, a Addr, implied bool) error {
	c, err := sc.peek()
	if err != nil {
		return err
	}
	var e fileEntry
	if c == '"' {
		body, plain, err := sc.text()
		if err != nil {
			return err
		}
		if plain && s.loadNum(a, body, Format{}, Style{}) {
			return nil
		}
		if e.input, err = unquote(body, plain); err != nil {
			return err
		}
	} else {
		raw, err := sc.raw()
		if err != nil {
			return err
		}
		if e, err = decodeNoted(raw); err != nil {
			return err
		}
		if e.note == "" && e.text == nil && s.loadNum(a, []byte(e.input), e.f, e.st) {
			return nil
		}
	}
	cell, err := e.cell(implied)
	if err != nil {
		return err
	}
	// An empty entry leaves no cell, even where the key came before.
	if cell != nil || s.cells.has(a) {
		s.place(a, cell)
	}
	return nil
}

// loadNum stores a number typed plainly ("12.5", "-3", "0.50", the most
// common entry by far) at a, from a file, straight into its slot: what
// newCell and place would store, without making a Cell or a string. It
// reports false for any other entry, a cell a holds with contents, or a
// style that wraps or draws borders (place indexes those by row), all
// of which go the general way. A sheet being read has no undo step,
// pivot or spill yet for place to see to.
func (s *Sheet) loadNum(a Addr, input []byte, f Format, st Style) bool {
	if f.Kind == FmtText || st.shapes() || s.cells.richAt(a) != nil {
		return false
	}
	d, ok := plainForm(string(input))
	if !ok {
		return false
	}
	v, err := strconv.ParseFloat(string(input), 64)
	if err != nil {
		return false
	}
	lk, ok := s.cells.lookID(look{f: f, st: st})
	if !ok {
		return false
	}
	s.version++
	s.cells.setSlot(a, slot{kind: slotNum, num: v, dec: d, look: lk})
	return true
}

// unmarshalFields decodes an object's fields, as written, into v.
func unmarshalFields(fields []field, v any) error {
	if len(fields) == 0 {
		return nil
	}
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range fields {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(jsonString(f.key))
		b.WriteByte(':')
		b.Write(f.raw)
	}
	b.WriteByte('}')
	return json.Unmarshal(b.Bytes(), v)
}
