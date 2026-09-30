package nushell

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Streams: a cell run as a stream prints each value its pipeline yields
// as soon as the pipeline yields it, one NUON value to a line, rather
// than the whole value once the pipeline ends, so a pipeline that never
// ends (tail -f app.log | lines, watch) is followed as it goes. The
// lines are what nuon.Reader reads as a stream of rows.

// StreamScript is what nu runs for job as a stream: the tables bound to
// their names, as Script binds them, then the pipeline, each value it
// yields printed as NUON on a line of its own.
func StreamScript(job Job, names []string) string {
	var b strings.Builder
	for i, name := range names {
		fmt.Fprintf(&b, "let %s = (open --raw $env.NU012_TABLE_%d | from nuon)\n", name, i)
	}
	b.WriteString("do {\n" + job.Command + "\n} | each {|row| $row | to nuon --raw | print } | ignore\n")
	return b.String()
}

// Stream runs job as a stream, writing to w a line of NUON for each
// value the pipeline yields, until the pipeline ends or ctx is done. It
// has no timeout: the stream runs until it's stopped.
func Stream(ctx context.Context, r Runner, job Job, w io.Writer) error {
	err := r.Run(ctx, job, StreamScript(job, sortedNames(job.Tables)), w)
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	return err
}
