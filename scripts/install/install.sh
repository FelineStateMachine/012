#!/bin/sh
# install.sh - installs a released 012 binary on macOS or Linux. Served
# at the docs site's root with the release archives under /releases/;
# see docs/getting-started/install.md.
#
#   curl -fsSL https://f58b.n.zip/install.sh | sh
#   curl -fsSL https://f58b.n.zip/install.sh | sh -s -- --system
#
# It picks the archive for this OS and architecture, checks its SHA256
# against the release's SHA256SUMS, and puts 012 in ~/.local/bin, in
# $PREFIX/bin when PREFIX is set, or in /usr/local/bin with --system.
# sudo is only run for --system when that directory isn't writable, and
# the command is printed first.
#
#   --version v1.2.3   a release other than the latest (or O12_VERSION)
#   --system           install into /usr/local/bin
#   O12_BASE_URL       the site serving /releases/ (https://f58b.n.zip)
set -eu

base=${O12_BASE_URL:-https://f58b.n.zip}
version=${O12_VERSION:-latest}
system=0

say() { printf '012 install: %s\n' "$*"; }
die() {
	printf '012 install: %s\n' "$*" >&2
	exit 1
}

while [ $# -gt 0 ]; do
	case $1 in
	--version)
		[ $# -ge 2 ] || die "--version needs a version, like v1.2.3"
		version=$2
		shift 2
		;;
	--version=*)
		version=${1#--version=}
		shift
		;;
	--system)
		system=1
		shift
		;;
	-h | --help)
		sed -n '2,17p' "$0" 2>/dev/null | sed 's/^# \{0,1\}//' || true
		exit 0
		;;
	*) die "unknown option $1 (--version v1.2.3, --system)" ;;
	esac
done

case $version in
latest | v[0-9]*) ;;
[0-9]*) version=v$version ;;
*) die "--version must be a release like v1.2.3 (got $version)" ;;
esac

os=$(uname -s)
case $os in
Darwin) os=darwin ;;
Linux) os=linux ;;
MINGW* | MSYS* | CYGWIN* | Windows*)
	die "on Windows, run in PowerShell: irm $base/install.ps1 | iex"
	;;
*) die "no release for $os; build from source: go install github.com/FelineStateMachine/012/cmd/012@latest" ;;
esac

arch=$(uname -m)
case $arch in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) die "no release for $arch; build from source: go install github.com/FelineStateMachine/012/cmd/012@latest" ;;
esac
# A shell under Rosetta reports x86_64 on an Apple silicon Mac.
if [ "$os/$arch" = darwin/amd64 ] && [ "$(sysctl -n hw.optional.arm64 2>/dev/null || echo 0)" = 1 ]; then
	arch=arm64
fi

if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -q -O "$2" "$1"; }
else
	die "needs curl or wget"
fi

if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | cut -d ' ' -f 1; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$1" | cut -d ' ' -f 1; }
elif command -v openssl >/dev/null 2>&1; then
	sha256() { openssl dgst -sha256 -r "$1" | cut -d ' ' -f 1; }
else
	die "needs sha256sum, shasum or openssl to check the download"
fi

if [ "$system" = 1 ]; then
	bindir=/usr/local/bin
elif [ -n "${PREFIX:-}" ]; then
	bindir=$PREFIX/bin
else
	[ -n "${HOME:-}" ] || die "HOME is not set; set PREFIX to install into \$PREFIX/bin"
	bindir=$HOME/.local/bin
fi

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t 012install)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM

url=$base/releases/$version
fetch "$url/SHA256SUMS" "$tmp/SHA256SUMS" || die "couldn't download $url/SHA256SUMS"

# A line of SHA256SUMS: the digest, two spaces, 012_1.2.3_linux_amd64.tar.gz.
line=$(grep "  012_[^ ]*_${os}_${arch}\.tar\.gz\$" "$tmp/SHA256SUMS" | head -n 1 || true)
[ -n "$line" ] || die "release $version has no archive for $os/$arch"
want=${line%% *}
file=${line##* }
dir=${file%.tar.gz}
got_version=${dir#012_}
got_version=v${got_version%_"${os}"_"${arch}"}

say "downloading $url/$file"
fetch "$url/$file" "$tmp/$file" || die "couldn't download $url/$file"
got=$(sha256 "$tmp/$file")
[ "$got" = "$want" ] || die "$file has SHA256 $got, SHA256SUMS says $want; nothing installed"
say "SHA256 checked against SHA256SUMS: $want"

tar -xzf "$tmp/$file" -C "$tmp" || die "couldn't unpack $file"
[ -f "$tmp/$dir/012" ] || die "$file holds no 012 binary"

if [ ! -d "$bindir" ] && ! mkdir -p "$bindir" 2>/dev/null; then
	[ "$system" = 1 ] || die "couldn't create $bindir"
fi
if [ -d "$bindir" ] && [ -w "$bindir" ]; then
	cp "$tmp/$dir/012" "$bindir/012.new"
	chmod 755 "$bindir/012.new"
	mv -f "$bindir/012.new" "$bindir/012"
elif [ "$system" = 1 ]; then
	command -v sudo >/dev/null 2>&1 || die "$bindir isn't writable and there is no sudo; run as root or leave out --system"
	say "$bindir isn't writable by $(id -un); running: sudo install -m 755 012 $bindir/012"
	sudo mkdir -p "$bindir"
	sudo install -m 755 "$tmp/$dir/012" "$bindir/012"
else
	die "$bindir isn't writable; set PREFIX to another directory"
fi

say "installed 012 $got_version ($os/$arch) as $bindir/012"
case ":${PATH:-}:" in
*":$bindir:"*) say "run 012 to start" ;;
*) say "$bindir is not on your PATH; add it (export PATH=\"$bindir:\$PATH\") or run $bindir/012" ;;
esac
