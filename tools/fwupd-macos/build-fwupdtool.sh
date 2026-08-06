#!/bin/sh
set -eu

fwupd_version="2.2.1"
fwupd_commit="028b9c5800d2351a98ceaed4410ab9224c2142ed"
fwupd_ref="fexxdev/darwin-hid-dell-dock"
fwupd_repo="https://github.com/fexxdev/fwupd.git"
jinja2_version="3.1.6"
markupsafe_version="3.0.3"
cache_root="${DOCKWARDEN_FWUPD_CACHE_DIR:-${TMPDIR:-/tmp}/dockwarden-fwupd}"
prefix="${1:-$HOME/Library/Application Support/dockwarden/fwupd-$fwupd_version}"
work_dir=""
stage_root=""
backup_prefix=""
managed_marker_name=".dockwarden-managed-fwupd"
managed_marker_value="dockwarden managed fwupd prefix v1"

cleanup() {
	if [ -n "$backup_prefix" ] && [ -e "$backup_prefix" ] && [ ! -e "$prefix" ]; then
		mv "$backup_prefix" "$prefix"
		backup_prefix=""
	fi
	if [ -n "$work_dir" ] && [ -d "$work_dir" ]; then
		rm -rf "$work_dir"
	fi
	if [ -n "$stage_root" ] && [ -d "$stage_root" ]; then
		rm -rf "$stage_root"
	fi
}

trap cleanup EXIT
trap 'exit 1' HUP INT TERM

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "missing required command: $1" >&2
		exit 2
	fi
}

