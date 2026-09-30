package cowork

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/mcp"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/room"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// person is a participant standing for a session.
type person struct{ name string }

func (p *person) Name() string { return p.name }
func (p *person) Notify()      {}

// bot is one of the agents participant without a connection.
type bot struct{ person }

func (*bot) Agent() {}

// liveRoom is a room with ann in it, on a workbook of 1, 2, 3 in A1:A3
// and their sum in A4, and the agent attached.
func liveRoom(t *testing.T) (ann *room.Seat, agent *Agent) {
	t.Helper()
	g := room.NewRegistry(room.Edit)
	ann, _ = g.Join("k", &person{"ann"}, func() *sheet.Workbook {
		w := sheet.New().Book()
		for i, in := range []string{"1", "2", "3", "=SUM(A1:A3)"} {
			w.Sheet(0).Set(sheet.Addr{Row: i}, in)
		}
		w.AddSheet("Q3", 1)
		return w
	})
	ann.Do(func(*sheet.Workbook) { ann.SetSaved(room.Saved{Name: "budget.012"}) })
	seat, ok := g.JoinOpen("k", &bot{person{"claude"}})
	if !ok {
		t.Fatal("the agent couldn't join")
	}
	return ann, NewAgent(seat)
}

func set(entries ...string) func(*sheet.Workbook) error {
	return func(w *sheet.Workbook) error {
		var es []headless.Entry
		for i := 0; i+1 < len(entries); i += 2 {
			es = append(es, headless.Entry{Ref: entries[i], Input: entries[i+1]})
		}
		_, err := headless.Set(w, es, headless.SetOptions{})
		return err
	}
}

func value(s *room.Seat, sheetName, ref string) sheet.Value {
	var v sheet.Value
	s.Do(func(w *sheet.Workbook) {
		a, _ := sheet.ParseAddr(ref)
		v = w.Lookup(sheetName).Value(a)
	})
	return v
}

func TestSuggestionWaitsForThePerson(t *testing.T) {
	ann, a := liveRoom(t)
	made, err := a.Make(context.Background(), mcp.Making{Label: "set", Message: "fix the totals", Fn: set("A1", "10", "A2", "20")})
	if err != nil {
		t.Fatal(err)
	}
	if made.Applied || made.Suggestion != 1 || len(made.Changes) == 0 {
		t.Fatalf("made %+v", made)
	}
	if v := value(ann, "Sheet1", "A4"); v.Num != 6 {
		t.Fatalf("the suggestion was made: A4 is %v", v)
	}
	ann.Do(func(*sheet.Workbook) {
		b := BoardOf(ann)
		s, i, ok := b.At("Sheet1", sheet.Addr{Row: 1})
		if !ok || s.ID != 1 || i != 1 || s.Message != "fix the totals" || s.Agent != "claude" {
			t.Errorf("A2's suggestion: %+v %d %v", s, i, ok)
		}
	})
}

func TestSuggestionAcceptedIsTheAgentsStep(t *testing.T) {
	ann, a := liveRoom(t)
	a.Make(context.Background(), mcp.Making{Label: "set", Fn: set("A1", "10", "A2", "20")})
	ann.Do(func(w *sheet.Workbook) {
		if err := BoardOf(ann).Accept(w, 1, nil); err != nil {
			t.Fatal(err)
		}
		if w.CanUndo() {
			t.Error("ann can undo the agent's step")
		}
	})
	if v := value(ann, "Sheet1", "A4"); v.Num != 33 {
		t.Fatalf("A4 is %v after accepting", v)
	}
	news := a.report()
	if len(news) != 1 || !strings.Contains(news[0], "was accepted") {
		t.Fatalf("news %v", news)
	}
	if label, err := a.Undo(context.Background()); err != nil || label != "set" {
		t.Fatalf("the agent's undo: %q %v", label, err)
	}
	if v := value(ann, "Sheet1", "A4"); v.Num != 6 {
		t.Fatalf("A4 is %v after the agent's undo", v)
	}
}

func TestSuggestionByCell(t *testing.T) {
	ann, a := liveRoom(t)
	a.Make(context.Background(), mcp.Making{Label: "set", Fn: set("A1", "10", "A2", "20", "A3", "30")})
	ann.Do(func(w *sheet.Workbook) {
		b := BoardOf(ann)
		if err := b.Accept(w, 1, []int{0}); err != nil {
			t.Fatal(err)
		}
		if err := b.Reject(1, []int{1}); err != nil {
			t.Fatal(err)
		}
		if !b.Get(1).Pending() {
			t.Error("A3 isn't pending")
		}
		b.Accept(w, 1, nil) // the rest: A3
	})
	if v := value(ann, "Sheet1", "A4"); v.Num != 42 {
		t.Fatalf("A4 is %v: 10 + 2 + 30", v)
	}
	list, _ := a.Suggestions(context.Background())
	if len(list) != 1 || list[0].State != "accepted in part" || list[0].Accepted != 2 || list[0].Rejected != 1 {
		t.Fatalf("suggestions %+v", list)
	}
}

