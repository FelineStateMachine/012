package srcview

import tea "charm.land/bubbletea/v2"

// Key moves the active cell as a sheet's keys do, reporting whether the
// key was the view's: the arrows, Page Up and Down, Home and End, and
// with Ctrl the data's edges, which on a source are its first and last
// rows and columns.
func (v *View) Key(k tea.KeyPressMsg) bool {
	page := int64(max(v.Lines()-1, 1))
	switch k.String() {
	case "up":
		v.Move(-1, 0)
	case "down", "enter":
		v.Move(1, 0)
	case "left", "shift+tab":
		v.Move(0, -1)
	case "right", "tab":
		v.Move(0, 1)
	case "pgup":
		v.Scroll(-page)
		v.Move(-page, 0)
	case "pgdown":
		v.Scroll(page)
		v.Move(page, 0)
	case "home", "ctrl+left":
		v.MoveTo(v.cur, 0)
	case "end", "ctrl+right":
		v.MoveTo(v.cur, -1)
	case "ctrl+up":
		v.MoveTo(0, v.col)
	case "ctrl+down":
		v.MoveTo(-1, v.col)
	case "ctrl+home":
		v.MoveTo(0, 0)
	case "ctrl+end":
		v.MoveTo(-1, -1)
	default:
		return false
	}
	return true
}
