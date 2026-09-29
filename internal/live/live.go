// Package live reads the sources of linked regions as they change: a
// file followed as it grows or is rewritten (File), and anything else
// that yields a table's rows over time behind the same Source.
//
// A Source is polled on its owner's schedule, one poll at a time, from
// a goroutine of the owner's (a Bubble Tea command), and answers with an
// Update: rows to add, or all of them again, which the owner turns into
// a sheet.LiveOp for the region (Update.Op) and applies on the goroutine
// that owns the workbook. Sources never touch the workbook.
package live

import (
	"context"
	"time"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// A Source yields a table's rows as they change.
type Source interface {
	// Poll looks once for what changed since the last poll. It reports
	// false when nothing did. Polls never overlap.
	Poll(ctx context.Context) (Update, bool)
	// Reload has the next poll read the table again, whole.
	Reload()
	// Close lets the source go; it is polled no more.
	Close()
}

// Update is what a poll found.
type Update struct {
	// Reset has Rows replace every row; otherwise they follow the last.
	Reset bool
	// Header is the table's first row when it's new or changed.
	Header sheet.LiveRow
	Rows   []sheet.LiveRow
	// Err says why the source can't be read, "" once it can.
	Err string
	// Note says what was left out or changed in reading.
	Note string
	// More is set when more is waiting to be read at once, rather than
	// after the next interval.
	More bool
	// At is when the poll found it.
	At time.Time
}

// Op is u as the operation that applies it to the region id.
func (u Update) Op(id int) sheet.LiveOp {
	return sheet.LiveOp{Link: id, At: u.At, Reset: u.Reset, Header: u.Header, Rows: u.Rows, Err: u.Err, Note: u.Note}
}

// Interval is how often a followed source is polled.
const Interval = 250 * time.Millisecond
