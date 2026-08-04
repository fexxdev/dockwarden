#!/bin/sh
set -eu

fwupd_commit="61c7cf1873fedd78fa031e8a8829cb3413aaef46"
fwupd_repo="https://github.com/fwupd/fwupd.git"
cache_root="${DOCKWARDEN_FWUPD_CACHE_DIR:-${TMPDIR:-/tmp}/dockwarden-fwupd}"
prefix="${1:-${TMPDIR:-/tmp}/dockwarden-fwupd-install}"
source_dir="$cache_root/fwupd-$fwupd_commit"
build_dir="$cache_root/build-$fwupd_commit"
venv_dir="$cache_root/venv"
script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
patch_file="$script_dir/patches/0001-darwin-iohid-hid-device.patch"

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "missing required command: $1" >&2
		exit 2
	fi
}

require_command brew
require_command git
require_command meson
require_command ninja
require_command pkg-config
require_command python3
require_command rustc
require_command cargo

if [ ! -f "$patch_file" ]; then
	echo "missing fwupd Darwin patch: $patch_file" >&2
	exit 2
fi

for formula in gcab glib gnutls json-glib libjcat libusb libxmlb; do
	if ! brew list --versions "$formula" >/dev/null 2>&1; then
		echo "missing Homebrew formula: $formula" >&2
		exit 2
	fi
done

if [ ! -d "$source_dir/.git" ]; then
	mkdir -p "$cache_root"
	git clone --filter=blob:none --no-checkout "$fwupd_repo" "$source_dir"
fi
git -C "$source_dir" fetch --depth=1 origin "$fwupd_commit"
git -C "$source_dir" checkout --detach "$fwupd_commit"
if git -C "$source_dir" apply --reverse --check "$patch_file" >/dev/null 2>&1; then
	:
elif git -C "$source_dir" apply --check "$patch_file" >/dev/null 2>&1; then
	git -C "$source_dir" apply "$patch_file"
else
	echo "cannot apply the fwupd Darwin patch to $source_dir" >&2
	echo "remove that cached source directory and run this script again" >&2
	exit 2
fi

if ! python3 -c 'import jinja2' >/dev/null 2>&1; then
	if [ ! -x "$venv_dir/bin/python" ]; then
		python3 -m venv "$venv_dir"
	fi
	"$venv_dir/bin/python" -m pip install --disable-pip-version-check jinja2
	python3_bin="$venv_dir/bin/python"
else
	python3_bin="$(command -v python3)"
fi

brew_prefix="$(brew --prefix)"
pkg_config_path=""
for formula in gcab glib gnutls json-glib libjcat libusb libxmlb; do
	formula_prefix="$(brew --prefix "$formula")"
	pkg_config_path="$formula_prefix/lib/pkgconfig:$pkg_config_path"
done
pkg_config_path="$pkg_config_path$brew_prefix/lib/pkgconfig:$brew_prefix/share/pkgconfig"

export PATH="$(dirname "$python3_bin"):$brew_prefix/bin:/usr/bin:/bin"
export PKG_CONFIG_PATH="$pkg_config_path"

meson_options="\
-Dbuild=standalone \
-Dtests=false \
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

mkdir -p "$cache_root" "$prefix"
if [ -f "$build_dir/build.ninja" ]; then
	meson setup --reconfigure "$build_dir" "$source_dir" --prefix="$prefix" $meson_options
else
	meson setup "$build_dir" "$source_dir" --prefix="$prefix" $meson_options
fi
meson compile -C "$build_dir" fwupdtool
meson install -C "$build_dir"

echo "$prefix/bin/fwupdtool"
