#!/bin/sh
set -eu

if [ "$#" -lt 5 ] || [ "$#" -gt 6 ]; then
	printf '%s\n' 'usage: package.sh VERSION GOOS GOARCH BINARY OUTPUT_DIR [FWUPD_PREFIX]' >&2
	exit 2
fi

version="$1"
target_os="$2"
target_arch="$3"
binary="$4"
output_dir="$5"
fwupd_prefix="${6:-}"
root="$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)"
stage_root=""

die() {
	printf '%s\n' "dockwarden package: $*" >&2
	exit 2
}

cleanup() {
	if [ -n "$stage_root" ] && [ -d "$stage_root" ]; then
		rm -rf "$stage_root"
	fi
}

trap cleanup EXIT HUP INT TERM

case "$version" in
v[0-9]*.[0-9]*.[0-9]*) ;;
*) die "version must have the form vMAJOR.MINOR.PATCH: $version" ;;
esac

case "$target_os/$target_arch" in
darwin/arm64|darwin/amd64|linux/amd64|linux/arm64) ;;
*) die "unsupported target: $target_os/$target_arch" ;;
esac

[ -x "$binary" ] || die "binary is not executable: $binary"
for required_file in README.md CHANGELOG.md LICENSE tools/release/install.sh; do
	[ -f "$root/$required_file" ] || die "missing package file: $required_file"
done

if [ "$target_os" = darwin ]; then
	[ "$(uname -s)" = Darwin ] || die "Darwin archives must be built on macOS"
	[ -n "$fwupd_prefix" ] || die "Darwin archives require a fwupd prefix"
	[ -x "$fwupd_prefix/bin/fwupdtool" ] || die "fwupdtool is missing from $fwupd_prefix"
	[ -f "$fwupd_prefix/manifest.json" ] || die "fwupd manifest is missing from $fwupd_prefix"
	command -v file >/dev/null 2>&1 || die "missing required command: file"
	command -v install_name_tool >/dev/null 2>&1 || die "missing required command: install_name_tool"
	command -v otool >/dev/null 2>&1 || die "missing required command: otool"
	command -v python3 >/dev/null 2>&1 || die "missing required command: python3"
fi

mkdir -p "$output_dir"
stage_root="$(mktemp -d "${TMPDIR:-/tmp}/dockwarden-package.XXXXXX")"
cp "$binary" "$stage_root/dockwarden"
chmod 0755 "$stage_root/dockwarden"
cp "$root/README.md" "$stage_root/README.md"
cp "$root/CHANGELOG.md" "$stage_root/CHANGELOG.md"
cp "$root/LICENSE" "$stage_root/LICENSE"
cp "$root/tools/release/install.sh" "$stage_root/install.sh"
chmod 0755 "$stage_root/install.sh"

relocate_darwin_prefix() {
	source_prefix="$1"
	stage_prefix="$2"
	find "$stage_prefix/bin" "$stage_prefix/lib" -type f | while IFS= read -r mach_o_file; do
		if ! file "$mach_o_file" | grep -q 'Mach-O'; then
			continue
		fi
		otool -L "$mach_o_file" | sed -n '2,$s/^[[:space:]]*//p' | sed 's/ (.*$//' |
		while IFS= read -r dependency; do
			case "$dependency" in
			"$source_prefix"/*)
				relative_source_path="${dependency#"$source_prefix"/}"
				stage_dependency="$stage_prefix/$relative_source_path"
				[ -e "$stage_dependency" ] || die "missing internal fwupd dependency: $stage_dependency"
				loader_relative_path="$(python3 - "$mach_o_file" "$stage_dependency" <<'PY'
import os
import sys

print(os.path.relpath(sys.argv[2], os.path.dirname(sys.argv[1])))
PY
)"
				install_name_tool -change "$dependency" "@loader_path/$loader_relative_path" "$mach_o_file"
				;;
			esac
		done
		case "$mach_o_file" in
		*.dylib)
			install_name="$(otool -D "$mach_o_file" 2>/dev/null | sed -n '2p' || true)"
			case "$install_name" in
			"$source_prefix"/*)
				install_name_tool -id "@loader_path/$(basename -- "$mach_o_file")" "$mach_o_file"
				;;
			esac
			esac
	done

	find "$stage_prefix/bin" "$stage_prefix/lib" -type f | while IFS= read -r mach_o_file; do
		if file "$mach_o_file" | grep -q 'Mach-O' && otool -L "$mach_o_file" | grep -F "$source_prefix" >/dev/null 2>&1; then
			die "build-prefix path remains in $mach_o_file"
		fi
	done
}

refresh_fwupd_manifest() {
	manifest="$1"
	prefix="$2"
	python3 - "$manifest" "$prefix" <<'PY'
import hashlib
import json
import pathlib
import sys

manifest_path = pathlib.Path(sys.argv[1])
prefix = pathlib.Path(sys.argv[2])
manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
hashes = {}
for relative_root in ("bin", "etc/fwupd", "lib", "share/fwupd"):
    root = prefix / relative_root
    if not root.is_dir():
        raise SystemExit(f"missing fwupd runtime directory: {relative_root}")
    for path in sorted(root.rglob("*")):
        if path.is_file():
            relative_path = path.relative_to(prefix).as_posix()
            hashes[relative_path] = hashlib.sha256(path.read_bytes()).hexdigest()
if "bin/fwupdtool" not in hashes:
    raise SystemExit("missing fwupdtool in runtime manifest")
manifest["binary_sha256"] = hashes["bin/fwupdtool"]
manifest["runtime_sha256"] = hashes
manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
}

if [ "$target_os" = darwin ]; then
	cp -R "$fwupd_prefix" "$stage_root/fwupd-2.2.1"
	relocate_darwin_prefix "$fwupd_prefix" "$stage_root/fwupd-2.2.1"
	refresh_fwupd_manifest "$stage_root/fwupd-2.2.1/manifest.json" "$stage_root/fwupd-2.2.1"
fi

archive_name="dockwarden-$version-$target_os-$target_arch.tar.gz"
archive_path="$output_dir/$archive_name"
archive_tmp="$archive_path.tmp"
rm -f "$archive_tmp"
tar -czf "$archive_tmp" -C "$stage_root" .
mv -f "$archive_tmp" "$archive_path"
printf '%s\n' "$archive_path"
