# dockwarden

`dockwarden` is a safety-first command-line tool for Dell docking stations on
macOS and Linux. The current target is the Dell Dock WD19.

> **Safety notice:** Firmware updates can leave a dock unusable if power or
> USB-C connectivity is lost. Review the read-only plan first. Keep the dock
> on stable power. Never run an unattended firmware update.

[![CI](https://github.com/fexxdev/dockwarden/actions/workflows/ci.yml/badge.svg)](https://github.com/fexxdev/dockwarden/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Contents**

- [Install](#install)
- [Does it work?](#does-it-work)
- [What it does](#what-it-does)
- [Safety model](#safety-model)
- [Firmware source](#firmware-source)
- [Development](#development)
- [Documentation and credits](#documentation-and-credits)

## Install

The simplest installation uses the signed release checksums and the bootstrap
installer. It detects macOS or Linux and the host architecture, downloads the
matching archive, verifies its SHA-256, and installs the binary for the current
user.

```sh
curl -fsSL https://github.com/fexxdev/dockwarden/releases/latest/download/install.sh | sh
```

The installer places `dockwarden` in `$HOME/.local/bin`. Add that directory to
`PATH` if it is not already present. On macOS, the installer also installs the
complete managed `fwupdtool` runtime and checks the required Homebrew formulae.
If Homebrew is not installed, the installer prints the exact prerequisite
command and stops. It never writes firmware.

To install a specific release, pin the tag:

```sh
curl -fsSL https://github.com/fexxdev/dockwarden/releases/latest/download/install.sh \
  | DOCKWARDEN_VERSION=v0.3.0 sh
```

To install an archive that you downloaded manually:

```sh
mkdir dockwarden-v0.3.0-darwin-arm64
tar -xzf dockwarden-v0.3.0-darwin-arm64.tar.gz \
  -C dockwarden-v0.3.0-darwin-arm64
cd dockwarden-v0.3.0-darwin-arm64
./install.sh
```

Release `v0.3.0` provides archives for macOS arm64, macOS amd64, Linux amd64,
and Linux arm64, plus the `SHA256SUMS` file. Each archive contains the binary,
installer, README, changelog, and license. The macOS archives also contain the
managed `fwupd-2.2.1` prefix. Keep that complete prefix. Do not copy only its
`fwupdtool` executable.

On macOS, grant Input Monitoring permission to the terminal or application
that runs `dockwarden`. On Linux, install `fwupdmgr` using the distribution's
normal package manager. Dockwarden does not invoke `sudo`.

Run these commands first. They are read-only:

```sh
dockwarden --version
dockwarden status
dockwarden update
```

`dockwarden update` shows the plan and does not download or write firmware.
Only `dockwarden update --apply` can start an update. Use it only with the
dock connected, stable power, an attended computer, and an explicit decision
to proceed.

## Does it work?

Yes. The verified WD19 development run updated the dock. The update command
itself returned an error during progress, so the final result was accepted only
after a physical reconnect and a fresh read-only status check.

The observed versions were:

| Component | Before | After | Result |
| --- | --- | --- | --- |
| Package | `01.00.47.01` | `01.01.01.01` | Updated |
| Embedded controller | `01.01.00.13` | `01.01.00.15` | Updated |
| USB hub Gen1 | `01.23` | `01.23` | Already current |
| USB hub Gen2 | `01.62` | `01.62` | Already current |
| MST | `05.07.08` | `05.07.08` | Already current |

The install used the official Dell CAB
`DellDockFirmwarePackage_WD19_WD22_HD22_WD25_SD25_01.01.11.cab`. Dockwarden
verified its SHA-256 before handing it to upstream `fwupdtool`.

The relevant output stopped at:

```text
Writing…: 70.5%
fwupdtool: exit status 1
```

The old logger retained only the first 4096 bytes, so the final low-level
error is not available. The output contains several device restart and wait
phases. This is consistent with a transient HID or USB re-enumeration loss,
but it does not prove the cause. fwupd also printed a package validation
warning. The log does not prove that warning caused the interruption.

After the recovery reconnect, status reported the new package and embedded
controller versions, no checks, and no warnings. This confirms the effective
hardware result even though the original process did not exit cleanly.

The local evidence is preserved in `dockwarden-flash-20260805-0005.txt` and
`dockwarden-recovery-status-20260805.txt`. New logs preserve both the start and
the end of command output. Dockwarden performs one install attempt only. It
does not retry an install after an error; it performs a read-only verification
instead.

## What it does

Dockwarden can:

- identify the WD19 USB device and its topology;
- report USB, Ethernet, audio, and downstream USB services;
- read component firmware versions through native HID on macOS;
- check official Dell firmware metadata and the pinned CAB checksum;
- create a read-only update plan;
- update the WD19 through `fwupdmgr` on Linux;
- update the WD19 through the upstream standalone `fwupdtool` on macOS.

The macOS writer uses the upstream Dell Dock plugin with libusb enumeration
and an Apple IOHIDManager transport for HID reports. Dockwarden selects one
exact WD19 DeviceId, checks the serial in the same process, and verifies every
candidate component after the install.

Dockwarden does not execute Dell Windows `.exe` packages. It does not accept
arbitrary payloads, forced downgrades, or unverified firmware files. A USB
descriptor version is not a firmware version.

## Safety model

Before an install, macOS checks the dock identity, board, power state, update
state, five component versions, CAB members, and the MST policy. It selects a
single `dell_dock` DeviceId by plugin, embedded-controller instance ID, and
`<service-tag>/<module-serial>` serial. The Dell plugin reads the hardware
serial again immediately before its writer runs. Any failed check stops the
install.

An `update_staged` result means that the platform accepted the update and
requires a physical reconnect. Reconnect the dock USB-C cable, then run
`dockwarden status`. An `update_verified` result means that fwupd returned
success, or returned an error after the dock reported every candidate version.
If verification is not possible, the result remains `update_failed` or
`update_staged`.

Every command writes user-only text logs to `dockwarden-log.txt`. Use
`--log-file PATH` to select another file. Update logs include the preflight,
selected DeviceId, fwupd commands, command output head and tail, post-install
version verification, and the final state. Protect logs because they can
contain dock identifiers.

Exit codes are:

- `0`: the command succeeded;
- `1`: no supported dock was detected;
- `2`: the command or an explicit firmware apply failed.

## Firmware source

The updater uses Dell driver `389W0`, which publishes the Linux CAB for the
WD19 family. If Dell blocks its metadata page with HTTP 403, Dockwarden uses
browser-compatible request headers and a pinned official CAB. It verifies the
CAB SHA-256 before any firmware operation.

The candidate records package, embedded-controller, USB hub Gen1, USB hub
Gen2, and MST versions. A live Dell candidate inherits those values only when
its SHA-256 matches the pinned CAB. Missing or conflicting evidence gives
`version_check_unavailable`; apply mode exits with code `2` and does not write.

The Windows `.exe` is never executed. The project does not bundle Dell
firmware blobs.

## Development

Build a development binary with Go 1.22 or newer:

```sh
GOCACHE=/tmp/dockwarden-go-cache go test ./...
GOCACHE=/tmp/dockwarden-go-cache go vet ./...
CGO_ENABLED=1 GOCACHE=/tmp/dockwarden-go-cache go build ./cmd/dockwarden
```

Run the release packaging checks without hardware:

```sh
sh -n tools/release/*.sh
sh tools/release/install_test.sh
sh tools/release/package_test.sh
```

The macOS fwupd port is built from the pinned upstream commit. See
[the macOS build guide](tools/fwupd-macos/README.md). The build runs upstream
non-hardware tests and does not update a dock.

Tests use fake HID, HTTP, and command interfaces. They never flash a physical
dock. See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow.

## Documentation and credits

- [Adding support for another docking station](docs/ADDING-DOCK-SUPPORT.md)
- [Building the macOS fwupdtool port](tools/fwupd-macos/README.md)
- [Change log](CHANGELOG.md)
- [Credits and external projects](CREDITS.md)
- [Security policy](SECURITY.md)
- [MIT License](LICENSE)