func TestSuggestionRejected(t *testing.T) {
	ann, a := liveRoom(t)
	a.Make(context.Background(), mcp.Making{Label: "set", Fn: set("A1", "10")})
	ann.Do(func(*sheet.Workbook) { BoardOf(ann).Reject(1, nil) })
	made, _ := a.Make(context.Background(), mcp.Making{Label: "set", Fn: set("B1", "x")})
	if len(made.Notices) != 1 || !strings.Contains(made.Notices[0], "Suggestion 1 (set) was rejected") {
		t.Fatalf("notices %v", made.Notices)
	}
	if v := value(ann, "Sheet1", "A1"); v.Num != 1 {
		t.Fatal("a rejected suggestion changed A1")
	}
	ann.Do(func(w *sheet.Workbook) {
		if err := BoardOf(ann).Accept(w, 1, nil); !errors.Is(err, ErrGone) {
			t.Errorf("accepting a rejected suggestion: %v", err)
		}
	})
}

func TestDirectEdits(t *testing.T) {
	ann, a := liveRoom(t)
	ann.Do(func(*sheet.Workbook) { BoardOf(ann).Direct = true })
	made, err := a.Make(context.Background(), mcp.Making{Label: "set", Fn: set("A1", "10")})
	if err != nil || !made.Applied || made.Suggestion != 0 {
		t.Fatalf("made %+v, %v", made, err)
	}
	if v := value(ann, "Sheet1", "A4"); v.Num != 15 {
		t.Fatalf("A4 is %v", v)
	}
	var where room.Presence
	ann.Do(func(*sheet.Workbook) { where = ann.Peers()[0].Presence })
	if where.Cursor != (sheet.Addr{}) || where.Sheet == nil {
		t.Errorf("the agent's pointer isn't on its change: %+v", where)
	}
}

