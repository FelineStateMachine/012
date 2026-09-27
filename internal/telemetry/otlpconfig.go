package telemetry

import (
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// OTLPConfig says where OTLP goes. ConfigFromEnv reads it from the
// standard OTEL_* variables. Nothing is sent unless an endpoint is set.
type OTLPConfig struct {
	// Endpoint is the base URL, like http://localhost:4318, to which
	// /v1/logs, /v1/traces and /v1/metrics are appended
	// (OTEL_EXPORTER_OTLP_ENDPOINT, or 012 --otlp).
	Endpoint string
	// LogsEndpoint, TracesEndpoint and MetricsEndpoint are full URLs for
	// one signal each, used as they are
	// (OTEL_EXPORTER_OTLP_{LOGS,TRACES,METRICS}_ENDPOINT).
	LogsEndpoint, TracesEndpoint, MetricsEndpoint string
	// NoLogs, NoTraces and NoMetrics leave a signal out
	// (OTEL_{LOGS,TRACES,METRICS}_EXPORTER=none).
	NoLogs, NoTraces, NoMetrics bool
	// Headers go with every request, e.g. for authentication
	// (OTEL_EXPORTER_OTLP_HEADERS). They are never logged.
	Headers map[string]string
	// Resource adds resource attributes (OTEL_RESOURCE_ATTRIBUTES);
	// service.name stays 012.
	Resource map[string]string
	// Timeout bounds each request (OTEL_EXPORTER_OTLP_TIMEOUT, in
	// milliseconds); 3 s when zero.
	Timeout time.Duration
	// NoCompression sends plain JSON instead of gzip
	// (OTEL_EXPORTER_OTLP_COMPRESSION=none).
	NoCompression bool
	// Disabled turns OTLP off whatever else is set (OTEL_SDK_DISABLED).
	Disabled bool
}

const defaultTimeout = 3 * time.Second

// otlpFromEnv reads the OTEL_* variables.
func otlpFromEnv() OTLPConfig {
	env := func(signal string) string {
		return os.Getenv("OTEL_EXPORTER_OTLP_" + signal + "ENDPOINT")
	}
	none := func(signal string) bool {
		return strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_"+signal+"_EXPORTER")), "none")
	}
	c := OTLPConfig{
		Endpoint:        env(""),
		LogsEndpoint:    env("LOGS_"),
		TracesEndpoint:  env("TRACES_"),
		MetricsEndpoint: env("METRICS_"),
		NoLogs:          none("LOGS"),
		NoTraces:        none("TRACES"),
		NoMetrics:       none("METRICS"),
		Headers:         keyValues(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")),
		Resource:        keyValues(os.Getenv("OTEL_RESOURCE_ATTRIBUTES")),
		NoCompression:   strings.EqualFold(os.Getenv("OTEL_EXPORTER_OTLP_COMPRESSION"), "none"),
		Disabled:        strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")), "true"),
	}
	if ms, err := strconv.Atoi(os.Getenv("OTEL_EXPORTER_OTLP_TIMEOUT")); err == nil && ms > 0 {
		c.Timeout = time.Duration(ms) * time.Millisecond
	}
	return c
}

// keyValues parses the "k1=v1,k2=v2" lists of OTEL_EXPORTER_OTLP_HEADERS
// and OTEL_RESOURCE_ATTRIBUTES, whose values are percent-encoded.
func keyValues(s string) map[string]string {
	var m map[string]string
	for kv := range strings.SplitSeq(s, ",") {
		k, v, ok := strings.Cut(kv, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			continue
		}
		if d, err := url.PathUnescape(strings.TrimSpace(v)); err == nil {
			v = d
		}
		if m == nil {
			m = map[string]string{}
		}
		m[k] = strings.TrimSpace(v)
	}
	return m
}

// urls are where each signal goes; empty leaves it out.
type urls struct{ logs, traces, metrics string }

func (u urls) any() bool { return u.logs != "" || u.traces != "" || u.metrics != "" }

func (c OTLPConfig) urls() urls {
	if c.Disabled {
		return urls{}
	}
	base := c.Endpoint
	if base != "" && !strings.Contains(base, "://") {
		base = "http://" + base
	}
	pick := func(own, path string, off bool) string {
		switch {
		case off:
			return ""
		case own != "":
			return own
		case base != "":
			return strings.TrimRight(base, "/") + path
		}
		return ""
	}
	return urls{
		logs:    pick(c.LogsEndpoint, "/v1/logs", c.NoLogs),
		traces:  pick(c.TracesEndpoint, "/v1/traces", c.NoTraces),
		metrics: pick(c.MetricsEndpoint, "/v1/metrics", c.NoMetrics),
	}
}
