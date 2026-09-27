package telemetry

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

// The OTLP/HTTP JSON encoding of logs, traces and metrics: the protobuf
// messages of opentelemetry-proto under the protobuf JSON mapping, so
// field names are lowerCamelCase, 64-bit integers are decimal strings,
// enums are their numbers and trace and span ids are hex. Only the
// fields 012 sends are declared.

// i64 is a 64-bit integer, written as a JSON string.
type i64 int64

func (n i64) MarshalJSON() ([]byte, error) {
	return strconv.AppendQuote(nil, strconv.FormatInt(int64(n), 10)), nil
}

// u64 is an unsigned 64-bit integer, written as a JSON string.
type u64 uint64

func (n u64) MarshalJSON() ([]byte, error) {
	return strconv.AppendQuote(nil, strconv.FormatUint(uint64(n), 10)), nil
}

func nanos(t time.Time) u64 { return u64(t.UnixNano()) }

type traceID [16]byte
type spanID [8]byte

func (id traceID) MarshalJSON() ([]byte, error) { return json.Marshal(hex.EncodeToString(id[:])) }
func (id spanID) MarshalJSON() ([]byte, error)  { return json.Marshal(hex.EncodeToString(id[:])) }

func (id traceID) IsZero() bool { return id == traceID{} }
func (id spanID) IsZero() bool  { return id == spanID{} }

// anyValue is an AnyValue holding one scalar.
type anyValue struct {
	kind byte // 's', 'i', 'd' or 'b'
	s    string
	i    int64
	d    float64
	b    bool
}

func str(s string) anyValue { return anyValue{kind: 's', s: s} }

func (v anyValue) MarshalJSON() ([]byte, error) {
	switch v.kind {
	case 'i':
		return fmt.Appendf(nil, `{"intValue":"%d"}`, v.i), nil
	case 'd':
		b, err := json.Marshal(v.d)
		return append(append([]byte(`{"doubleValue":`), b...), '}'), err
	case 'b':
		return fmt.Appendf(nil, `{"boolValue":%t}`, v.b), nil
	}
	b, err := json.Marshal(v.s)
	return append(append([]byte(`{"stringValue":`), b...), '}'), err
}

type keyValue struct {
	Key   string   `json:"key"`
	Value anyValue `json:"value"`
}

// appendAttr adds a to kvs, flattening groups into dotted keys.
func appendAttr(kvs []keyValue, prefix string, a slog.Attr) []keyValue {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return kvs
	}
	if a.Value.Kind() == slog.KindGroup {
		p := prefix
		if a.Key != "" {
			p += a.Key + "."
		}
		for _, g := range a.Value.Group() {
			kvs = appendAttr(kvs, p, g)
		}
		return kvs
	}
	return append(kvs, keyValue{Key: prefix + a.Key, Value: valueOf(a.Value)})
}

func valueOf(v slog.Value) anyValue {
	switch v.Kind() {
	case slog.KindString:
		return str(v.String())
	case slog.KindInt64:
		return anyValue{kind: 'i', i: v.Int64()}
	case slog.KindUint64:
		return anyValue{kind: 'i', i: int64(v.Uint64())}
	case slog.KindFloat64:
		return anyValue{kind: 'd', d: v.Float64()}
	case slog.KindBool:
		return anyValue{kind: 'b', b: v.Bool()}
	case slog.KindDuration:
		return anyValue{kind: 'd', d: ms(v.Duration())}
	case slog.KindTime:
		return str(v.Time().Format(time.RFC3339Nano))
	}
	return str(v.String())
}

func attrsOf(as []slog.Attr) []keyValue {
	var kvs []keyValue
	for _, a := range as {
		kvs = appendAttr(kvs, "", a)
	}
	return kvs
}

type resource struct {
	Attributes []keyValue `json:"attributes"`
}

type scope struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Logs.

type logsRequest struct {
	ResourceLogs []resourceLogs `json:"resourceLogs"`
}

type resourceLogs struct {
	Resource  resource    `json:"resource"`
	ScopeLogs []scopeLogs `json:"scopeLogs"`
}

type scopeLogs struct {
	Scope      scope       `json:"scope"`
	LogRecords []logRecord `json:"logRecords"`
}

type logRecord struct {
	TimeUnixNano         u64        `json:"timeUnixNano"`
	ObservedTimeUnixNano u64        `json:"observedTimeUnixNano"`
	SeverityNumber       int        `json:"severityNumber"`
	SeverityText         string     `json:"severityText"`
	Body                 anyValue   `json:"body"`
	Attributes           []keyValue `json:"attributes,omitempty"`
	TraceID              traceID    `json:"traceId,omitzero"`
	SpanID               spanID     `json:"spanId,omitzero"`
}

