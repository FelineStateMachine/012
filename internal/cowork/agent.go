package cowork

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/diff"
	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/mcp"
	"github.com/FelineStateMachine/012/internal/room"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Agent is an attached agent's side of the room: its seat, and the
// mcp.Live its tools stand on. Every call is a turn of its seat, so its
// reads and changes take their place among the people's, and its steps
// are its own.
type Agent struct {
	seat *room.Seat
	book string // the workbook's name, for the tools

	mu      sync.Mutex
	notices []string // news of its suggestions, for its next write
}

var _ mcp.Live = (*Agent)(nil)

// NewAgent is the agent sitting at seat.
func NewAgent(seat *room.Seat) *Agent {
	a := &Agent{seat: seat}
	seat.Do(func(*sheet.Workbook) {
		name := seat.Saved().Name
		a.book = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	})
	if a.book == "" || a.book == "." {
		a.book = "Untitled"
	}
	return a
}

// Name is the workbook's.
func (a *Agent) Name() string { return a.book }

// Seat is the agent's seat.
func (a *Agent) Seat() *room.Seat { return a.seat }

// View runs fn on the room's workbook in a turn.
func (a *Agent) View(_ context.Context, fn func(*sheet.Workbook) error) error {
	var err error
	a.seat.Do(func(w *sheet.Workbook) { err = fn(w) })
	return err
}

// Scratch runs fn on a copy of the workbook taken in a turn.
func (a *Agent) Scratch(_ context.Context, fn func(*sheet.Workbook) error) error {
	var data bytes.Buffer
	var err error
	a.seat.Do(func(w *sheet.Workbook) { err = w.Write(&data) })
	if err != nil {
		return err
	}
	s, err := sheet.Read(&data)
	if err != nil {
		return err
	}
	return fn(s.Book())
}

// Change is Make without a message.
func (a *Agent) Change(ctx context.Context, label string, dryRun bool, fn func(*sheet.Workbook) error) ([]diff.Change, error) {
	made, err := a.Make(ctx, mcp.Making{Label: label, DryRun: dryRun, Fn: fn})
	return made.Changes, err
}

// Make proposes the change on a copy of the workbook, checks it against
// the scope, and then suggests it, or with direct edits allowed makes
// it as the agent's step; a formula asking JEV made directly asks the
// person first.
func (a *Agent) Make(ctx context.Context, mk mcp.Making) (mcp.Made, error) {
	var made mcp.Made
	var p *sheet.Proposal
	var err error
	needJEV := false
	a.seat.Do(func(w *sheet.Workbook) {
		b := BoardOf(a.seat)
		if !a.seat.Writing() {
			err = fmt.Errorf("%s writes here and the others follow: nobody else can change the workbook", a.seat.Writer())
			return
		}
		if p, err = sheet.Propose(w, mk.Label, mk.Fn); err != nil || p.Empty() {
			return
		}
		if err = b.Scope.Check(p); err != nil {
			return
		}
		if made.Changes, err = changesOf(w, p.Result()); err != nil || mk.DryRun {
			return
		}
		switch {
		case !b.Direct:
			made.Suggestion = b.Add(&Suggestion{Author: a.seat.ID(), Agent: a.seat.Name(), Color: a.seat.Color(), Message: mk.Message, Proposal: p})
			a.seat.Touch()
		case p.Remote && !b.Grants.JEV:
			needJEV = true
			return
		default:
			err = p.Apply(w, nil)
			made.Applied = err == nil
		}
		a.point(w, p)
	})
	if needJEV {
		err = a.grant(ctx, "jev", a.seat.Name()+" enters formulas that ask JEV, over the network, with your key")
		if err == nil {
			a.seat.Do(func(w *sheet.Workbook) {
				err = p.Apply(w, nil)
				made.Applied = err == nil
				a.point(w, p)
			})
		}
	}
	made.Notices = a.takeNotices()
	return made, err
}

