package sheet

// kindLabel names a format kind in undo labels: "format B3 as date time".
func kindLabel(k FormatKind) string {
	switch k {
	case FmtAuto:
		return "automatic"
	case FmtText:
		return "plain text"
	case FmtDateTime:
		return "date time"
	case FmtCustom:
		return "custom"
	}
	return k.String()
}

// Align is a cell's horizontal alignment.
type Align uint8

const (
	AlignAuto   Align = iota // numbers right, text left, booleans and errors centered
	AlignLeft                //
	AlignCenter              //
	AlignRight               //
	// AlignFill is only returned by Display: the text spans the cell
	// exactly and isn't padded (Accounting's $ at the left edge).
	AlignFill
)

var alignNames = [...]string{"", "left", "center", "right"}

// String returns the alignment's name as stored in files.
func (a Align) String() string {
	if int(a) < len(alignNames) {
		return alignNames[a]
	}
	return ""
}

// ParseAlign is the inverse of Align.String.
func ParseAlign(s string) (Align, bool) {
	for i, n := range alignNames {
		if n == s {
			return Align(i), true
		}
	}
	return AlignAuto, false
}

// Style is a cell's text style. It is a plain value so cells can be
// copied freely.
type Style struct {
	Bold, Italic, Underline, Strikethrough bool
	Align                                  Align

	// own marks a cell's format and style as wholly its own, not falling
	// back on its row's or column's even where they are Automatic or
	// plain; see lines.go.
	own bool
}

// IsZero reports whether s is the default style.
func (s Style) IsZero() bool { return s == Style{} }
