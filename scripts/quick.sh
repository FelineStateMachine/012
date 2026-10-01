#!/bin/sh
# make quick: the checks of make check that concern what changed, for
# iterating. The packages are those with Go files changed since the
# branch left BASE (main), committed or not, or PKGS when set (./x/y or
# ./x/... directories). For them: make lint's Go checks (the docs checks
# cover the repo), and their unit tests; the oracle when it changed; and
# the e2e tests and golden screens defined in changed e2e files, or all
# of e2e with E2E=1. It is no stand-in for make check, which must pass
# before a push. See docs/contributing/testing.md.
set -u
cd "$(dirname "$0")/.."
MAKE=${MAKE:-make}
BASE=${BASE:-main}
# TIMED is the tests make test leaves to make speed, from the Makefile.
TIMED=${TIMED:-^$}

if [ -n "${PKGS:-}" ]; then
	dirs="" files=""
	for p in $PKGS; do
		p=${p#./}
		case $p in
		... | */...) dirs="$dirs $(git ls-files "${p%...}*.go" | xargs -n1 dirname | sort -u)" ;;
		*) dirs="$dirs $p" ;;
		esac
	done
else
	from=$(git merge-base "$BASE" HEAD) || exit 1
	files=$( (git diff --name-only "$from"; git ls-files --others --exclude-standard) | sort -u)
	dirs=$(for f in $files; do
		case $f in (*.go) [ -f "$f" ] && dirname "$f" ;; esac
	done | sort -u)
fi
dirs=$(echo $dirs)

out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
fail=0
names=""
run() {
	name=$1
	shift
	("$@") >"$out/$name" 2>&1 &
	eval "pid_$name=$!"
	names="$names $name"
}

lint() {
	if [ -n "$dirs" ]; then
		scripts/lint.sh $dirs
	else
		go run ./scripts/doclint && go run ./scripts/doccheck
	fi
}

unit() {
	set --
	for d in $dirs; do
		case $d in oracle | oracle/* | e2e | e2e/*) ;; *) set -- "$@" "./$d" ;; esac
	done
	[ $# -eq 0 ] || STRESS_DIR=$(pwd)/.deps/stress go test -skip "$TIMED" "$@"
}

oracle() {
	case " $dirs " in
	*" oracle"*) cd oracle && go test -count=1 ./... ;;
	esac
}

# e2e runs the tests named in changed e2e files, and the golden screens
# they define with their light and high-contrast variants; all of e2e
# with E2E=1 or when the harness changed.
e2e() {
	pattern="" screens=""
	for f in $files; do
		case $f in
		e2e/harness_test.go | e2e/screens_test.go) E2E=1 ;;
		e2e/*_test.go)
			[ -f "$f" ] || continue
			pattern="$pattern$(sed -n 's/^func \(Test[A-Za-z0-9_]*\)(t \*testing\.T).*/|\1/p' "$f" | tr -d '\n')"
			screens="$screens$(sed -n 's/.*{name: "\([^"]*\)".*/|\1/p' "$f" | tr -d '\n')"
			;;
		esac
	done
	if [ -z "${E2E:-}" ] && [ -z "$pattern$screens" ]; then
		return 0
	fi
	"$MAKE" --no-print-directory -s libghostty || return 1
	PKG_CONFIG_PATH=$(pwd)/.deps/ghostty/share/pkgconfig
	export PKG_CONFIG_PATH
	cd e2e || return 1
	if [ -n "${E2E:-}" ]; then
		go test -count=1 ./...
		return
	fi
	ok=0
	if [ -n "$pattern" ]; then
		go test -count=1 -run "^(${pattern#|})\$" ./... || ok=1
	fi
	if [ -n "$screens" ]; then
		go test -count=1 -run "^TestScreens\$/^(${screens#|})(-light|-high-contrast)?\$" ./... || ok=1
	fi
	return $ok
}

echo "quick: ${dirs:-no Go packages} changed"
run lint lint
run unit unit
run oracle oracle
run e2e e2e
for name in $names; do
	eval "pid=\$pid_$name"
	wait "$pid" || { fail=1; echo "=== $name: FAILED"; }
	cat "$out/$name"
done
if [ $fail = 0 ]; then
	echo "quick: ok; make check before a push"
fi
exit $fail