// changesOf is what the copy changes of w, as 012 diff lists it.
func changesOf(w, copy *sheet.Workbook) ([]diff.Change, error) {
	var before, after bytes.Buffer
	if err := w.Write(&before); err != nil {
		return nil, err
	}
	if err := copy.Write(&after); err != nil {
		return nil, err
	}
	x, err := diff.Read(before.Bytes())
	if err != nil {
		return nil, err
	}
	y, err := diff.Read(after.Bytes())
	if err != nil {
		return nil, err
	}
	return diff.Compare(x, y), nil
}

// point moves the agent's pointer to the cells a proposal sets.
func (a *Agent) point(w *sheet.Workbook, p *sheet.Proposal) {
	if len(p.Cells) == 0 {
		return
	}
	first := p.Cells[0]
	s := w.Lookup(first.Sheet)
	if s == nil {
		return
	}
	r := sheet.Rect{From: first.At, To: first.At}
	for _, c := range p.Cells {
		if c.Sheet == first.Sheet {
			r.From.Row, r.From.Col = min(r.From.Row, c.At.Row), min(r.From.Col, c.At.Col)
			r.To.Row, r.To.Col = max(r.To.Row, c.At.Row), max(r.To.Col, c.At.Col)
		}
	}
	a.seat.Move(room.Presence{Sheet: s, Cursor: first.At, Selection: r})
}

// Focus moves the agent's pointer to ref.
func (a *Agent) Focus(_ context.Context, ref string) (string, error) {
	var at string
	var err error
	a.seat.Do(func(w *sheet.Workbook) {
		var t headless.Target
		if t, err = headless.Resolve(w, ref); err != nil {
			return
		}
		scope := BoardOf(a.seat).Scope
		if !scope.Allows(t.Sheet, t.Range) {
			err = fmt.Errorf("%s is outside what the person let you work on (%s)", ref, scope)
			return
		}
		a.seat.Move(room.Presence{Sheet: t.Sheet, Cursor: t.Range.From, Selection: t.Range})
		at = sheet.Qualified(t.Sheet.Name(), t.Range)
	})
	return at, err
}

// Undo takes back the agent's latest step.
func (a *Agent) Undo(context.Context) (string, error) {
	var label string
	var err error
	a.seat.Do(func(w *sheet.Workbook) {
		switch b, blocked := w.UndoBlocked(); {
		case !w.CanUndo():
			err = errors.New("you have no change of yours to undo: a suggestion is one once the person accepts it; the suggestions tool lists them")
		case blocked:
			who := "someone who left"
			if p, ok := a.seat.Peer(b.Author); ok {
				who = p.Name
			}
			err = fmt.Errorf("%s has changed what your latest change (%s) changed since: undo takes back only your own changes, never over theirs", who, w.UndoLabel())
		default:
			label = w.UndoLabel()
			w.Undo()
		}
	})
	return label, err
}

// Ask puts the question on the board and waits for the person.
func (a *Agent) Ask(ctx context.Context, p *sdk.ElicitParams) (*sdk.ElicitResult, error) {
	q, err := NewAsk(p.Message, p.RequestedSchema)
	if err != nil {
		return nil, err
	}
	ans, err := a.wait(ctx, q)
	if err != nil {
		return nil, err
	}
	return &sdk.ElicitResult{Action: ans.Action, Content: ans.Content}, nil
}

// grant asks leave for what, unless the person gave it for the session.
func (a *Agent) grant(ctx context.Context, what, message string) error {
	ans, err := a.wait(ctx, NewGrant(what, message))
	if err != nil {
		return err
	}
	if ans.Action != "accept" {
		return errors.New("the person didn't allow it")
	}
	return nil
}

// wait asks q and waits for the answer, withdrawing the question when
// ctx ends first.
func (a *Agent) wait(ctx context.Context, q *Ask) (Answer, error) {
	q.Author, q.Agent, q.Color = a.seat.ID(), a.seat.Name(), a.seat.Color()
	a.seat.Do(func(*sheet.Workbook) {
		BoardOf(a.seat).AddAsk(q)
		a.seat.Touch()
	})
	select {
	case ans := <-q.Reply():
		return ans, nil
	case <-ctx.Done():
		a.seat.Do(func(*sheet.Workbook) {
			BoardOf(a.seat).Withdraw(q)
			a.seat.Touch()
		})
		return Answer{}, ctx.Err()
	}
}

