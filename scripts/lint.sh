#!/bin/sh
# Checked by make lint:
#   - staticcheck, with staticcheck.conf, in every module
#   - no function with cognitive complexity over 25 (gocognit)
#   - no Go file over 500 lines
#   - no docs or comments narrating history (scripts/doclint)
#   - links, anchors, the docs index, media and tapes in step (scripts/doccheck)
# The tools run with go run at pinned versions, so they never enter go.mod.
set -eu
cd "$(dirname "$0")/.."

GOCOGNIT=github.com/uudashr/gocognit/cmd/gocognit@v1.2.1
STATICCHECK=honnef.co/go/tools/cmd/staticcheck@2026.2.1
MAX_COGNIT=25
MAX_LINES=500
fail=0

# Warnings are fixed, or turned off in staticcheck.conf with a reason;
# none are left standing.
for mod in . oracle e2e; do
	if [ "$mod" = e2e ] && [ -z "${PKG_CONFIG_PATH:-}" ]; then
		PKG_CONFIG_PATH="$(pwd)/.deps/ghostty/share/pkgconfig"
		export PKG_CONFIG_PATH
	fi
	if ! (cd "$mod" && go run $STATICCHECK ./...); then
		echo "staticcheck: findings in $mod"
		fail=1
	fi
done

# Tracked files stay small and textual: no compiled binaries, nothing over
# 2 MB (docs/media GIFs have their own 1 MB limit in doccheck).
big=$(git ls-files | while IFS= read -r f; do
	if [ -f "$f" ] && [ "$(wc -c <"$f")" -gt 2097152 ]; then echo "$f"; fi
done)
if [ -n "$big" ]; then
	echo "tracked files over 2 MB:"
	echo "$big"
	fail=1
fi
bins=$(git ls-files | grep -E '\.(test|exe|o|a|so|dylib)$|^bin/|(^|/)(zz_|tmp_?|scratch|dbg_)[^/]*$' || true)
if [ -n "$bins" ]; then
	echo "compiled or scratch files tracked by git:"
	echo "$bins"
	fail=1
fi

dirs=$(find cmd internal demos e2e oracle -name '*.go' -exec dirname {} \; | sort -u)
over=$(go run $GOCOGNIT -over $MAX_COGNIT $dirs || true)
if [ -n "$over" ]; then
	echo "cognitive complexity over $MAX_COGNIT:"
	echo "$over" | sort -rn
	fail=1
fi

long=$(find cmd internal demos e2e oracle -name '*.go' -exec wc -l {} + | awk -v max=$MAX_LINES '$2 != "total" && $1 > max')
if [ -n "$long" ]; then
	echo "files over $MAX_LINES lines:"
	echo "$long" | sort -rn
	fail=1
fi

# Docs and comments describe the code as it is, not how it got here.
if ! go run ./scripts/doclint; then
	fail=1
fi

# Links and anchors resolve, the index lists every doc, and every tape
# records something a doc shows.
if ! go run ./scripts/doccheck; then
	fail=1
fi

exit $fail
