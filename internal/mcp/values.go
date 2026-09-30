package mcp

import (
	"encoding/json"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/FelineStateMachine/012/internal/headless"
)

// The schema of a cell's value (headless.Value), which the tools that
// write take and the tools that read return, so a model sees in the
// tools' schemas how to write money, percentages, dates, times,
// durations and sizes, and what it reads back.

// valueDescription says what a value is.
const valueDescription = `A cell's value with its type: a number, a string (always text, even "00123"), true, false, null (blank), ` +
	`or an object naming its type: {"currency": 3.5} ($3.50; "symbol": "€" for another currency), {"percent": 0.12} (12%), ` +
	`{"date": "2026-09-29"} or {"date": "2026-09-29T14:30:00"}, {"time": "14:30"}, {"duration": "90min"} (nushell's units, or seconds), ` +
	`{"size": 1500} (bytes, or "1.5kb"), {"number": 1234.5}, {"text": "00123"}. ` +
	`Any object may add "decimals" (2) or "format", a number format code ("#,##0.00", "yyyy-mm-dd").`

// valueSchema is headless.Value's schema.
func valueSchema() *jsonschema.Schema {
	str := &jsonschema.Schema{Type: "string"}
	num := &jsonschema.Schema{Type: "number"}
	numOrStr := &jsonschema.Schema{Types: []string{"number", "string"}}
	typed := func(key string, v *jsonschema.Schema) *jsonschema.Schema {
		return &jsonschema.Schema{Type: "object", Required: []string{key}, Properties: map[string]*jsonschema.Schema{
			key: v, "decimals": {Type: "integer", Minimum: ptr(0.0), Maximum: ptr(15.0)}, "format": str, "symbol": str,
		}}
	}
	return &jsonschema.Schema{Description: valueDescription, AnyOf: []*jsonschema.Schema{
		{Types: []string{"number", "string", "boolean", "null"}},
		typed("currency", num), typed("percent", num), typed("date", str), typed("time", str),
		typed("duration", numOrStr), typed("size", numOrStr), typed("number", num), typed("text", str),
	}}
}

// schemaTypes infer json.RawMessage, a value already in JSON, as any
// value, and a cell's value as valueSchema says.
var schemaTypes = map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[json.RawMessage](): {},
	reflect.TypeFor[headless.Value]():  valueSchema(),
	reflect.TypeFor[headless.Row](): {Description: "A row: its columns' values by name, or a list of them in columns' order, each a cell's value with its type",
		AnyOf: []*jsonschema.Schema{{Type: "object", AdditionalProperties: valueSchema()}, {Type: "array", Items: valueSchema()}}},
}

// rawSchemas are the options inferring the tools' schemas.
var rawSchemas = &jsonschema.ForOptions{TypeSchemas: schemaTypes}
