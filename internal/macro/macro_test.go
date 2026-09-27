package macro

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fakeHost records the calls it gets and answers from a few cells.
type fakeHost struct {
	calls []string
	cells map[string]any
}

func (f *fakeHost) log(format string, args ...any) error {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
	return nil
}

func (f *fakeHost) Values(ref string) ([][]any, error) {
	if ref == "bad" {
		return nil, errors.New("not a cell or range: bad")
	}
	if strings.Contains(ref, ":") {
		return [][]any{{f.cells["A1"], f.cells["B1"]}}, nil
	}
	return [][]any{{f.cells[ref]}}, nil
}
func (f *fakeHost) Formulas(ref string) ([][]string, error) { return [][]string{{"=1+1"}}, nil }
func (f *fakeHost) SetInput(ref, in string) error {
	f.cells[ref] = in
	return f.log("set %s %q", ref, in)
}
func (f *fakeHost) SetInputs(ref string, rows [][]string) error { return f.log("set %s %q", ref, rows) }
func (f *fakeHost) SetFormula(ref, fm string) error             { return f.log("formula %s %s", ref, fm) }
func (f *fakeHost) Clear(ref string) error                      { return f.log("clear %q", ref) }
func (f *fakeHost) NumberFormat(string) (string, int, error)    { return "currency", 2, nil }
func (f *fakeHost) SetNumberFormat(ref, kind string, d int, p string) error {
	return f.log("format %s %s %d %q", ref, kind, d, p)
}
func (f *fakeHost) Selection() (string, string)         { return "B2:C3", "B2" }
func (f *fakeHost) Select(ref, active string) error     { return f.log("select %s %q", ref, active) }
func (f *fakeHost) Move(c, r int) error                 { return f.log("move %d %d", c, r) }
func (f *fakeHost) Extend(c, r int, whole string) error { return f.log("extend %d %d %q", c, r, whole) }
func (f *fakeHost) Jump(to string, ext bool) error      { return f.log("jump %s %v", to, ext) }
func (f *fakeHost) Enter(text string, fill bool, origin string) error {
	return f.log("enter %q %v %q", text, fill, origin)
}
func (f *fakeHost) PasteText(text string) error          { return f.log("paste %q", text) }
func (f *fakeHost) Sheets() []string                     { return []string{"Sheet1", "Two"} }
func (f *fakeHost) ActiveSheet() string                  { return "Sheet1" }
func (f *fakeHost) ActivateSheet(name string) error      { return f.log("activate %s", name) }
func (f *fakeHost) AddSheet(name string) (string, error) { return "Sheet3", f.log("add %q", name) }
func (f *fakeHost) MoveSheet(pos int) error              { return f.log("move sheet %d", pos) }
func (f *fakeHost) Run(id string, answer *string) error {
	if answer == nil {
		return f.log("run %s", id)
	}
	return f.log("run %s %q", id, *answer)
}
func (f *fakeHost) SetWidth(cols string, w int) error { return f.log("width %s %d", cols, w) }
func (f *fakeHost) Fill(to string, rows, cols int) error {
	return f.log("fill %q %d %d", to, rows, cols)
}

func newFake() *fakeHost { return &fakeHost{cells: map[string]any{"A1": 2.0, "B1": "x", "C1": 2.5}} }

