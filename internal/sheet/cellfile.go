package sheet

import (
	"encoding/json"
	"fmt"
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
