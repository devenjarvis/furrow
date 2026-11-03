#!/bin/sh
# bin/furrow runs Furrow {{VERSION}}, pinned for this repo. `furrow init --pin` wrote it.
#
# On first run it downloads that release into .furrow/bin/ (ignored by git), checks the archive's
# sha256 against the lines below, and runs it. It refuses an archive that doesn't match.
# To move the pin, delete this file and run `furrow init --pin` with the newer furrow.
# `furrow doctor` says when a newer release is out.
set -eu

FURROW_VERSION={{VERSION}}
FURROW_CHECKSUMS='
{{CHECKSUMS}}'

say() { echo "bin/furrow: $*" >&2; }

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
dir="$root/.furrow/bin/$FURROW_VERSION"
bin="$dir/furrow"

install_furrow() {
	case $(uname -s) in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*) say "Furrow has no release for $(uname -s). It runs on macOS and Linux."; return 1 ;;
	esac
	case $(uname -m) in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) say "Furrow has no release for $(uname -m). It runs on amd64 and arm64."; return 1 ;;
	esac
	archive="furrow_${FURROW_VERSION#v}_${os}_${arch}.tar.gz"
	want=$(printf '%s\n' "$FURROW_CHECKSUMS" | awk -v f="$archive" '$2 == f { print $1 }')
	if [ -z "$want" ]; then
		say "this pin has no checksum for $archive. Run \`furrow init --pin\` again with the release's checksums.txt."
		return 1
	fi
	url="${FURROW_DOWNLOAD_URL:-https://github.com/devenjarvis/furrow/releases/download}/$FURROW_VERSION/$archive"

	tmp=$(mktemp -d "${TMPDIR:-/tmp}/furrow.XXXXXX")
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$tmp/$archive" "$url" || { say "couldn't download $url."; return 1; }
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$tmp/$archive" "$url" || { say "couldn't download $url."; return 1; }
	else
		say "needs curl or wget to download Furrow $FURROW_VERSION. Install one and run it again."
		return 1
	fi

	if command -v sha256sum >/dev/null 2>&1; then
		got=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
	elif command -v shasum >/dev/null 2>&1; then
		got=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
	else
		say "needs sha256sum or shasum to check Furrow $FURROW_VERSION. Install one and run it again."
		return 1
	fi
	if [ "$got" != "$want" ]; then
		say "refused $archive: its sha256 is $got, but this pin expects $want. Nothing was installed."
		return 1
	fi

	tar -xzf "$tmp/$archive" -C "$tmp" furrow || { say "couldn't unpack $archive."; return 1; }
	mkdir -p "$dir"
	chmod 755 "$tmp/furrow"
	mv "$tmp/furrow" "$bin"
}

if [ ! -x "$bin" ]; then
	tmp=''
	status=0
	install_furrow || status=$?
	[ -z "$tmp" ] || rm -rf "$tmp"
	[ "$status" -eq 0 ] || exit "$status"
fi
exec "$bin" "$@"
