#!/bin/sh
set -eu

root="$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)"
tmp_root="$(mktemp -d "${TMPDIR:-/tmp}/dockwarden-installer-test.XXXXXX")"
trap 'rm -rf "$tmp_root"' EXIT HUP INT TERM
host_os="$(uname -s)"

package_dir="$tmp_root/package"
install_dir="$tmp_root/bin"
home_dir="$tmp_root/home"
marker="$tmp_root/firmware-was-written"
mkdir -p "$package_dir" "$install_dir" "$home_dir"
test_path="$PATH"
if [ "$(uname -s)" = "Darwin" ]; then
	fake_bin="$tmp_root/fake-bin"
	brew_log="$tmp_root/brew.log"
	mkdir -p "$fake_bin"
	cat >"$fake_bin/brew" <<'SCRIPT'
#!/bin/sh
printf '%s\n' "$*" >>"${DOCKWARDEN_TEST_BREW_LOG:?}"
exit 0
SCRIPT
	chmod 0755 "$fake_bin/brew"
	test_path="$fake_bin:$PATH"
else
	fake_bin="$tmp_root/fake-bin"
	mkdir -p "$fake_bin"
	cat >"$fake_bin/fwupdmgr" <<'SCRIPT'
#!/bin/sh
printf '%s\n' 'fwupdmgr test stub'
SCRIPT
	chmod 0755 "$fake_bin/fwupdmgr"
	test_path="$fake_bin:$PATH"
fi

cat >"$package_dir/dockwarden" <<'SCRIPT'
#!/bin/sh
if [ "${1:-}" = "--version" ]; then
	printf '%s\n' '0.3.0'
elif [ "${1:-}" = "--json" ] && [ "${2:-}" = "doctor" ]; then
	printf '%s\n' '{"state":"no_dock","warnings":[]}'
else
	: >"${DOCKWARDEN_TEST_MARKER:?}"
fi
SCRIPT
chmod 0755 "$package_dir/dockwarden"
printf '%s\n' '# test readme' >"$package_dir/README.md"
printf '%s\n' '# test changelog' >"$package_dir/CHANGELOG.md"
printf '%s\n' 'test license' >"$package_dir/LICENSE"
cp "$root/tools/release/install.sh" "$package_dir/install.sh"
chmod 0755 "$package_dir/install.sh"
if [ "$(uname -s)" = "Darwin" ]; then
	mkdir -p "$package_dir/fwupd-2.2.1/bin"
	printf '%s\n' 'fake fwupdtool' >"$package_dir/fwupd-2.2.1/bin/fwupdtool"
	chmod 0755 "$package_dir/fwupd-2.2.1/bin/fwupdtool"
	printf '%s\n' '{"fwupd_version":"2.2.1"}' >"$package_dir/fwupd-2.2.1/manifest.json"
fi

PATH="$test_path" \
DOCKWARDEN_TEST_BREW_LOG="${brew_log:-$tmp_root/brew.log}" \
DOCKWARDEN_TEST_PLATFORM="$host_os" \
HOME="$home_dir" \
DOCKWARDEN_INSTALL_DIR="$install_dir" \
DOCKWARDEN_TEST_MARKER="$marker" \
sh "$package_dir/install.sh"

test -x "$install_dir/dockwarden"
test "$("$install_dir/dockwarden" --version)" = "0.3.0"
test ! -e "$marker"

if [ "$(uname -s)" = "Darwin" ]; then
	mac_package="$tmp_root/mac-package"
	mac_install="$tmp_root/mac-bin"
	mac_home="$tmp_root/mac-home"
	mkdir -p "$mac_package/fwupd-2.2.1/bin" "$mac_install" "$mac_home"
	cp "$root/tools/release/install.sh" "$mac_package/install.sh"
	cp "$package_dir/dockwarden" "$mac_package/dockwarden"
	chmod 0755 "$mac_package/install.sh" "$mac_package/dockwarden"
	printf '%s\n' '# test readme' >"$mac_package/README.md"
	printf '%s\n' '# test changelog' >"$mac_package/CHANGELOG.md"
	printf '%s\n' 'test license' >"$mac_package/LICENSE"
	printf '%s\n' 'fake fwupdtool' >"$mac_package/fwupd-2.2.1/bin/fwupdtool"
	chmod 0755 "$mac_package/fwupd-2.2.1/bin/fwupdtool"
	printf '%s\n' '{"fwupd_version":"2.2.1"}' >"$mac_package/fwupd-2.2.1/manifest.json"
	PATH="$test_path" \
	HOME="$mac_home" \
	DOCKWARDEN_INSTALL_DIR="$mac_install" \
	DOCKWARDEN_TEST_MARKER="$marker" \
	DOCKWARDEN_TEST_BREW_LOG="$brew_log" \
	sh "$mac_package/install.sh"
	test -f "$mac_home/Library/Application Support/dockwarden/fwupd-2.2.1/manifest.json"
	test -s "$brew_log"
fi

linux_package="$tmp_root/linux-package"
linux_install="$tmp_root/linux-bin"
linux_home="$tmp_root/linux-home"
linux_fake_bin="$tmp_root/linux-fake-bin"
linux_fwupdmgr="$linux_fake_bin/fwupdmgr"
mkdir -p "$linux_package" "$linux_install" "$linux_home" "$linux_fake_bin"
cp "$root/tools/release/install.sh" "$linux_package/install.sh"
cp "$package_dir/dockwarden" "$linux_package/dockwarden"
chmod 0755 "$linux_package/install.sh" "$linux_package/dockwarden"
printf '%s\n' '# test readme' >"$linux_package/README.md"
printf '%s\n' '# test changelog' >"$linux_package/CHANGELOG.md"
printf '%s\n' 'test license' >"$linux_package/LICENSE"
cat >"$linux_fake_bin/sudo" <<'SCRIPT'
#!/bin/sh
"$@"
SCRIPT
chmod 0755 "$linux_fake_bin/sudo"
cat >"$linux_fake_bin/apt-get" <<'SCRIPT'
#!/bin/sh
if [ "${1:-}" = "install" ]; then
	cat >"${DOCKWARDEN_TEST_FWUPDMGR_PATH:?}" <<'FWUPD'
#!/bin/sh
printf '%s\n' 'fwupdmgr test stub'
FWUPD
	chmod 0755 "${DOCKWARDEN_TEST_FWUPDMGR_PATH:?}"
fi
SCRIPT
chmod 0755 "$linux_fake_bin/apt-get"
PATH="$linux_fake_bin:$PATH" \
DOCKWARDEN_TEST_PLATFORM=Linux \
DOCKWARDEN_TEST_FWUPDMGR_PATH="$linux_fwupdmgr" \
HOME="$linux_home" \
DOCKWARDEN_INSTALL_DIR="$linux_install" \
DOCKWARDEN_TEST_MARKER="$marker" \
DOCKWARDEN_LOCAL_INSTALL=1 \
sh "$linux_package/install.sh"
test -x "$linux_install/dockwarden"
test -x "$linux_fwupdmgr"

printf '%s\n' 'installer tests passed'
