# one23 builds as pure Go. Only the e2e tests need cgo and libghostty-vt,
# which is built from source with Zig into .deps/.

GHOSTTY_COMMIT := 27e8b3fa85d9cf8c7cd5ae2ced348bcb0a4fba9c
DEPS           := $(CURDIR)/.deps
GHOSTTY_SRC    := $(DEPS)/ghostty-src
GHOSTTY_OUT    := $(DEPS)/ghostty
GHOSTTY_STAMP  := $(GHOSTTY_OUT)/.built-$(GHOSTTY_COMMIT)

.PHONY: build run test fuzz e2e libghostty clean

build:
	CGO_ENABLED=0 go build -o bin/one23 ./cmd/one23

run: build
	./bin/one23

test:
	go test ./...

fuzz:
	go test ./internal/sheet -run '^$$' -fuzz FuzzParse -fuzztime 60s

e2e: $(GHOSTTY_STAMP)
	cd e2e && PKG_CONFIG_PATH=$(GHOSTTY_OUT)/share/pkgconfig go test -count=1 ./...

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
