#!/usr/bin/env bash
# stress-data.sh - fetch real public datasets for stress-testing 012's importers.
#
# Downloads every file listed in scripts/stress-data.tsv into .deps/stress/
# (override with STRESS_DIR), verifies SHA-256, and is idempotent: files that
# are already present with the right checksum are left alone, files with a
# wrong checksum are re-downloaded, and a download that still does not match
# fails loudly. All URLs are pinned to a commit hash or a versioned release.
#
# The manifest (scripts/stress-data.tsv, tab-separated, one header row:
# name, url, sha256, bytes, license, shape, purpose) is the single source of
# truth; Go code can read it too. The table below is a summary of it.
#
# | file                           | exercises                                        | source (pinned)                                   | license                  | bytes   | rows x cols                  |
# |--------------------------------|--------------------------------------------------|---------------------------------------------------|--------------------------|---------|------------------------------|
# | owid-energy-data.csv           | wide CSV (130 cols) + rows past the 8192 limit   | github owid/energy-data @7e387a1                  | CC-BY-4.0                | 9229369 | 23377 x 130                  |
# | airport-codes.csv              | very long CSV (10x row limit), accented Latin    | github datasets/airport-codes @08b5eef            | ODC-PDDL-1.0             | 8809882 | 86134 x 13                   |
# | co2-ppm-daily.csv              | numeric CSV past the row limit, dates           | github datasets/co2-ppm-daily @ac476b2           | ODC-PDDL-1.0 (NOAA)      | 347788  | 18304 x 2                    |
# | country-codes.csv              | Arabic/CJK/Cyrillic text, leading-zero codes     | github datasets/country-codes @6a595f1            | ODC-PDDL-1.0             | 134003  | 249 x 56                     |
# | FormulaEvalTestData_Copy.xlsx  | XLSX formulas (1189 on sheet 1) + number formats | github apache/poi @942d95d test-data/spreadsheet  | Apache-2.0               | 65011   | sheet 1 A2:AL1504 (788 rows) |
# | Chinook_Sqlite.sqlite          | SQLite tables (PlaylistTrack past row limit)     | github lerocha/chinook-database release v1.4.5    | MIT                      | 1067008 | 11 tables, max 8715 x 2      |
# | alltypes_tiny_pages.parquet    | Parquet, every primitive type, tiny pages        | github apache/parquet-testing @56653c4            | Apache-2.0               | 454233  | 7300 x 13                    |
#
# Total: about 20 MB. Every file is openly licensed (CC-BY-4.0, public domain
# dedications, Apache-2.0 or MIT); see the license column of the manifest.
#
# Usage: scripts/stress-data.sh            (from anywhere)
#        STRESS_DIR=/tmp/stress scripts/stress-data.sh

set -eu

script_dir=$(cd "$(dirname "$0")" && pwd)
repo_root=$(cd "$script_dir/.." && pwd)
manifest="$script_dir/stress-data.tsv"
dest=${STRESS_DIR:-"$repo_root/.deps/stress"}

if [ ! -f "$manifest" ]; then
	echo "stress-data: manifest not found: $manifest" >&2
	exit 1
fi

if command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
elif command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | awk '{print $1}'; }
else
	echo "stress-data: need shasum or sha256sum" >&2
	exit 1
fi

filesize() { wc -c <"$1" | tr -d ' '; }

mkdir -p "$dest"

failed=0
first=1
while IFS=$'\t' read -r name url want bytes _license _shape _purpose; do
	if [ "$first" = 1 ]; then # header row
		first=0
		continue
	fi
	[ -n "$name" ] || continue
	path="$dest/$name"

	if [ -f "$path" ] && [ "$(sha256 "$path")" = "$want" ]; then
		printf '%-32s %10s  ok\n' "$name" "$(filesize "$path")"
		continue
	fi

	if [ -f "$path" ]; then
		echo "stress-data: $name: checksum mismatch on disk, re-downloading" >&2
	fi

	tmp="$path.part"
	rm -f "$tmp"
	if ! curl -fL --retry 3 --silent --show-error -o "$tmp" "$url"; then
		rm -f "$tmp"
		echo "stress-data: ERROR: $name: download failed: $url" >&2
		failed=1
		continue
	fi

	got=$(sha256 "$tmp")
	if [ "$got" != "$want" ]; then
		echo "stress-data: ERROR: $name: SHA-256 mismatch after download" >&2
		echo "  url:      $url" >&2
		echo "  expected: $want ($bytes bytes)" >&2
		echo "  got:      $got ($(filesize "$tmp") bytes)" >&2
		echo "  the upstream file changed or the download is corrupt; kept as $tmp" >&2
		failed=1
		continue
	fi

	mv -f "$tmp" "$path"
	printf '%-32s %10s  downloaded\n' "$name" "$(filesize "$path")"
done <"$manifest"

if [ "$failed" != 0 ]; then
	echo "stress-data: FAILED: one or more files could not be fetched and verified (see above)" >&2
	exit 1
fi
