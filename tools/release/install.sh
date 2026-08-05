#!/bin/sh
set -eu

repository="${DOCKWARDEN_REPOSITORY:-fexxdev/dockwarden}"
required_brew_formulas="glib gnutls json-glib libjcat libusb libxmlb"
script_dir="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
temp_root=""
prefix_stage_parent=""
prefix_backup=""

die() {
	printf '%s\n' "dockwarden installer: $*" >&2
	exit 2
}

cleanup() {
	if [ -n "$prefix_stage_parent" ] && [ -d "$prefix_stage_parent" ]; then
		rm -rf "$prefix_stage_parent"
	fi
	if [ -n "$temp_root" ] && [ -d "$temp_root" ]; then
		rm -rf "$temp_root"
	fi
	if [ -n "$prefix_backup" ] && [ -d "$prefix_backup" ]; then
		printf '%s\n' "dockwarden installer: restoring previous fwupd prefix" >&2
		prefix_destination="${DOCKWARDEN_PREFIX_DESTINATION:?}"
		if [ ! -e "$prefix_destination" ]; then
			mv "$prefix_backup" "$prefix_destination"
		fi
	fi
}

trap cleanup EXIT HUP INT TERM

require_command() {
	command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

sha256_file() {
	file="$1"
	if command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$file" | awk '{print $1}'
		return
	fi
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$file" | awk '{print $1}'
		return
	fi
	die "missing SHA-256 command: install shasum or sha256sum"
}

target_os() {
	case "$(platform_name)" in
	Darwin) printf '%s\n' darwin ;;
	Linux) printf '%s\n' linux ;;
	*) die "unsupported operating system: $(platform_name)" ;;
	esac
}

platform_name() {
	if [ -n "${DOCKWARDEN_TEST_PLATFORM:-}" ]; then
		printf '%s\n' "$DOCKWARDEN_TEST_PLATFORM"
		return
	fi
	uname -s
}

target_arch() {
	case "$(uname -m)" in
	arm64|aarch64) printf '%s\n' arm64 ;;
	x86_64|amd64) printf '%s\n' amd64 ;;
	*) die "unsupported architecture: $(uname -m)" ;;
	esac
}

run_privileged() {
	if [ "$(id -u)" -eq 0 ]; then
		"$@"
		return
	fi
	command -v sudo >/dev/null 2>&1 || die "installing fwupd needs sudo; install fwupd manually, then run the installer again"
	sudo "$@"
}

ensure_linux_fwupd() {
	if command -v fwupdmgr >/dev/null 2>&1; then
		printf '%s\n' "Using fwupdmgr at $(command -v fwupdmgr)"
		return
	fi

	printf '%s\n' 'fwupdmgr was not found; installing the fwupd package.'
	if command -v apt-get >/dev/null 2>&1; then
		run_privileged apt-get update
		run_privileged apt-get install -y fwupd
	elif command -v dnf >/dev/null 2>&1; then
		run_privileged dnf install -y fwupd
	elif command -v yum >/dev/null 2>&1; then
		run_privileged yum install -y fwupd
	elif command -v pacman >/dev/null 2>&1; then
		run_privileged pacman -Sy --needed --noconfirm fwupd
	elif command -v zypper >/dev/null 2>&1; then
		run_privileged zypper --non-interactive install fwupd
	else
		die "cannot install fwupd automatically; install the fwupd package for this Linux distribution, then run the installer again"
	fi
	command -v fwupdmgr >/dev/null 2>&1 || die "fwupd installation completed but fwupdmgr is still not in PATH"
	printf '%s\n' "Installed fwupdmgr at $(command -v fwupdmgr)"
}

run_macos_permission_check() {
	printf '%s\n' 'Running the read-only macOS fwupd HID permission check.'
	doctor_output="$("$1" --json doctor 2>&1 || true)"
	if [ -n "$doctor_output" ]; then
		printf '%s\n' "$doctor_output"
	fi
	case "$doctor_output" in
		*"Input Monitoring"*|*"HID access"*)
			printf '%s\n' 'Warning: macOS denied HID access. Open System Settings > Privacy & Security > Input Monitoring, enable the terminal or app that runs dockwarden, then quit and reopen it.' >&2
			;;
	esac
}