func TestSourceRunsAsRecorded(t *testing.T) {
	actions := []Action{
		Call("select", "B2"),
		Call("enter", "=SUM(A1:A3)"),
		Call("move", 0, 1),
		Call("extend", 2, 0).With("whole", "columns"),
		Call("jump", "down").With("extend", true),
		Call("enter", "total").With("fill", true),
		Call("enter", "=A1").With("origin", "B2"),
		Call("paste_text", "a\tb\n\"c\""),
		Note("not recorded: sort A1:C9"),
		Call("run", "column.width").With("answer", "12"),
		Call("run", "format.bold"),
		Call("activate_sheet", "Q3 plan"),
		Call("set_width", "B:C", 14),
		Call("fill").With("rows", 3),
		Call("move_sheet", 2),
	}
	src := Source("Recorded with absolute references.\n\nSecond line", actions)
	if !strings.HasPrefix(src, "# Recorded with absolute references.\n#\n# Second line\nselect(\"B2\")\n") {
		t.Errorf("source:\n%s", src)
	}
	f := newFake()
	if _, err := New("m", src, Env{Host: f}).Exec(); err != nil {
		t.Fatalf("%v\n%s", err, src)
	}
	want := []string{
		`select B2 ""`, `enter "=SUM(A1:A3)" false ""`, "move 0 1", `extend 2 0 "columns"`, "jump down true",
		`enter "total" true ""`, `enter "=A1" false "B2"`, `paste "a\tb\n\"c\""`, `run column.width "12"`, "run format.bold",
		"activate Q3 plan", "width B:C 14", `fill "" 3 0`, "move sheet 2",
	}
	if strings.Join(f.calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(f.calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestScriptAPI(t *testing.T) {
	src := `
a = get("A1")
b = get("B1")
row = get("A1:B1")
set("D1", a * 3)
set("D2", get("C1") * 2)
set("D3", True)
set("D4", None)
set("E1:F2", [[1, "two"], [3.5, False]])
set_formula("G1:G9", "A1*2")
set("H1", "%s %s %s" % (type(a), b, row))
set("H2", get_formula("G1") + get_number_format("A1"))
set("H3", selection() + " " + active_cell() + " " + active_sheet() + " " + ",".join(sheets()))
set("H4", offset("A1", 2, 3) + " " + offset("'Q3 plan'!A1:B2", 1, 0))
number_format("A1:A3", "percent", decimals=1)
clear()
x = add_sheet()
print("done", x)
`
	f := newFake()
	var printed []string
	st, err := New("api", src, Env{Host: f, Print: func(s string) { printed = append(printed, s) }}).Exec()
	if err != nil {
		t.Fatal(err)
	}
	for ref, want := range map[string]string{
		"D1": "6", "D2": "5", "D3": "TRUE", "D4": "", "H1": `int x [[2, "x"]]`, "H2": "=1+1currency",
		"H3": "B2:C3 B2 Sheet1 Sheet1,Two", "H4": "C4 'Q3 plan'!B1:C2",
	} {
		if got := f.cells[ref]; got != want {
			t.Errorf("%s = %q, want %q", ref, got, want)
		}
	}
	for _, want := range []string{`set E1:F2 [["1" "two"] ["3.5" "FALSE"]]`, "formula G1:G9 =A1*2", `format A1:A3 percent 1 ""`, `clear ""`, `add ""`} {
		if !strings.Contains(strings.Join(f.calls, "\n"), want) {
			t.Errorf("no call %q in\n%s", want, strings.Join(f.calls, "\n"))
		}
	}
	if len(printed) != 1 || printed[0] != "done Sheet3" {
		t.Errorf("printed %q", printed)
	}
	if st.Calls < 15 || st.Steps == 0 {
		t.Errorf("stats %+v", st)
	}
}

func TestErrorsHavePositions(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		compiles  bool
	}{
		{"move(0, 1)\nset(\"A1\"", "m:2:9: got end of file, want ')'", false},
		{"x = 1\nnope(2)", "m:2:1: undefined: nope", false},
		{"move(0, 1)\n  get(\"bad\")", "m:2:3: got indent, want primary expression", false},
		{"move()\nget(\"bad\")", "m:2:4: get: not a cell or range: bad", true},
		{"def f():\n    jump(\"sideways\")\nf()", "m:2:9: jump: to is one of up, down, left, right, home, start, end, not \"sideways\"", true},
		{"set(\"A1\", {})", "m:1:4: set: a cell takes a number, text, True, False or None, not dict", true},
		{"move(1, 2, 3)", "m:1:5: move: got 3 arguments, want at most 2", true},
	} {
		_, err := New("m", tc.src, Env{Host: newFake()}).Exec()
		if err == nil || err.Error() != tc.want {
			t.Errorf("%q: %v, want %q", tc.src, err, tc.want)
		}
		if err := Check("m", tc.src); (err == nil) != tc.compiles {
			t.Errorf("Check(%q) = %v", tc.src, err)
		}
	}
}

func TestStepLimitStopsALoop(t *testing.T) {
	_, err := New("loop", "while True:\n    pass\n", Env{Host: newFake(), MaxSteps: 10_000}).Exec()
	var me *Error
	if !errors.As(err, &me) || !me.Limit || !strings.HasPrefix(err.Error(), "loop:1:1: stopped after 1000") {
		t.Fatalf("err %v", err)
	}
}

func TestCancelStopsARun(t *testing.T) {
	r := New("spin", "while True:\n    move(0, 0)\n", Env{Host: newFake(), MaxSteps: 1 << 62})
	done := make(chan error)
	go func() { _, err := r.Exec(); done <- err }()
	time.Sleep(10 * time.Millisecond)
	r.Cancel()
	select {
	case err := <-done:
		var me *Error
		if !errors.As(err, &me) || !me.Cancel || !strings.Contains(err.Error(), "stopped with Esc") {
			t.Fatalf("err %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel didn't stop the run")
	}
}

func TestNoWayOut(t *testing.T) {
	// Starlark has no file, network, time or random access of its own,
	// and without a loader nothing else can be brought in.
	for _, name := range []string{"open", "time", "random", "exec", "eval", "load_module", "__import__"} {
		if err := Check("m", name+"()"); err == nil {
			t.Errorf("%s is reachable", name)
		}
	}
	_, err := New("m", `load("time.star", "now")`, Env{Host: newFake()}).Exec()
	if err == nil {
		t.Error("load works")
	}
}
