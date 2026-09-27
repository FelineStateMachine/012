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

// Wrap is what a cell's text does when it is wider than its column, as
// Sheets' Format > Wrapping.
type Wrap uint8

const (
	WrapOverflow Wrap = iota // run on into blank neighbors, Sheets' default
	WrapOn                   // break into lines; the row grows to fit them
	WrapClip                 // cut at the cell's edge
)

var wrapNames = [...]string{"", "wrap", "clip"}

// String returns the wrapping's name as stored in files.
func (w Wrap) String() string {
	if int(w) < len(wrapNames) {
		return wrapNames[w]
	}
	return ""
}

// ParseWrap is the inverse of Wrap.String.
func ParseWrap(s string) (Wrap, bool) {
	for i, n := range wrapNames {
		if n == s {
			return Wrap(i), true
		}
	}
	return WrapOverflow, false
}

// VAlign is where a cell's text sits in a row taller than it, as
// Sheets' Format > Alignment's top, middle and bottom.
type VAlign uint8

const (
	VAlignAuto   VAlign = iota // the bottom, as in Sheets; the middle of a merged cell
	VAlignTop                  //
	VAlignMiddle               //
	VAlignBottom               //
)

var valignNames = [...]string{"", "top", "middle", "bottom"}

// String returns the vertical alignment's name as stored in files.
func (v VAlign) String() string {
	if int(v) < len(valignNames) {
		return valignNames[v]
	}
	return ""
}

// ParseVAlign is the inverse of VAlign.String.
func ParseVAlign(s string) (VAlign, bool) {
	for i, n := range valignNames {
		if n == s {
			return VAlign(i), true
		}
	}
	return VAlignAuto, false
}

// Offset is the line, of n a row has, where the first of k lines of
// text starts: the top, the middle or the bottom, the bottom for
// VAlignAuto unless merged is set, which centers it.
func (v VAlign) Offset(k, n int, merged bool) int {
	switch {
	case v == VAlignTop:
		return 0
	case v == VAlignMiddle, v == VAlignAuto && merged:
		return max(n-k, 0) / 2
	}
	return max(n-k, 0)
}

// Style is a cell's text style, with its wrapping, alignment and
// borders. It is a plain value so cells can be copied freely, twelve
// bytes, which the end of a Cell has room for (see Cell).
type Style struct {
	Bold, Italic, Underline, Strikethrough bool
	Align                                  Align
	Wrap                                   Wrap
	VAlign                                 VAlign

	// own marks a cell's format and style as wholly its own, not falling
	// back on its row's or column's even where they are Automatic or
	// plain; see lines.go.
	own bool

	Borders Borders // see borders.go
}

// IsZero reports whether s is the default style.
func (s Style) IsZero() bool { return s == Style{} }