install_local_archive() {
	package_dir="$1"
	binary_source="$package_dir/dockwarden"
	if [ ! -x "$binary_source" ]; then
		die "archive does not contain an executable dockwarden binary"
	fi

	if [ -n "${DOCKWARDEN_INSTALL_DIR:-}" ]; then
		install_dir="$DOCKWARDEN_INSTALL_DIR"
	elif [ -n "${HOME:-}" ]; then
		install_dir="$HOME/.local/bin"
	else
		die "HOME is not set; set DOCKWARDEN_INSTALL_DIR"
	fi
	mkdir -p "$install_dir"
	binary_stage="$install_dir/.dockwarden.$$"
	cp "$binary_source" "$binary_stage"
	chmod 0755 "$binary_stage"
	mv -f "$binary_stage" "$install_dir/dockwarden"

	platform="$(platform_name)"
	if [ "$platform" = "Darwin" ]; then
		brew_path="$(command -v brew 2>/dev/null || true)"
		if [ -z "$brew_path" ]; then
			die "macOS needs Homebrew. Install it, then run: brew install $required_brew_formulas"
		fi
		for formula in $required_brew_formulas; do
			if ! brew list --versions "$formula" >/dev/null 2>&1; then
				brew install "$formula"
			fi
		done

		prefix_source="$package_dir/fwupd-2.2.1"
		[ -x "$prefix_source/bin/fwupdtool" ] || die "macOS archive is missing fwupdtool"
		[ -f "$prefix_source/manifest.json" ] || die "macOS archive is missing fwupd manifest"
		prefix_parent="${HOME:?}/Library/Application Support/dockwarden"
		prefix_destination="$prefix_parent/fwupd-2.2.1"
		DOCKWARDEN_PREFIX_DESTINATION="$prefix_destination"
		export DOCKWARDEN_PREFIX_DESTINATION
		mkdir -p "$prefix_parent"
		prefix_stage_parent="$(mktemp -d "$prefix_parent/.dockwarden-prefix-stage.XXXXXX")"
		cp -R "$prefix_source" "$prefix_stage_parent/fwupd-2.2.1"
		if [ -e "$prefix_destination" ] || [ -L "$prefix_destination" ]; then
			prefix_backup="$prefix_parent/.fwupd-2.2.1.previous.$$"
			[ ! -e "$prefix_backup" ] || die "temporary fwupd backup already exists"
			mv "$prefix_destination" "$prefix_backup"
		fi
		if ! mv "$prefix_stage_parent/fwupd-2.2.1" "$prefix_destination"; then
			if [ -n "$prefix_backup" ] && [ ! -e "$prefix_destination" ]; then
				mv "$prefix_backup" "$prefix_destination"
				prefix_backup=""
			fi
			die "cannot activate managed fwupd prefix"
		fi
		rm -rf "$prefix_stage_parent"
		prefix_stage_parent=""
		if [ -n "$prefix_backup" ]; then
			rm -rf "$prefix_backup"
			prefix_backup=""
		fi
	elif [ "$platform" = "Linux" ]; then
		ensure_linux_fwupd
	else
		die "unsupported operating system: $platform"
	fi

	installed_version="$($install_dir/dockwarden --version 2>/dev/null || true)"
	[ -n "$installed_version" ] || die "installed dockwarden did not report a version"
	printf '%s\n' "Installed dockwarden $installed_version at $install_dir/dockwarden"
	if [ "$platform" = "Darwin" ]; then
		run_macos_permission_check "$install_dir/dockwarden"
	fi
	printf '%s\n' 'Next read-only check: dockwarden status'
}

bootstrap_latest() {
	require_command curl
	require_command tar
	require_command mktemp
	temp_root="$(mktemp -d "${TMPDIR:-/tmp}/dockwarden-download.XXXXXX")"

	release_version="${DOCKWARDEN_VERSION:-latest}"
	case "$release_version" in
	v*) ;;
	*) release_version="v$release_version" ;;
	esac
	if [ "$release_version" = "vlatest" ]; then
		release_json="$(curl -fsSL "https://api.github.com/repos/$repository/releases/latest")"
		release_version="$(printf '%s\n' "$release_json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
		[ -n "$release_version" ] || die "cannot determine the latest release tag"
	fi

	os_name="$(target_os)"
	arch_name="$(target_arch)"
	archive_name="dockwarden-$release_version-$os_name-$arch_name.tar.gz"
	archive_url="https://github.com/$repository/releases/download/$release_version/$archive_name"
	checksum_url="https://github.com/$repository/releases/download/$release_version/SHA256SUMS"
	archive_path="$temp_root/$archive_name"
	checksum_path="$temp_root/SHA256SUMS"
	curl -fL --retry 3 -o "$archive_path" "$archive_url"
	curl -fL --retry 3 -o "$checksum_path" "$checksum_url"
	expected_checksum="$(awk -v name="$archive_name" '$2 == name {print $1; exit}' "$checksum_path")"
	[ -n "$expected_checksum" ] || die "archive is not listed in SHA256SUMS"
	actual_checksum="$(sha256_file "$archive_path")"
	[ "$actual_checksum" = "$expected_checksum" ] || die "SHA-256 mismatch for $archive_name"

	extract_dir="$temp_root/extracted"
	mkdir -p "$extract_dir"
	tar -xzf "$archive_path" -C "$extract_dir"
	[ -x "$extract_dir/install.sh" ] || die "downloaded archive has no installer"
	DOCKWARDEN_LOCAL_INSTALL=1 sh "$extract_dir/install.sh"
}

if [ "${DOCKWARDEN_LOCAL_INSTALL:-0}" = "1" ]; then
	install_local_archive "$script_dir"
elif [ -x "$script_dir/dockwarden" ] && [ -f "$script_dir/README.md" ] && [ -f "$script_dir/install.sh" ]; then
	install_local_archive "$script_dir"
else
	bootstrap_latest
fi
