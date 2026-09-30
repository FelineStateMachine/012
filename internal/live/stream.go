package live

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Stream follows a program that prints a table as it makes it: NUON
// values one after another, one to a line, as a notebook cell run as a
// stream prints them (nushell.Stream). The program starts with the
// stream and runs on a goroutine of its own until it ends or the stream
// is closed. A poll waits up to Wait for something to arrive, so rows
// reach the region as soon as the program prints them, and the owner
// polls again at once rather than after Interval.
//
// It keeps the last Keep values printed, for the cell's output (NUON)
// and for reading the table whole again (Reload).
type Stream struct {
	mu      sync.Mutex
	pending []byte // printed since the last poll
	ended   bool
	err     error // why the program ended, nil when it ended well
	arrived chan struct{}
	cancel  context.CancelFunc

	// The rest is the poller's: polls never overlap. values is guarded
	// by mu too, since the owner reads it (NUON).
	partial []byte // a line not yet ended
	tail    *fileio.Tail
	values  [][]byte
	printed int // values printed in all; guarded by mu
	header  sheet.LiveRow
	rows    []sheet.LiveRow
	total   int // rows printed in all; guarded by mu
	reload  bool
	done    bool // the end was reported; guarded by mu

	// Wait is how long a poll waits for the program to print.
	Wait time.Duration
	// Keep is how many of the last values are kept.
	Keep int
	// Now is the clock; tests replace it.
	Now func() time.Time
}

// StreamKeep is how many of the last values a stream keeps.
const StreamKeep = 10000

// NewStream starts run, which prints the table to w until ctx is done,
// and follows what it prints.
func NewStream(run func(ctx context.Context, w io.Writer) error) *Stream {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Stream{arrived: make(chan struct{}, 1), cancel: cancel, Wait: Interval, Keep: StreamKeep, Now: time.Now}
	s.tail, _ = fileio.NewTail(fileio.NUON, nil, nil)
	go func() {
		err := run(ctx, streamWriter{s})
		s.mu.Lock()
		s.ended, s.err = true, err
		s.mu.Unlock()
		s.kick()
	}()
	return s
}

// streamWriter takes what the program prints.
type streamWriter struct{ s *Stream }

func (w streamWriter) Write(p []byte) (int, error) {
	w.s.mu.Lock()
	w.s.pending = append(w.s.pending, p...)
	w.s.mu.Unlock()
	w.s.kick()
	return len(p), nil
}

// kick tells a poll waiting that something arrived.
func (s *Stream) kick() {
	select {
	case s.arrived <- struct{}{}:
	default:
	}
}

// Poll waits up to Wait for the program to print, and returns the rows
// it completed: all of those kept again after Reload, and the reason
// it failed once it ends with an error.
func (s *Stream) Poll(ctx context.Context) (Update, bool) {
	if done, _ := s.Ended(); done {
		return Update{}, false
	}
	t := time.NewTimer(s.Wait)
	select {
	case <-s.arrived:
	case <-t.C:
	case <-ctx.Done():
	}
	t.Stop()
	s.mu.Lock()
	got, ended, err := s.pending, s.ended, s.err
	s.pending = nil
	s.mu.Unlock()

	u := Update{At: s.Now()}
	s.read(got, ended, &u)
	if s.reload {
		s.reload = false
		u.Reset, u.Header, u.Rows = true, s.header, s.rows
	}
	if ended && !s.done {
		s.mu.Lock()
		s.done = true
		s.mu.Unlock()
		if err != nil && !isStop(err) {
			u.Err = err.Error()
		}
		return u, true
	}
	return u, u.Reset || len(u.Rows) > 0 || u.Header != nil
}

// read parses the complete lines of what arrived into u, keeping them.
func (s *Stream) read(got []byte, ended bool, u *Update) {
	s.partial = append(s.partial, got...)
	end := bytes.LastIndexByte(s.partial, '\n') + 1
	if ended {
		end = len(s.partial)
	}
	if end == 0 {
		return
	}
	text := s.partial[:end]
	s.partial = append([]byte(nil), s.partial[end:]...)
	var lines [][]byte
	for line := range bytes.SplitSeq(text, []byte("\n")) {
		if line = bytes.TrimSpace(line); len(line) > 0 {
			lines = append(lines, append([]byte(nil), line...))
		}
	}
	if len(lines) == 0 {
		return
	}
	res, err := s.tail.Feed(append(bytes.Join(lines, []byte("\n")), '\n'))
	if err != nil {
		u.Err = err.Error()
	}
	if res.Header != nil {
		s.header = res.Header
		u.Header = res.Header
	}
	s.mu.Lock() // the values and the counts change together (Snapshot)
	s.values = keepLast(append(s.values, lines...), s.Keep)
	s.printed += len(lines)
	s.total += len(res.Rows)
	s.mu.Unlock()
	s.rows = keepLast(append(s.rows, res.Rows...), s.Keep)
	u.Rows = res.Rows
}

// keepLast is the last n of xs.
func keepLast[T any](xs []T, n int) []T {
	if n > 0 && len(xs) > n {
		return append(xs[:0:0], xs[len(xs)-n:]...)
	}
	return xs
}

// isStop reports whether err is the stream being stopped rather than
// failing.
func isStop(err error) bool { return errors.Is(err, context.Canceled) }

// Reload has the next poll give every row kept again.
func (s *Stream) Reload() { s.reload = true }

// Close stops the program; the stream is polled no more.
func (s *Stream) Close() {
	s.cancel()
	s.tail.Close()
}

// Ended reports whether the program has ended and the last poll gave
// what it printed, and why it failed if it did (nil when it ended well
// or was stopped).
func (s *Stream) Ended() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.done {
		return false, nil
	}
	if isStop(s.err) {
		return true, nil
	}
	return true, s.err
}

// Rows is how many rows the program has printed so far.
func (s *Stream) Rows() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total
}

// NUON is the values kept, as a NUON list: the cell's output so far.
func (s *Stream) NUON() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return list(s.values)
}

// list is values as a NUON list.
func list(values [][]byte) []byte {
	return append(append([]byte("["), bytes.Join(values, []byte(", "))...), ']')
}

// Snapshot is what a stream has printed, read at one moment.
type Snapshot struct {
	NUON []byte // the values kept, as NUON gives them
	// More are the values printed after the first seen (Snapshot's
	// argument), as a NUON list, so what shows the output can add them
	// to what it shows rather than read it all again; nil when some of
	// them are kept no more.
	More []byte
	// Values and Rows are how many values and rows have been printed.
	Values, Rows int
}

// Snapshot is what the stream has printed so far, and what of it came
// after the first seen values.
func (s *Stream) Snapshot(seen int) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := Snapshot{NUON: list(s.values), Values: s.printed, Rows: s.total}
	if n := s.printed - seen; n >= 0 && n <= len(s.values) {
		snap.More = list(s.values[len(s.values)-n:])
	}
	return snap
}