// severity maps a slog level onto an OTLP SeverityNumber: slog's levels
// are spaced so that Debug, Info, Warn and Error land on DEBUG (5), INFO
// (9), WARN (13) and ERROR (17).
func severity(l slog.Level) int { return min(max(int(l)+9, 1), 24) }

// Traces.

type tracesRequest struct {
	ResourceSpans []resourceSpans `json:"resourceSpans"`
}

type resourceSpans struct {
	Resource   resource     `json:"resource"`
	ScopeSpans []scopeSpans `json:"scopeSpans"`
}

type scopeSpans struct {
	Scope scope      `json:"scope"`
	Spans []spanData `json:"spans"`
}

const (
	spanKindInternal = 1
	statusOK         = 1
	statusError      = 2
)

type spanData struct {
	TraceID           traceID    `json:"traceId"`
	SpanID            spanID     `json:"spanId"`
	ParentSpanID      spanID     `json:"parentSpanId,omitzero"`
	Name              string     `json:"name"`
	Kind              int        `json:"kind"`
	StartTimeUnixNano u64        `json:"startTimeUnixNano"`
	EndTimeUnixNano   u64        `json:"endTimeUnixNano"`
	Attributes        []keyValue `json:"attributes,omitempty"`
	Status            status     `json:"status"`
}

type status struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// Metrics.

type metricsRequest struct {
	ResourceMetrics []resourceMetrics `json:"resourceMetrics"`
}

type resourceMetrics struct {
	Resource     resource       `json:"resource"`
	ScopeMetrics []scopeMetrics `json:"scopeMetrics"`
}

type scopeMetrics struct {
	Scope   scope    `json:"scope"`
	Metrics []metric `json:"metrics"`
}

const temporalityCumulative = 2

type metric struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Unit        string     `json:"unit,omitempty"`
	Gauge       *gauge     `json:"gauge,omitempty"`
	Sum         *sum       `json:"sum,omitempty"`
	Histogram   *histogram `json:"histogram,omitempty"`
	Summary     *summary   `json:"summary,omitempty"`
}

type gauge struct {
	DataPoints []numberPoint `json:"dataPoints"`
}

type sum struct {
	DataPoints             []numberPoint `json:"dataPoints"`
	AggregationTemporality int           `json:"aggregationTemporality"`
	IsMonotonic            bool          `json:"isMonotonic"`
}

type numberPoint struct {
	Attributes        []keyValue `json:"attributes,omitempty"`
	StartTimeUnixNano u64        `json:"startTimeUnixNano,omitempty"`
	TimeUnixNano      u64        `json:"timeUnixNano"`
	AsInt             *i64       `json:"asInt,omitempty"`
	AsDouble          *float64   `json:"asDouble,omitempty"`
}

func intPoint(start, t time.Time, v int64, attrs ...keyValue) numberPoint {
	n := i64(v)
	p := numberPoint{Attributes: attrs, TimeUnixNano: nanos(t), AsInt: &n}
	if !start.IsZero() {
		p.StartTimeUnixNano = nanos(start)
	}
	return p
}

type histogram struct {
	DataPoints             []histogramPoint `json:"dataPoints"`
	AggregationTemporality int              `json:"aggregationTemporality"`
}

type histogramPoint struct {
	Attributes        []keyValue `json:"attributes,omitempty"`
	StartTimeUnixNano u64        `json:"startTimeUnixNano"`
	TimeUnixNano      u64        `json:"timeUnixNano"`
	Count             u64        `json:"count"`
	Sum               float64    `json:"sum"`
	BucketCounts      []u64      `json:"bucketCounts"`
	ExplicitBounds    []float64  `json:"explicitBounds"`
	Min               float64    `json:"min"`
	Max               float64    `json:"max"`
}

type summary struct {
	DataPoints []summaryPoint `json:"dataPoints"`
}

type summaryPoint struct {
	StartTimeUnixNano u64             `json:"startTimeUnixNano"`
	TimeUnixNano      u64             `json:"timeUnixNano"`
	Count             u64             `json:"count"`
	Sum               float64         `json:"sum"`
	QuantileValues    []quantileValue `json:"quantileValues"`
}

type quantileValue struct {
	Quantile float64 `json:"quantile"`
	Value    float64 `json:"value"`
}
