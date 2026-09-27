package telemetry

import (
	"context"
	"log/slog"
	"slices"
	"time"
)

// spanKey carries a span's ids from log to the OTLP log handler, so the
// log record of an operation links to its span.
type spanKey struct{}

type spanIDs struct {
	trace traceID
	span  spanID
}

// otlpHandler is a slog.Handler that queues OTLP log records.
type otlpHandler struct {
	e      *exporter
	level  slog.Leveler
	attrs  []keyValue
	prefix string
}

func (h *otlpHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *otlpHandler) Handle(ctx context.Context, r slog.Record) error {
	rec := logRecord{
		TimeUnixNano:         nanos(r.Time),
		ObservedTimeUnixNano: nanos(time.Now()),
		SeverityNumber:       severity(r.Level),
		SeverityText:         r.Level.String(),
		Body:                 str(r.Message),
		Attributes:           slices.Clip(h.attrs),
	}
	r.Attrs(func(a slog.Attr) bool {
		rec.Attributes = appendAttr(rec.Attributes, h.prefix, a)
		return true
	})
	if ids, ok := ctx.Value(spanKey{}).(spanIDs); ok {
		rec.TraceID, rec.SpanID = ids.trace, ids.span
	}
	h.e.log(rec)
	return nil
}

func (h *otlpHandler) WithAttrs(as []slog.Attr) slog.Handler {
	h2 := *h
	h2.attrs = slices.Clip(h.attrs)
	for _, a := range as {
		h2.attrs = appendAttr(h2.attrs, h.prefix, a)
	}
	return &h2
}

func (h *otlpHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := *h
	h2.prefix += name + "."
	return &h2
}