// Suggestions lists the agent's suggestions.
func (a *Agent) Suggestions(context.Context) ([]mcp.Suggestion, error) {
	var out []mcp.Suggestion
	a.seat.Do(func(*sheet.Workbook) {
		for _, s := range BoardOf(a.seat).Of(a.seat.ID()) {
			n := len(s.Cells)
			acc, rej := s.Count(CellAccepted), s.Count(CellRejected)
			if s.Whole() {
				acc, rej = acc*n, rej*n
			}
			out = append(out, mcp.Suggestion{ID: s.ID, Label: s.Label, Message: s.Message, State: s.Outcome(), Cells: n, Accepted: acc, Rejected: rej})
		}
	})
	return out, nil
}

// Scope is the board's scope in words.
func (a *Agent) Scope() string {
	var s string
	a.seat.Do(func(*sheet.Workbook) { s = BoardOf(a.seat).Scope.String() })
	return s
}

// RunCell is a request for the keeper's session to run a notebook cell,
// which it takes from the room (room.Seat.Post).
type RunCell struct {
	Notebook string
	Cell     int // from 0
	Agent    string
}

// RunCell asks leave to run a notebook cell, unless the person gave it
// for the session, and has the room's keeper run it.
func (a *Agent) RunCell(ctx context.Context, notebook string, cell int) (string, error) {
	var err error
	granted := false
	a.seat.Do(func(w *sheet.Workbook) {
		b := BoardOf(a.seat)
		granted = b.Grants.Notebooks
		if b.Scope.Kind != Workbook {
			err = fmt.Errorf("running a notebook cell changes its outputs and the sheets they're sent to, outside what the person let you change (%s)", b.Scope)
			return
		}
		s := w.Lookup(notebook)
		switch {
		case s == nil || !s.IsNotebook():
			err = fmt.Errorf("there's no notebook tab named %s: describe lists them", notebook)
		case cell < 1 || cell > len(s.NotebookCells()):
			err = fmt.Errorf("%s has %d cells", notebook, len(s.NotebookCells()))
		}
	})
	if err != nil {
		return "", err
	}
	if !granted {
		if err := a.grant(ctx, "notebooks", fmt.Sprintf("%s asks to run cell %d of %s with nu, as you", a.seat.Name(), cell, notebook)); err != nil {
			return "", err
		}
	}
	a.seat.Post(RunCell{Notebook: notebook, Cell: cell - 1, Agent: a.seat.Name()})
	return fmt.Sprintf("cell %d of %s runs on the person's screen: its output arrives in the workbook as when they run it; read it with describe or read_range", cell, notebook), nil
}

// report takes the news of the agent's settled suggestions, keeping it
// for its next write, and returns it.
func (a *Agent) report() []string {
	var news []string
	a.seat.Do(func(*sheet.Workbook) {
		for _, s := range BoardOf(a.seat).Resolved(a.seat.ID()) {
			news = append(news, s.Notice())
		}
	})
	if len(news) > 0 {
		a.mu.Lock()
		a.notices = append(a.notices, news...)
		a.mu.Unlock()
	}
	return news
}

// takeNotices is the news kept since the agent last wrote.
func (a *Agent) takeNotices() []string {
	a.report()
	a.mu.Lock()
	defer a.mu.Unlock()
	n := a.notices
	a.notices = nil
	return n
}

// Leave takes the agent's questions off the board and gives up its
// seat. Its suggestions stay for the person to settle.
func (a *Agent) Leave() {
	a.seat.Do(func(*sheet.Workbook) {
		b := BoardOf(a.seat)
		for _, q := range b.Asks() {
			if q.Author == a.seat.ID() {
				b.Withdraw(q)
			}
		}
		a.seat.Touch()
	})
	a.seat.Leave()
}
