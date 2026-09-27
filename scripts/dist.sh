#!/usr/bin/env bash
# dist.sh - what `make dist VERSION=v1.2.3` does: cross-compile 012 as
# pure Go for each platform into dist/, stamped with the version, one
# archive per platform (tar.gz, zip for Windows) and a SHA256SUMS file.
# Nothing is uploaded. See docs/contributing/releasing.md.
#
#   VERSION=v1.2.3     required, a semantic version tag
#   PLATFORMS="darwin/arm64 linux/amd64"   narrow the list
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

version=${VERSION:-}
if ! [[ $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
	echo "dist: VERSION must be a version like v1.2.3 (got '${version}')" >&2
	exit 1
fi
if [ -n "$(git status --porcelain --untracked-files=no 2>/dev/null)" ]; then
	echo "dist: warning: the working tree has uncommitted changes; the binaries include them" >&2
fi
if tag=$(git rev-parse -q --verify "refs/tags/$version^{commit}" 2>/dev/null) && [ "$tag" != "$(git rev-parse HEAD)" ]; then
	echo "dist: $version is tagged at another commit; check out the tag first" >&2
	exit 1
fi
platforms=${PLATFORMS:-"darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64"}

out="$root/dist"
rm -rf "$out"
mkdir -p "$out"

for p in $platforms; do
	goos=${p%/*}
	goarch=${p#*/}
	name="012_${version#v}_${goos}_${goarch}"
	dir="$out/$name"
	exe=012
	if [ "$goos" = windows ]; then
		exe=012.exe
	fi
	mkdir -p "$dir"
	echo "dist: $name"
	CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath \
		-ldflags "-s -w -X main.version=$version" -o "$dir/$exe" ./cmd/012
	cp LICENSE NOTICE README.md "$dir/"
	if [ "$goos" = windows ]; then
		(cd "$out" && zip -qrX "$name.zip" "$name")
	else
		tar -C "$out" -czf "$out/$name.tar.gz" "$name"
	fi
	rm -rf "$dir"
done

(
	cd "$out"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum -- 012_* >SHA256SUMS
	else
		shasum -a 256 -- 012_* >SHA256SUMS
	fi
)
echo "dist: $(find "$out" -type f | wc -l | tr -d ' ') files in dist/"
cat "$out/SHA256SUMS"
