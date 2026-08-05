#!/bin/sh
set -eu

root="$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)"
tmp_root="$(mktemp -d "${TMPDIR:-/tmp}/dockwarden-package-test.XXXXXX")"
trap 'rm -rf "$tmp_root"' EXIT HUP INT TERM

binary="$tmp_root/dockwarden"
output_dir="$tmp_root/output"
extract_dir="$tmp_root/extracted"
mkdir -p "$output_dir" "$extract_dir"
cat >"$binary" <<'SCRIPT'
#!/bin/sh
printf '%s\n' '0.3.0'
SCRIPT
chmod 0755 "$binary"

sh "$root/tools/release/package.sh" v0.3.0 linux amd64 "$binary" "$output_dir"
archive="$output_dir/dockwarden-v0.3.0-linux-amd64.tar.gz"
test -f "$archive"
tar -xzf "$archive" -C "$extract_dir"
test -x "$extract_dir/dockwarden"
test -f "$extract_dir/install.sh"
test -f "$extract_dir/README.md"
test -f "$extract_dir/CHANGELOG.md"
test -f "$extract_dir/LICENSE"

if sh "$root/tools/release/package.sh" v0.3.0 linux amd64 "$tmp_root/missing" "$output_dir"; then
	printf '%s\n' 'package accepted a missing binary' >&2
	exit 1
fi
test ! -f "$output_dir/dockwarden-v0.3.0-linux-amd64.tar.gz.tmp"

printf '%s\n' 'package tests passed'
