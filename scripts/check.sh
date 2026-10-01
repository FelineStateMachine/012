#!/bin/sh
# make check: lint, unit tests, the excelize oracle and the end-to-end
# tests run at once, each into a log of its own printed whole when it
# ends, so their output never interleaves; then the speed gate and the
# timed tests run alone, on a CPU no other tests share. Exits 1 if any
# phase failed. See docs/contributing/testing.md.
set -u
cd "$(dirname "$0")/.."
MAKE=${MAKE:-make}

out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
start=$(date +%s)
fail=""

phase() {
	"$MAKE" --no-print-directory "$1" >"$out/$1" 2>&1
	echo $? >"$out/$1.status"
	date +%s >"$out/$1.end"
}

report() {
	status=$(cat "$out/$1.status")
	took=$(($(cat "$out/$1.end") - start))
	if [ "$status" = 0 ]; then
		echo "=== $1: ok, done at ${took}s"
	else
		echo "=== $1: FAILED, done at ${took}s"
		fail="$fail $1"
	fi
	cat "$out/$1"
}

# e2e first: it's the longest.
for p in e2e test lint oracle; do
	phase "$p" &
	eval "pid_$p=$!"
done
for p in lint oracle test e2e; do
	eval "wait \$pid_$p"
	report "$p"
done

phase speed
report speed

echo "=== make check: $(($(date +%s) - start))s"
if [ -n "$fail" ]; then
	echo "failed:$fail"
	exit 1
fi