case "$prefix" in
	/*) ;;
	*)
		echo "fwupdtool prefix must be an absolute path: $prefix" >&2
		exit 2
		;;
esac
prefix_parent="$(dirname -- "$prefix")"
if [ "$(basename -- "$prefix_parent")" != "dockwarden" ] ||
	[ "$(basename -- "$prefix")" != "fwupd-$fwupd_version" ]; then
	echo "fwupdtool prefix must end in dockwarden/fwupd-$fwupd_version: $prefix" >&2
	exit 2
fi

require_command python3

if [ -e "$prefix" ] || [ -L "$prefix" ]; then
	if [ ! -d "$prefix" ] || [ -L "$prefix" ]; then
		echo "existing fwupdtool prefix is not a managed directory: $prefix" >&2
		exit 2
	fi
	if [ ! -f "$prefix/$managed_marker_name" ] || [ -L "$prefix/$managed_marker_name" ] ||
		[ "$(sed -n '1p' "$prefix/$managed_marker_name" 2>/dev/null || true)" != "$managed_marker_value" ]; then
		legacy_manifest="$prefix/manifest.json"
		if [ ! -f "$legacy_manifest" ] || [ -L "$legacy_manifest" ] ||
		! python3 - "$legacy_manifest" "$fwupd_version" "$fwupd_commit" <<'PY'
import json
import pathlib
import sys

manifest = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
expected = {
    "fwupd_version": sys.argv[2],
}
if any(manifest.get(key) != value for key, value in expected.items()):
    raise SystemExit(1)
if manifest.get("source_commit") not in {
    sys.argv[3],
    "09452b3ca1d2381568b90736382e995d69f7b584",
    "61c7cf1873fedd78fa031e8a8829cb3413aaef46",
}:
    raise SystemExit(1)
if not isinstance(manifest.get("runtime_sha256"), dict):
    raise SystemExit(1)
PY
		then
			echo "refusing to replace an unowned fwupdtool prefix: $prefix" >&2
			exit 2
		fi
	fi
fi

require_command brew
require_command git
require_command meson
require_command ninja
require_command pkg-config
require_command rustc
require_command cargo

for formula in gcab glib gnutls json-glib libjcat libusb libxmlb; do
	if ! brew list --versions "$formula" >/dev/null 2>&1; then
		echo "missing Homebrew formula: $formula" >&2
		exit 2
	fi
done

mkdir -p "$cache_root" "$prefix_parent"
work_dir="$(mktemp -d "$cache_root/work-$fwupd_commit.XXXXXX")"
stage_root="$(mktemp -d "$prefix_parent/.fwupd-stage-$fwupd_commit.XXXXXX")"
source_dir="$work_dir/source"
build_dir="$work_dir/build"
venv_dir="$work_dir/venv"

git init -q "$source_dir"
git -C "$source_dir" remote add origin "$fwupd_repo"
git -C "$source_dir" fetch --depth=1 origin "$fwupd_ref"
git -C "$source_dir" checkout --detach FETCH_HEAD
if ! git -C "$source_dir" diff --quiet HEAD -- || ! git -C "$source_dir" diff --cached --quiet HEAD --; then
	echo "fresh fwupd source is not clean" >&2
	exit 2
fi
actual_source_commit="$(git -C "$source_dir" rev-parse HEAD)"
if [ "$actual_source_commit" != "$fwupd_commit" ]; then
	echo "fwupd branch resolved to $actual_source_commit, expected $fwupd_commit" >&2
	exit 2
fi

python3 -m venv "$venv_dir"
"$venv_dir/bin/python" -m pip install --disable-pip-version-check \
	"jinja2==$jinja2_version" "markupsafe==$markupsafe_version"
python3_bin="$venv_dir/bin/python"

brew_prefix="$(brew --prefix)"
pkg_config_path=""
for formula in gcab glib gnutls json-glib libjcat libusb libxmlb; do
	formula_prefix="$(brew --prefix "$formula")"
	pkg_config_path="$formula_prefix/lib/pkgconfig:$pkg_config_path"
done
pkg_config_path="$pkg_config_path$brew_prefix/lib/pkgconfig:$brew_prefix/share/pkgconfig"

PATH="$(dirname "$python3_bin"):$brew_prefix/bin:/usr/bin:/bin"
export PATH
export PKG_CONFIG_PATH="$pkg_config_path"

meson_options="\
-Dbuild=standalone \
-Dtests=true \
-Ddocs=disabled \
-Dman=false \
-Dintrospection=disabled \
-Dmetainfo=false \
-Dbash_completion=false \
-Dfish_completion=false \
-Dsystemd=disabled \
-Dpolkit=disabled \
-Dlogind=disabled \
-Dbluez=disabled \
-Dpassim=disabled \
-Dreadline=disabled \
-Dumockdev_tests=disabled \
-Dlibdrm=disabled \
-Dblkid=disabled \
-Dvalgrind=disabled \
-Dgnutls=enabled \
-Dopenssl=disabled \
-Dlibmnl=disabled \
-Dlvfs=false \
-Dvendor_metadata=false \
-Dsupported_build=disabled \
-Dplugin_uefi_capsule_splash=false \
-Defi_binary=false \
-Dudev_hotplug=false \
-Dvendor_ids_dir=$brew_prefix/share/misc \
-Dpython=$python3_bin"

# meson_options contains separate command arguments.
# shellcheck disable=SC2086
meson setup "$build_dir" "$source_dir" --prefix="$prefix" $meson_options
meson compile -C "$build_dir"
test_names="$(meson test -C "$build_dir" --list | awk '
$0 ~ /^fwupd:/ &&
$0 != "fwupd:fu-usb-backend-test" &&
$0 != "fwupd:fu-engine-test" {print $0}
')"
if [ -z "$test_names" ]; then
	echo "fwupd did not configure any non-hardware tests" >&2
	exit 2
fi
# C test names contain no spaces. Intentional field splitting passes each name.
# shellcheck disable=SC2086
meson test -C "$build_dir" --no-rebuild fwupd-rust $test_names
# The Darwin USB backend test has a synthetic HID case and skips physical
# enumeration in self-test mode, so keep it in the build verification.
FWUPD_SELF_TEST=1 meson test -C "$build_dir" --no-rebuild fu-usb-backend-test
DESTDIR="$stage_root" meson install -C "$build_dir"

staged_prefix="$stage_root$prefix"
tool_path="$staged_prefix/bin/fwupdtool"
if [ ! -x "$tool_path" ]; then
	echo "fwupdtool was not installed at $tool_path" >&2
	exit 2
fi
"$python3_bin" - "$staged_prefix" "$fwupd_version" "$fwupd_commit" <<'PY'
import hashlib
import json
import pathlib
import sys

prefix = pathlib.Path(sys.argv[1])
roots = ("bin", "etc/fwupd", "lib", "share/fwupd")
hashes = {}
for relative_root in roots:
    root = prefix / relative_root
    if not root.is_dir():
        raise SystemExit(f"missing fwupd runtime directory: {relative_root}")
    for path in sorted(root.rglob("*")):
        if not path.is_file():
            continue
        relative_path = path.relative_to(prefix).as_posix()
        hashes[relative_path] = hashlib.sha256(path.read_bytes()).hexdigest()

required = (
    "bin/fwupdtool",
    "lib/fwupd-2.2.1/libfwupdcli.dylib",
    "lib/fwupd-2.2.1/libfwupdengine.dylib",
    "lib/fwupd-2.2.1/libfwupdplugin.dylib",
    "lib/libfwupd.3.dylib",
    "share/fwupd/quirks.d/builtin.quirk.gz",
)
for relative_path in required:
    if relative_path not in hashes:
        raise SystemExit(f"missing required fwupd runtime file: {relative_path}")

manifest = {
    "fwupd_version": sys.argv[2],
    "source_commit": sys.argv[3],
    "binary_sha256": hashes["bin/fwupdtool"],
    "runtime_sha256": hashes,
}
(prefix / "manifest.json").write_text(
    json.dumps(manifest, indent=2, sort_keys=True) + "\n",
    encoding="utf-8",
)
PY
printf '%s\n' "$managed_marker_value" >"$staged_prefix/$managed_marker_name"

if [ -e "$prefix" ]; then
	backup_prefix="$prefix_parent/.fwupd-$fwupd_version.previous.$$"
	if [ -e "$backup_prefix" ]; then
		echo "temporary fwupdtool backup already exists: $backup_prefix" >&2
		exit 2
	fi
	mv "$prefix" "$backup_prefix"
fi
if ! mv "$staged_prefix" "$prefix"; then
	echo "cannot activate managed fwupdtool prefix" >&2
	exit 2
fi
if [ -n "$backup_prefix" ]; then
	rm -rf "$backup_prefix"
	backup_prefix=""
fi

echo "$prefix/bin/fwupdtool"
