# macOS fwupd port design

Date: 2026-08-04

## Goal

Use the upstream fwupd Dell Dock plugin on macOS.

The port builds the standalone `fwupdtool` binary from fwupd 2.2.1.
It uses libusb for enumeration and IOHIDManager for HID reports.
It does not require udev, D-Bus, systemd, polkit, or the fwupd daemon.

## Update flow

Dockwarden keeps the existing Dell metadata and SHA-256 checks.
After `--apply`, it downloads the official Dell CAB and verifies its hash.
It then runs:

```text
fwupdtool --plugins dell_dock --assume-yes --no-reboot-check install VERIFIED.cab
```

The tool path comes from `DOCKWARDEN_FWUPDTOOL`.
When the variable is empty, Dockwarden uses `fwupdtool` from `PATH`.

Dockwarden gives fwupdtool a temporary state and cache directory.
This avoids writes to a Homebrew or system directory.

## Safety boundary

- `update` remains plan-only.
- Only `update --apply` can invoke fwupdtool.
- The dock identity remains the exact detected WD19 `413c:b06e`.
- The package must be the verified Dell CAB.
- The package hash is checked before fwupdtool starts.
- The updater does not invoke `sudo`.
- Tests never execute `fwupdtool install`.
- No live install command is run during development verification.

The existing native HID writer stays available for protocol tests.
The macOS production wiring selects fwupdtool instead.

## Build contract

`tools/fwupd-macos/build-fwupdtool.sh` checks the required Homebrew tools,
fetches the pinned upstream commit, configures a standalone Darwin build,
and prints the resulting `fwupdtool` path after installation.

The script does not install firmware and does not access a dock.

## Verification

The port is complete when:

1. The pinned upstream source builds `fwupdtool` on Apple Silicon.
2. Go tests verify validation, hash checks, arguments, temporary state, and
   command failures with a fake runner.
3. Read-only `fwupdtool get-devices` and Dockwarden status checks run without
   an install command.