func TestScopeRefuses(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scope func(w *sheet.Workbook) Scope
		fn    func(*sheet.Workbook) error
		want  string
	}{
		{"read only", func(*sheet.Workbook) Scope { return Scope{Kind: ReadOnly} }, set("A1", "5"), "read only"},
		{"another sheet", func(w *sheet.Workbook) Scope { return Scope{Kind: OneSheet, Sheet: w.Lookup("Q3")} }, set("A1", "5"), "Sheet1!A1, outside what the person let you change (sheet Q3)"},
		{"outside the range", func(w *sheet.Workbook) Scope {
			return Scope{Kind: OneRange, Sheet: w.Sheet(0), Range: sheet.Rect{To: sheet.Addr{Row: 2}}}
		}, set("B1", "5"), "Sheet1!B1, outside what the person let you change (Sheet1!A1:A3)"},
		{"rows in a range", func(w *sheet.Workbook) Scope {
			return Scope{Kind: OneRange, Sheet: w.Sheet(0), Range: sheet.Rect{To: sheet.Addr{Row: 2}}}
		}, func(w *sheet.Workbook) error { return w.Sheet(0).InsertRows(0, 1) }, "inserts or deletes rows"},
		{"a sheet added", func(w *sheet.Workbook) Scope { return Scope{Kind: OneSheet, Sheet: w.Sheet(0)} },
			func(w *sheet.Workbook) error { _, err := w.AddSheet("New", 2); return err }, "adds, deletes, renames or moves sheets"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ann, a := liveRoom(t)
			ann.Do(func(w *sheet.Workbook) { BoardOf(ann).Scope = tc.scope(w) })
			_, err := a.Make(context.Background(), mcp.Making{Label: "x", Fn: tc.fn})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
	ann, a := liveRoom(t)
	ann.Do(func(w *sheet.Workbook) { BoardOf(ann).Scope = Scope{Kind: OneSheet, Sheet: w.Sheet(0)} })
	if _, err := a.Make(context.Background(), mcp.Making{Label: "x", Fn: set("C3", "5")}); err != nil {
		t.Fatalf("inside the sheet: %v", err)
	}
	if _, err := a.Focus(context.Background(), "Q3!A1"); err == nil {
		t.Fatal("focus outside the scope")
	}
	if at, err := a.Focus(context.Background(), "B2:C3"); err != nil || at != "Sheet1!B2:C3" {
		t.Fatalf("focus: %q %v", at, err)
	}
}

// answer answers the first question on the board as a session would.
func answer(t *testing.T, ann *room.Seat, ans Answer) *Ask {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var q *Ask
		ann.Do(func(*sheet.Workbook) {
			b := BoardOf(ann)
			if q = b.NextAsk(ann.ID(), func(int) bool { return true }); q != nil {
				b.Answer(q, ans)
			}
		})
		if q != nil {
			return q
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no question came")
	return nil
}

func TestAskReturnsTheAnswer(t *testing.T) {
	ann, a := liveRoom(t)
	got := make(chan *sdk.ElicitResult, 1)
	go func() {
		res, err := a.Ask(context.Background(), &sdk.ElicitParams{Message: "Overwrite B2:B40?",
			RequestedSchema: map[string]any{"type": "object", "properties": map[string]any{
				"sure":  map[string]any{"type": "boolean", "title": "Sure"},
				"which": map[string]any{"type": "string", "enum": []string{"a", "b"}}}, "required": []string{"which"}}})
		if err != nil {
			t.Error(err)
		}
		got <- res
	}()
	q := answer(t, ann, Answer{Action: "accept", Content: map[string]any{"sure": true, "which": "b"}})
	if q.Message != "Overwrite B2:B40?" || len(q.Fields) != 2 || q.Fields[0].Name != "which" || q.Fields[0].Kind != Choice || q.Fields[1].Kind != Boolean {
		t.Fatalf("question %+v", q)
	}
	res := <-got
	if res.Action != "accept" || res.Content["which"] != "b" {
		t.Fatalf("result %+v", res)
	}
}

func TestAskWithdrawnWhenTheAgentGivesUp(t *testing.T) {
	ann, a := liveRoom(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := a.Ask(ctx, &sdk.ElicitParams{Message: "Sure?"})
		done <- err
	}()
	for n := 0; n == 0; {
		ann.Do(func(*sheet.Workbook) { n = len(BoardOf(ann).Asks()) })
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ann.Do(func(*sheet.Workbook) {
		if n := len(BoardOf(ann).Asks()); n != 0 {
			t.Errorf("%d questions left", n)
		}
	})
}

func TestAskRefusesSchemasItCantShow(t *testing.T) {
	_, a := liveRoom(t)
	_, err := a.Ask(context.Background(), &sdk.ElicitParams{Message: "Where?",
		RequestedSchema: map[string]any{"type": "object", "properties": map[string]any{"at": map[string]any{"type": "array"}}}})
	if err == nil || !strings.Contains(err.Error(), "questions take strings") {
		t.Fatal(err)
	}
}

func TestJEVDirectAsksFirst(t *testing.T) {
	ann, a := liveRoom(t)
	ann.Do(func(*sheet.Workbook) { BoardOf(ann).Direct = true })
	done := make(chan error, 1)
	go func() {
		_, err := a.Make(context.Background(), mcp.Making{Label: "ask", Fn: set("B1", `=JEV.TEST(A1, "odd?")`)})
		done <- err
	}()
	q := answer(t, ann, Answer{Action: "accept", Always: true})
	if q.Kind != Grant || q.Grant != "jev" {
		t.Fatalf("question %+v", q)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	ann.Do(func(*sheet.Workbook) {
		if !BoardOf(ann).Grants.JEV {
			t.Error("always didn't grant JEV for the session")
		}
	})
}

func TestNotebookRunsAskAndGoToTheKeeper(t *testing.T) {
	ann, a := liveRoom(t)
	ann.Do(func(w *sheet.Workbook) { w.AddNotebook("Notes", w.Sheet(0)) })
	ann.Do(func(w *sheet.Workbook) {
		w.Lookup("Notes").SetNotebookCells("add", []notebook.Cell{{Source: "ls"}})
	})
	if _, err := a.RunCell(context.Background(), "Nope", 1); err == nil {
		t.Fatal("ran a cell of no notebook")
	}
	done := make(chan error, 1)
	go func() {
		_, err := a.RunCell(context.Background(), "Notes", 1)
		done <- err
	}()
	q := answer(t, ann, Answer{Action: "decline"})
	if q.Grant != "notebooks" {
		t.Fatalf("question %+v", q)
	}
	if err := <-done; err == nil {
		t.Fatal("a declined run went ahead")
	}
}
