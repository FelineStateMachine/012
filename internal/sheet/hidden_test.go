package sheet

import (
	"bytes"
	"strings"
	"testing"
)

// Hiding a sheet keeps it in formulas, is one undo step each way, and
// the last visible sheet can't be hidden or deleted.
func TestHideSheet(t *testing.T) {
	w := bookOf(t,
		page{"Summary", map[string]string{"A1": "=Data!A1*2"}},
		page{"Data", map[string]string{"A1": "21"}},
	)
	data, summary := w.Lookup("Data"), w.Lookup("Summary")
	if err := w.HideSheet(data); err != nil {
		t.Fatal(err)
	}
	if !data.Hidden() || len(w.Visible()) != 1 || w.HiddenSheets()[0] != data {
		t.Fatalf("hidden %v, visible %d", data.Hidden(), len(w.Visible()))
	}
	if got := show(t, w, "Summary!A1"); got != "42" {
		t.Errorf("formula reading the hidden sheet = %s", got)
	}
	data.Set(at("A1"), "5")
	if got := show(t, w, "Summary!A1"); got != "10" {
		t.Errorf("after editing the hidden sheet = %s", got)
	}
	if err := w.HideSheet(summary); err != errLastVisible {
		t.Errorf("hiding the last visible sheet: %v", err)
	}
	if err := w.DeleteSheet(summary); err != errLastVisible {
		t.Errorf("deleting the last visible sheet: %v", err)
	}
	c, ok := w.Undo()
	if !ok || c.Label != "edit A1" {
		t.Fatalf("undo %+v", c)
	}
	c, ok = w.Undo()
	if !ok || c.Label != "hide sheet Data" || data.Hidden() {
		t.Fatalf("undo hide: %+v, hidden %v", c, data.Hidden())
	}
	w.Redo()
	if !data.Hidden() {
		t.Error("redo didn't hide")
	}
	if err := w.UnhideSheet(data); err != nil || data.Hidden() {
		t.Fatalf("unhide: %v", err)
	}
	if c, _ := w.Undo(); c.Label != "show sheet Data" || !data.Hidden() {
		t.Errorf("undo unhide: %+v", c)
	}
}

// Hidden sheets are saved in an optional field; a file whose sheets are
// all hidden, or that was showing a hidden one, opens on a visible one.
func TestHiddenSheetFile(t *testing.T) {
	w := bookOf(t,
		page{"Summary", map[string]string{"A1": "=Data!A1*2"}},
		page{"Data", map[string]string{"A1": "21"}},
	)
	w.HideSheet(w.Lookup("Data"))
	var b bytes.Buffer
	if err := w.Write(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `"version": 4,`) || strings.Count(out, `"hidden": true`) != 1 ||
		!strings.Contains(out, "\"name\": \"Data\",\n      \"hidden\": true,") {
		t.Fatalf("file:\n%s", out)
	}
	r, err := ReadBook(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Lookup("Data").Hidden() || r.Lookup("Summary").Hidden() || show(t, r, "Summary!A1") != "42" {
		t.Errorf("read back: Data hidden %v", r.Lookup("Data").Hidden())
	}

	all := strings.Replace(out, `"name": "Summary",`, `"name": "Summary", "hidden": true,`, 1)
	all = strings.Replace(all, `"sheets"`, `"active": 1, "sheets"`, 1)
	r, err = ReadBook(strings.NewReader(all))
	if err != nil {
		t.Fatal(err)
	}
	if r.Sheet(0).Hidden() || !r.Sheet(1).Hidden() || r.Active() != 0 {
		t.Errorf("every sheet hidden: first hidden %v, active %d", r.Sheet(0).Hidden(), r.Active())
	}
}
