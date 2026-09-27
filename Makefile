# 012 builds as pure Go. Only the e2e tests need cgo and libghostty-vt,
# which is built from source with Zig into .deps/.

GHOSTTY_COMMIT := 27e8b3fa85d9cf8c7cd5ae2ced348bcb0a4fba9c
DEPS           := $(CURDIR)/.deps
GHOSTTY_SRC    := $(DEPS)/ghostty-src
GHOSTTY_OUT    := $(DEPS)/ghostty
GHOSTTY_STAMP  := $(GHOSTTY_OUT)/.built-$(GHOSTTY_COMMIT)

.PHONY: build run test fuzz e2e screens oracle libghostty clean stress stress-data stress-report obs-up obs-down obs-status stress-load stress-e2e

build:
	CGO_ENABLED=0 go build -o bin/012 ./cmd/012

run: build
	./bin/012

test:
	go test ./...

fuzz:
	go test ./internal/sheet -run '^$$' -fuzz FuzzParse -fuzztime 60s
	go test ./internal/fileio -run '^$$' -fuzz FuzzReadDelimited -fuzztime 60s
	go test ./internal/fileio -run '^$$' -fuzz FuzzReadWK1 -fuzztime 60s

e2e: $(GHOSTTY_STAMP)
	cd e2e && PKG_CONFIG_PATH=$(GHOSTTY_OUT)/share/pkgconfig go test -count=1 ./...

# Rewrite golden screens and build e2e/testdata/screens/gallery.html for
# visual review. Review the diff and the gallery before committing.
screens: $(GHOSTTY_STAMP)
	cd e2e && PKG_CONFIG_PATH=$(GHOSTTY_OUT)/share/pkgconfig go test -count=1 -run TestScreens ./... -update

# Stress: fetch real datasets into .deps/stress, run the benchmarks built
# with -tags stress, print a summary and record the run in
# .deps/stress/results/runs.jsonl. BENCH, BENCHTIME and PKGS narrow it;
# see docs/limits.md. Plain `go test ./...` never runs these.
stress:
	scripts/stress/run.sh

stress-data:
	scripts/stress-data.sh

# Compare the latest run with the previous one and a baseline (DuckDB).
stress-report:
	scripts/stress/report.sh

# The observability stack (deploy/observability): an OpenTelemetry
# Collector, ClickHouse and Grafana in Docker. See docs/observability.md.
obs-up:
	scripts/obs.sh up

obs-down:
	scripts/obs.sh down

obs-status:
	scripts/obs.sh status

# End-to-end key latency on a big sheet through libghostty, with the
# telemetry log on (O12_E2E_LOG keeps it).
stress-e2e: $(GHOSTTY_STAMP)
	cd e2e && STRESS=1 PKG_CONFIG_PATH=$(GHOSTTY_OUT)/share/pkgconfig go test -count=1 -run TestStressKeyLatency -v ./...

# Load the recorded stress runs into the stack's ClickHouse.
stress-load:
	scripts/stress/clickhouse-load.sh

# Differential tests: formulas and number formats against excelize's
# calculation engine. A separate module, so the binary never depends on it.
oracle:
	cd oracle && go test -count=1 ./...

libghostty: $(GHOSTTY_STAMP)

# The commit matches the one pinned by go.mitchellh.com/libghostty.
$(GHOSTTY_STAMP):
	rm -rf $(GHOSTTY_SRC)
	git init -q $(GHOSTTY_SRC)
	git -C $(GHOSTTY_SRC) fetch -q --depth 1 https://github.com/ghostty-org/ghostty.git $(GHOSTTY_COMMIT)
	git -C $(GHOSTTY_SRC) checkout -q FETCH_HEAD
	cd $(GHOSTTY_SRC) && zig build -Demit-lib-vt -Demit-xcframework=false -Doptimize=ReleaseFast --prefix $(GHOSTTY_OUT)
	touch $@

clean:
	rm -rf bin $(DEPS)
