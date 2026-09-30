#!/usr/bin/env bash
# site-release.sh - what `make site-release` does after `make site`:
# puts a release's archives and SHA256SUMS in the built site under
# /releases/<version>/ and /releases/latest/, and the install scripts at
# its root. See docs/contributing/releasing.md.
#
#   VERSION=v1.2.3   the release to serve; the newest version tag by default
#   SITE_URL         the site's address, which the install scripts download from
#
# The archives come from dist/ when it holds that version; otherwise
# `make dist` builds them in a temporary checkout of the tag, so the
# site can be published from a later commit, and they're kept in dist/.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

site="$root/website/build"
if [ ! -f "$site/index.html" ]; then
	echo "site-release: no site in website/build; run make site first" >&2
	exit 1
fi

version=${VERSION:-$(git tag --list 'v[0-9]*' --sort=-v:refname | head -n 1)}
if [ -z "$version" ]; then
	echo "site-release: no version tag to serve; set VERSION" >&2
	exit 1
fi
if ! git rev-parse -q --verify "refs/tags/$version" >/dev/null; then
	echo "site-release: $version isn't a tag" >&2
	exit 1
fi

# dist/ holds this version when every archive in SHA256SUMS is named for it.
holds() {
	[ -f "$1/SHA256SUMS" ] && [ -s "$1/SHA256SUMS" ] &&
		! grep -qv "  012_${version#v}_" "$1/SHA256SUMS"
}

dist="$root/dist"
if ! holds "$dist"; then
	tmp=$(mktemp -d)
	trap 'git -C "$root" worktree remove --force "$tmp/src" 2>/dev/null || true; rm -rf "$tmp"' EXIT
	echo "site-release: building $version in a checkout of the tag"
	git worktree add -q --detach "$tmp/src" "$version"
	make -C "$tmp/src" dist VERSION="$version"
	holds "$tmp/src/dist" || {
		echo "site-release: make dist left no archives for $version" >&2
		exit 1
	}
	rm -rf "$dist"
	cp -R "$tmp/src/dist" "$dist"
fi
(
	cd "$dist"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum -c --quiet SHA256SUMS
	else
		shasum -a 256 -c --quiet SHA256SUMS
	fi
)

# The scripts name https://012.dev.site; a site built for another SITE_URL
# serves scripts that download from itself.
url=${SITE_URL:-https://012.dev.site}
url=${url%/}

rm -rf "$site/releases"
for dir in "$version" latest; do
	mkdir -p "$site/releases/$dir"
	(cd "$dist" && cp SHA256SUMS 012_* "$site/releases/$dir/")
	{
		printf '<!doctype html>\n<meta charset="utf-8">\n<title>012 %s</title>\n' "$version"
		printf '<h1>012 %s</h1>\n<p>Checksums: <a href="SHA256SUMS">SHA256SUMS</a>. ' "$version"
		printf 'Install with <code>curl -fsSL %s/install.sh | sh</code>.</p>\n<ul>\n' "$url"
		(cd "$dist" && for f in 012_*; do printf '<li><a href="%s">%s</a></li>\n' "$f" "$f"; done)
		printf '</ul>\n'
	} >"$site/releases/$dir/index.html"
done
for f in install.sh install.ps1; do
	sed "s|https://012\.dev\.site|$url|g" "scripts/install/$f" >"$site/$f"
done
echo "site-release: $version in website/build/releases/{$version,latest}, install.sh and install.ps1 at the root"
