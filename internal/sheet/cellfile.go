package sheet

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// fileCell is a cell with formatting or a note as the file writes it; a
// cell with neither is just its input (see fileFormat).
type fileCell struct {
	Input         string `json:"input,omitempty"`
	Format        string `json:"format,omitempty"`
	Decimals      *int   `json:"decimals,omitempty"`
	Pattern       string `json:"pattern,omitempty"`
	Bold          bool   `json:"bold,omitempty"`
	Italic        bool   `json:"italic,omitempty"`
	Underline     bool   `json:"underline,omitempty"`
	Strikethrough bool   `json:"strikethrough,omitempty"`
	Align         string `json:"align,omitempty"`
	Own           bool   `json:"own,omitempty"` // not taking its column's or row's formatting
	// Note needs no version bump: earlier builds ignore it and drop the
	// note.
	Note string `json:"note,omitempty"`
}

func encodeCell(c *Cell) (json.RawMessage, error) {
	if c.Format.IsZero() && c.Style.IsZero() && c.Note == "" {
		return json.Marshal(c.Input)
	}
	fc := fileCell{
		Input:         c.Input,
		Pattern:       c.Format.Pattern,
		Bold:          c.Style.Bold,
		Italic:        c.Style.Italic,
		Underline:     c.Style.Underline,
		Strikethrough: c.Style.Strikethrough,
		Align:         c.Style.Align.String(),
		Own:           c.Style.own,
		Note:          c.Note,
	}
	if !c.Format.IsZero() {
		fc.Format = c.Format.Kind.String()
	}
	if c.Format.Kind.HasDecimals() {
		d := c.Format.Decimals
		fc.Decimals = &d
	}
	return json.Marshal(fc)
}

// decodeCell reads a cell's entry and formatting, as line formats are
// stored too.
func decodeCell(raw json.RawMessage) (string, Format, Style, error) {
	input, f, st, _, err := decodeNoted(raw)
	return input, f, st, err
}

// decodeNoted is decodeCell with the cell's note.
func decodeNoted(raw json.RawMessage) (string, Format, Style, string, error) {
	var input string
	if err := json.Unmarshal(raw, &input); err == nil {
		return input, Format{}, Style{}, "", nil
	}
	var fc fileCell
	if err := json.Unmarshal(raw, &fc); err != nil {
		return "", Format{}, Style{}, "", err
	}
	input, f, st, err := decodeFormatted(fc)
	return input, f, st, fc.Note, err
}

// decodeFormatted reads the entry and formatting of a cell written as an
// object.
func decodeFormatted(fc fileCell) (string, Format, Style, error) {
	var f Format
	if fc.Format != "" {
		k, ok := ParseFormatKind(fc.Format)
		if !ok {
			return "", Format{}, Style{}, fmt.Errorf("unknown format %q", fc.Format)
		}
		f = Format{Kind: k, Pattern: fc.Pattern}
		if fc.Decimals != nil {
			f.Decimals = clampInt(*fc.Decimals, 0, MaxDecimals)
		}
	}
	al, ok := ParseAlign(fc.Align)
	if !ok {
		return "", Format{}, Style{}, fmt.Errorf("unknown alignment %q", fc.Align)
	}
	st := Style{Bold: fc.Bold, Italic: fc.Italic, Underline: fc.Underline, Strikethrough: fc.Strikethrough, Align: al, own: fc.Own}
	return fc.Input, f, st, nil
}

// appendSaved appends the cell at a, which holds one, as the file
// writes it, or reports false for a cell the file leaves out (a pivot's
// or a spill's result). A plain cell without formatting is written from
// its slot, without making a Cell.
func (st *cellStore) appendSaved(buf []byte, a Addr) ([]byte, bool, error) {
	b, i := st.find(a)
	if sl := b.vals[i]; sl.kind != slotRich && sl.look == 0 {
		return st.appendInput(buf, sl), true, nil
	}
	c := st.get(a).saved()
	if c == nil {
		return buf, false, nil
	}
	raw, err := encodeCell(c)
	return append(buf, raw...), true, err
}

// appendInput appends a plain slot's entry as a JSON string.
func (st *cellStore) appendInput(buf []byte, sl slot) []byte {
	switch {
	case sl.kind == slotBlank:
		return append(buf, '"', '"')
	case sl.ref != 0:
		return appendJSONString(buf, st.strs.strs[sl.ref])
	case sl.kind == slotBool:
		return append(append(append(buf, '"'), boolText(sl.num != 0)...), '"')
	}
	buf = strconv.AppendFloat(append(buf, '"'), sl.num, 'f', int(sl.dec)-1, 64)
	return append(buf, '"')
}

// appendJSONString appends s as json.Marshal writes it: as it is between
// quotes when it is printable ASCII that needs no escape (HTML's
// characters included, which json.Marshal escapes), and through
// json.Marshal otherwise.
func appendJSONString(buf []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c > 0x7e || c == '"' || c == '\\' || c == '<' || c == '>' || c == '&' {
			raw, _ := json.Marshal(s)
			return append(buf, raw...)
		}
	}
	return append(append(append(buf, '"'), s...), '"')
}

// appendAddr appends a's A1 name, as a.String() writes it.
func appendAddr(buf []byte, a Addr) []byte {
	var col [4]byte
	i := len(col)
	for c := a.Col + 1; c > 0; c = (c - 1) / 26 {
		i--
		col[i] = byte('A' + (c-1)%26)
	}
	return strconv.AppendInt(append(buf, col[i:]...), int64(a.Row+1), 10)
}
