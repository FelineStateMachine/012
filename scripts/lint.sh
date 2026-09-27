#!/bin/sh
# Code shape limits, checked by make lint:
#   - no function with cognitive complexity over 25 (gocognit)
#   - no Go file over 500 lines
# gocognit runs with go run at a pinned version, so it never enters go.mod.
set -eu
cd "$(dirname "$0")/.."

GOCOGNIT=github.com/uudashr/gocognit/cmd/gocognit@v1.2.1
MAX_COGNIT=25
MAX_LINES=500
fail=0

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

exit $fail
