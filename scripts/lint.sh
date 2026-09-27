#!/bin/sh
# Checked by make lint:
#   - staticcheck, with staticcheck.conf, in every module
#   - no function with cognitive complexity over 25 (gocognit)
#   - no Go file over 500 lines
#   - no docs or comments narrating history (scripts/doclint)
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

exit $fail
