# 012 builds as pure Go. Only the e2e tests need cgo and libghostty-vt,
# which is built from source with Zig into .deps/.

GHOSTTY_COMMIT := 27e8b3fa85d9cf8c7cd5ae2ced348bcb0a4fba9c
DEPS           := $(CURDIR)/.deps
GHOSTTY_SRC    := $(DEPS)/ghostty-src
GHOSTTY_OUT    := $(DEPS)/ghostty
GHOSTTY_STAMP  := $(GHOSTTY_OUT)/.built-$(GHOSTTY_COMMIT)

.PHONY: check lint build run test fuzz e2e screens oracle demos libghostty clean stress stress-data stress-report obs-up obs-down obs-status stress-load stress-e2e dist site site-serve site-deps

build:
	CGO_ENABLED=0 go build -o bin/012 ./cmd/012

run: build
	./bin/012

# Release archives: 012 cross-compiled (pure Go, -trimpath, stamped with
# VERSION) for macOS, Linux and Windows on amd64 and arm64 into dist/,
# with SHA256SUMS. Nothing is uploaded; see docs/contributing/releasing.md.
dist:
	VERSION=$(VERSION) scripts/dist.sh

test:
	STRESS_DIR=$(DEPS)/stress go test ./...

# Everything that must pass before a push: formatting, vet (also with the
# stress benchmarks), shape limits, unit tests, the excelize oracle and the
# end-to-end tests in libghostty.
check:
	@test -z "$$(gofmt -l cmd internal demos e2e oracle)" || { gofmt -l cmd internal demos e2e oracle; echo "gofmt: files above need formatting"; exit 1; }
	go vet -tags stress ./...
	$(MAKE) lint
	STRESS_DIR=$(DEPS)/stress go test ./...
	$(MAKE) oracle
	$(MAKE) e2e

# Code shape limits: cognitive complexity and file length.
lint:
	go vet ./...
	scripts/lint.sh

fuzz:
	go test ./internal/sheet -run '^$$' -fuzz FuzzParse -fuzztime 60s
	go test ./internal/sheet -run '^$$' -fuzz FuzzRead -fuzztime 60s
	go test ./internal/fileio -run '^$$' -fuzz FuzzReadDelimited -fuzztime 60s
	go test ./internal/fileio -run '^$$' -fuzz FuzzReadWK1 -fuzztime 60s
	go test ./internal/fileio -run '^$$' -fuzz 'FuzzReadXLSX$$' -fuzztime 60s
	go test ./internal/fileio -run '^$$' -fuzz FuzzReadXLSXParts -fuzztime 60s
	go test ./internal/nuon -run '^$$' -fuzz FuzzParse -fuzztime 60s
	go test ./internal/fileio -run '^$$' -fuzz FuzzReadNUON -fuzztime 60s

e2e: $(GHOSTTY_STAMP)
	cd e2e && PKG_CONFIG_PATH=$(GHOSTTY_OUT)/share/pkgconfig go test -count=1 ./...

# Rewrite golden screens and build e2e/testdata/screens/gallery.html for
# visual review. Review the diff and the gallery before committing.
screens: $(GHOSTTY_STAMP)
	cd e2e && PKG_CONFIG_PATH=$(GHOSTTY_OUT)/share/pkgconfig go test -count=1 -run TestScreens ./... -update

# Stress: fetch real datasets into .deps/stress, run the benchmarks built
# with -tags stress, print a summary and record the run in
# .deps/stress/results/runs.jsonl. BENCH, BENCHTIME and PKGS narrow it;
# see docs/contributing/limits.md. Plain `go test ./...` never runs these.
stress:
	scripts/stress/run.sh

stress-data:
	scripts/stress-data.sh

# Compare the latest run with the previous one, a baseline and the last
# tagged release's run (DuckDB); exits 1 on a regression against the
# release past THRESHOLD (0.10) plus the benchmark's noise.
stress-report:
	scripts/stress/report.sh

# The observability stack (deploy/observability): an OpenTelemetry
# Collector, ClickHouse and Grafana in Docker. See docs/contributing/observability.md.
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

# Demo GIFs from the VHS tapes in demos/ into demos/out/ (gitignored),
# PNG stills of key moments in demos/out/stills/, and smaller GIFs for
# the README in demos/out/media/ (12 fps, a 64-color palette, the last
# frame held). Needs vhs 0.12+ (go install
# github.com/charmbracelet/vhs@latest), ttyd and ffmpeg. The JEV demo
# talks to demos/fakejev, started here, never the real service.
# DEMOS=jev renders just one.
DEMOS ?= $(basename $(notdir $(wildcard demos/*.tape)))
FAKEJEV_ADDR := 127.0.0.1:8799

demos: build
	@for t in vhs ttyd ffmpeg; do command -v $$t >/dev/null || { echo "demos need $$t (vhs: go install github.com/charmbracelet/vhs@latest; ttyd, ffmpeg: brew install ttyd ffmpeg)"; exit 1; }; done
	CGO_ENABLED=0 go build -o bin/fakejev ./demos/fakejev
	mkdir -p demos/out/stills
	bin/fakejev -addr $(FAKEJEV_ADDR) & pid=$$!; trap "kill $$pid" EXIT; \
	export DEMOS_TTYD="$$(command -v ttyd)" PATH="$(CURDIR)/demos/lib:$$PATH"; \
	export XDG_CONFIG_HOME="$$(mktemp -d)" O12_JEV_CREDENTIAL_STORE=false O12_THEME= O12_LOCALE=en-US; \
	cd demos && for d in $(DEMOS); do echo "vhs $$d.tape"; vhs -q $$d.tape || exit 1; done
	mkdir -p demos/out/media
	for d in $(DEMOS); do ffmpeg -v error -y -i demos/out/$$d.gif -filter_complex \
		"fps=12,tpad=stop_mode=clone:stop_duration=2,split[a][b];[a]palettegen=max_colors=64:stats_mode=diff[p];[b][p]paletteuse=dither=none:diff_mode=rectangle" \
		demos/out/media/$$d.gif || exit 1; done
	@ls -l demos/out/media

# The docs site (website/): Docusaurus over docs/, built into
# website/build. Needs Node 20.11+ and npm; nothing else in the build or
# make check does. The build fails on a broken link or anchor. SITE_URL
# sets the address canonical links and the sitemap use. Node's warning
# that localStorage has no backing file is about the build's own process,
# which never stores anything. See docs/contributing/site.md.
SITE_NODE := NODE_OPTIONS=--disable-warning=ExperimentalWarning

site: site-deps
	cd website && $(SITE_NODE) npm run build

site-serve: site-deps
	cd website && $(SITE_NODE) npm run start

site-deps:
	@command -v npm >/dev/null || { echo "the site needs Node 20.11+ and npm"; exit 1; }
	cd website && npm ci

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
