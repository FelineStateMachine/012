#!/bin/sh
# Checked by make lint, all at once, each check's findings printed whole
# once it ends:
#   - gofmt, and go vet with -tags stress (every file plain vet sees, and
#     the stress benchmarks; nothing builds only without the tag)
#   - staticcheck, with staticcheck.conf, in every module
#   - no function with cognitive complexity over 25 (gocognit)
#   - no Go file over 500 lines, no tracked file over 2 MB or compiled
#   - no docs or comments narrating history (scripts/doclint)
#   - links, anchors, the docs index, media and tapes in step (scripts/doccheck)
# The tools run with go run at pinned versions, so they never enter go.mod;
# go run and staticcheck cache their work, so a second run is quick.
#
# With directories as arguments (make quick), the Go checks look only at
# those packages; the tracked-file and docs checks always cover the repo.
set -eu
cd "$(dirname "$0")/.."

GOCOGNIT=github.com/uudashr/gocognit/cmd/gocognit@v1.2.1
STATICCHECK=honnef.co/go/tools/cmd/staticcheck@2026.2.1
MAX_COGNIT=25
MAX_LINES=500
: "${PKG_CONFIG_PATH:=$(pwd)/.deps/ghostty/share/pkgconfig}"
export PKG_CONFIG_PATH

whole=true
if [ $# -gt 0 ]; then
	dirs=$* whole=false
else
	dirs=$(find cmd internal demos e2e oracle -name '*.go' -exec dirname {} \; | sort -u)
fi

# pkgs MOD prints the packages to check in module MOD as paths relative
# to it: ./... for the whole module, else those of the directories given.
pkgs() {
	if $whole; then
		echo ./...
		return
	fi
	for d in $dirs; do
		case $1 in
		.) case $d in oracle | oracle/* | e2e | e2e/*) continue ;; esac; echo "./$d" ;;
		*) case $d in "$1") echo . ;; "$1"/*) echo "./${d#"$1"/}" ;; esac ;;
		esac
	done
}

gofmt_check() {
	bad=$(gofmt -l $dirs)
	if [ -n "$bad" ]; then
		echo "$bad"
		echo "gofmt: files above need formatting"
		return 1
	fi
}

vet() {
	set -- $(pkgs .)
	[ $# -eq 0 ] || go vet -tags stress "$@"
}

# Warnings are fixed, or turned off in staticcheck.conf with a reason;
# none are left standing.
staticcheck() {
	mod=$1
	set -- $(pkgs "$mod")
	[ $# -gt 0 ] || return 0
	if ! (cd "$mod" && go run $STATICCHECK "$@"); then
		echo "staticcheck: findings in $mod"
		return 1
	fi
}

# Tracked files stay small and textual: no compiled binaries, nothing over
# 2 MB (docs/media GIFs have their own 1 MB limit in doccheck).
tracked() {
	big=$(git ls-files -z | xargs -0 wc -c 2>/dev/null | awk '$1 > 2097152 && $2 != "total" { sub(/^ *[0-9]+ /, ""); print }')
	bins=$(git ls-files | grep -E '\.(test|exe|o|a|so|dylib)$|^bin/|(^|/)(zz_|tmp_?|scratch|dbg_)[^/]*$' || true)
	if [ -n "$big" ]; then
		echo "tracked files over 2 MB:"
		echo "$big"
	fi
	if [ -n "$bins" ]; then
		echo "compiled or scratch files tracked by git:"
		echo "$bins"
	fi
	[ -z "$big$bins" ]
}

shape() {
	over=$(go run $GOCOGNIT -over $MAX_COGNIT $dirs || true)
	long=$(find $dirs -maxdepth 1 -name '*.go' -exec wc -l {} + | awk -v max=$MAX_LINES '$2 != "total" && $1 > max')
	if [ -n "$over" ]; then
		echo "cognitive complexity over $MAX_COGNIT:"
		echo "$over" | sort -rn
	fi
	if [ -n "$long" ]; then
		echo "files over $MAX_LINES lines:"
		echo "$long" | sort -rn
	fi
	[ -z "$over$long" ]
}

# Docs and comments describe the code as it is, not how it got here
# (doclint); links and anchors resolve, the index lists every doc, and
# every tape records something a doc shows (doccheck).
doclint() { go run ./scripts/doclint; }
doccheck() { go run ./scripts/doccheck; }

out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
names=""
run() {
	name=$1
	shift
	("$@") >"$out/$name" 2>&1 &
	eval "pid_$name=$!"
	names="$names $name"
}

run gofmt gofmt_check
run vet vet
run staticcheck staticcheck .
run staticcheck_oracle staticcheck oracle
run staticcheck_e2e staticcheck e2e
run tracked tracked
run shape shape
run doclint doclint
run doccheck doccheck

fail=0
for name in $names; do
	eval "pid=\$pid_$name"
	wait "$pid" || fail=1
	cat "$out/$name"
done
exit $fail
