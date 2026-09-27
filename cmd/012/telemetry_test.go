package main

import (
	"slices"
	"testing"
)

func TestTelemetryFlags(t *testing.T) {
	t.Setenv("O12_LOG", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://env:4318")
	c, rest, err := telemetryFlags([]string{"--log", "a.jsonl", "--otlp=http://localhost:4318", "b.csv"})
	if err != nil || c.LogPath != "a.jsonl" || c.OTLP.Endpoint != "http://localhost:4318" || !slices.Equal(rest, []string{"b.csv"}) {
		t.Errorf("got %+v %v %v", c, rest, err)
	}
	c, rest, err = telemetryFlags([]string{"b.csv"})
	if err != nil || c.OTLP.Endpoint != "http://env:4318" || len(rest) != 1 {
		t.Errorf("env endpoint: %+v %v %v", c, rest, err)
	}
	if _, _, err := telemetryFlags([]string{"--otlp"}); err == nil {
		t.Error("--otlp without a value")
	}
}
