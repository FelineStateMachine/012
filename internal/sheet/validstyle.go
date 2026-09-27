package sheet

import (
	"errors"
	"slices"
	"strings"
)

// How dropdowns and checkboxes look, as Sheets' Data validation offers
// it: a dropdown's cells show an arrow, their value as a chip, or plain
// text; a checkbox may enter values of its own for checked and
// unchecked rather than TRUE and FALSE.

// DropDisplay is how a dropdown's cells show that they have one.
type DropDisplay uint8

const (
	DropArrow DropDisplay = iota // ▾ at the cell's right
	DropChip                     // the value on a chip, with its ▾, as Sheets draws them
	DropPlain                    // the value alone; Alt+Down still opens the list
	numDropDisplays
)

var dropNames = [numDropDisplays]string{"", "chip", "plain"}
var dropTitles = [numDropDisplays]string{"Arrow", "Chip", "Plain text"}

func (d DropDisplay) String() string {
	if d >= numDropDisplays {
		return ""
	}
	return dropNames[d]
}

// Title names the display for people, e.g. "Chip".
func (d DropDisplay) Title() string {
	if d >= numDropDisplays {
		return ""
	}
	return dropTitles[d]
}

// ParseDropDisplay is the inverse of DropDisplay.String.
func ParseDropDisplay(s string) (DropDisplay, bool) {
	i := slices.Index(dropNames[:], s)
	return DropDisplay(max(i, 0)), i >= 0
}

// DropDisplays lists the displays in the order the rules editor offers
// them.
func DropDisplays() []DropDisplay { return []DropDisplay{DropChip, DropArrow, DropPlain} }

// CheckboxValues are what a checkbox rule's cells hold checked and
// unchecked: TRUE and FALSE, or its own values (Items), the unchecked
// one "" for a blank cell.
func (v Validation) CheckboxValues() (checked, unchecked string, custom bool) {
	if v.Kind != ValidCheckbox || len(v.Items) == 0 {
		return "TRUE", "FALSE", false
	}
	if len(v.Items) > 1 {
		unchecked = v.Items[1]
	}
	return v.Items[0], unchecked, true
}

// checkCheckbox checks a checkbox's own values: a checked value, and an
// unchecked one that differs from it.
func (v Validation) checkCheckbox() error {
	switch on, off, custom := v.CheckboxValues(); {
	case !custom:
		return nil
	case len(v.Items) > 2:
		return errors.New("A checkbox has a checked and an unchecked value")
	case strings.TrimSpace(on) == "":
		return errors.New("Enter the value a checked box holds")
	case strings.EqualFold(strings.TrimSpace(on), strings.TrimSpace(off)):
		return errors.New("The checked and unchecked values must differ")
	}
	return nil
}

// isChecked reports whether a checkbox cell of value v, shown as shown,
// is checked by its rule.
func (v Validation) isChecked(val Value, shown string) bool {
	on, _, custom := v.CheckboxValues()
	if !custom {
		return val.Kind == Bool && val.Num != 0
	}
	return strings.EqualFold(strings.TrimSpace(shown), strings.TrimSpace(on))
}

// checkboxValid reports whether a checkbox cell may hold v, shown as
// shown: its checked or unchecked value, or a blank.
func (v Validation) checkboxValid(val Value, shown string) bool {
	if val.Kind == Empty {
		return true
	}
	on, off, custom := v.CheckboxValues()
	if !custom {
		return val.Kind == Bool
	}
	shown = strings.TrimSpace(shown)
	return strings.EqualFold(shown, strings.TrimSpace(on)) || off != "" && strings.EqualFold(shown, strings.TrimSpace(off))
}

// CheckboxInput is what checking (on) or unchecking the checkbox at a
// enters: TRUE or FALSE, or its rule's own values.
func (s *Sheet) CheckboxInput(a Addr, on bool) string {
	v, _ := s.Validation(a)
	checked, unchecked, _ := v.CheckboxValues()
	if on {
		return checked
	}
	return unchecked
}
