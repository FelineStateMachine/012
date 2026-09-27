package transfer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/telemetry"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

func TestProgressDisplay(t *testing.T) {
	th := theme.New(true)
	var x Transfer
	prog := fileio.NewProgress()
	cancelled := false
	x.Begin("data/big.csv", prog, func() { cancelled = true }, Book)
	prog.Report(1234, 0, 0)
	if !x.Busy() || ansi.Strip(x.Line(&th)) != "Importing big.csv…    Esc  cancel" {
		t.Errorf("line %q", ansi.Strip(x.Line(&th)))
	}
	left, right := x.Status(&th, 80)
	if ansi.Strip(left) != "Importing big.csv" || right != "1,234 rows read" {
		t.Errorf("status %q %q", left, right)
	}
	if pb := x.ProgressBar(); pb == nil || pb.State != tea.ProgressBarIndeterminate {
		t.Errorf("progress %+v", pb)
	}
	prog.Report(4096, 1, 2)
	if _, right := x.Status(&th, 80); !strings.HasPrefix(ansi.Strip(right), "4,096 rows read  ━") || !strings.HasSuffix(right, " 50%") {
		t.Errorf("status %q", ansi.Strip(right))
	}
	if pb := x.ProgressBar(); pb == nil || pb.Value != 50 {
		t.Errorf("progress %+v", pb)
	}
	if name := x.Cancel(); name != "data/big.csv" || !cancelled || x.Busy() || x.ProgressBar() != nil {
		t.Errorf("cancel: %q cancelled %v", name, cancelled)
	}
}

func TestUpdateDropsStaleResults(t *testing.T) {
	var x Transfer
	old := x.Begin("old.csv", fileio.NewProgress(), func() {}, Book)
	id := x.Begin("new.csv", fileio.NewProgress(), func() {}, Sheet)
	if _, done, cmd := x.Update(TickMsg{old}); done || cmd != nil {
		t.Error("a stale tick ticked")
	}
	if _, _, cmd := x.Update(TickMsg{id}); cmd == nil {
		t.Error("the running import stopped ticking")
	}
	if _, done, _ := x.Update(ImportedMsg{ID: old}); done {
		t.Error("a stale result was taken")
	}
	res, done, _ := x.Update(ImportedMsg{ID: id, Name: "new.csv", Place: Sheet})
	if !done || res.Name != "new.csv" || res.Place != Sheet || x.Busy() {
		t.Errorf("result %+v done %v busy %v", res, done, x.Busy())
	}
}

func TestStartReadsInTheBackground(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.csv")
	if err := os.WriteFile(path, []byte("1,2\n3,4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var x Transfer
	cmd := x.Start("a.csv", path, fileio.Options{}, NewSheets, telemetry.Parent{})
	var res ImportedMsg
	for _, c := range cmd().(tea.BatchMsg) {
		if msg, ok := c().(ImportedMsg); ok {
			res = msg
		}
	}
	got, done, _ := x.Update(res)
	if !done || got.Err != nil || got.Res.Rows != 2 || got.Place != NewSheets {
		t.Errorf("imported %+v, err %v", got.Res, got.Err)
	}
}

func TestRows(t *testing.T) {
	for n, want := range map[int]string{1: "1 row", 2: "2 rows", 1234567: "1,234,567 rows"} {
		if got := Rows(n); got != want {
			t.Errorf("Rows(%d) = %q", n, got)
		}
	}
}
