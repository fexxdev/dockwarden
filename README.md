# dockwarden

`dockwarden` is a command-line utility for Dell docking stations on macOS and
Linux. The current target is the Dell Dock WD19.

> A safety-first firmware control plane for the dock between your laptop,
> displays, and last good USB-C cable.

> **Use at your own risk.** Firmware updates can leave a dock unusable if power or USB
> connectivity is lost. Read the plan before applying an update, and keep the dock on
> stable power.

[![CI](https://github.com/fexxdev/dockwarden/actions/workflows/ci.yml/badge.svg)](https://github.com/fexxdev/dockwarden/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## What it does

The utility can:

- identify the WD19 USB device and its topology;
- report USB, Ethernet, audio, and downstream USB enumeration;
- check official Dell firmware metadata;
- update the WD19 through `fwupdmgr` on Linux;
- update the WD19 through upstream `fwupdtool` on macOS.

The macOS status reader uses direct HID access. The macOS write path uses the
upstream Dell Dock plugin in standalone `fwupdtool`, with libusb enumeration
and an IOHIDManager transport for HID reports.

The utility does not execute Dell Windows `.exe` packages. It does not accept
arbitrary payloads, forced downgrades, or unverified firmware files.

A USB descriptor version is not a firmware version.

## Install

Build a native binary with Go 1.22 or newer:

```sh
go build -o dockwarden ./cmd/dockwarden
```

On macOS, build with cgo enabled. The status reader uses IOKit and
CoreFoundation. Build the upstream `fwupdtool` port first; see
[the macOS build guide](tools/fwupd-macos/README.md). Dockwarden uses the
managed tool at
`~/Library/Application Support/dockwarden/fwupd-2.2.1/bin/fwupdtool`.
`DOCKWARDEN_FWUPDTOOL` can override it with an absolute path. Dockwarden never
searches `PATH` for a firmware writer. macOS may require HID or Input Monitoring
permission for the terminal. If `status` reports denied HID access, grant that
permission and run it again.

On Linux, install `fwupdmgr` and use the privilege model configured by the
system. `dockwarden` does not invoke `sudo`.

## Commands

Inspect the dock:

```sh
./dockwarden scan
./dockwarden status
./dockwarden doctor
```

Check Dell metadata and show an update plan:

```sh
./dockwarden check-updates
./dockwarden update
```

Apply a firmware update:

```sh
./dockwarden update --apply
```

`update` is read-only. Only `update --apply` can download and write firmware.
The updater accepts the official Dell CAB, verifies its SHA-256, and removes
the temporary file after use. On macOS it verifies all managed runtime files
and the fwupd compile/runtime version before network access. It runs fwupd with
a minimal environment and isolated temporary state.

Before an install, macOS reads the WD19 through HID. It checks identity, board,
power, update state, five component versions, CAB members, and the MST policy.
It selects exactly one fwupd device by plugin, embedded-controller instance ID,
and `<service-tag>/<module-serial>` serial. The install receives that full
40-character DeviceId. The Dell plugin reads the hardware serial again in the
same process before its writer runs. Any failed check stops the install.

An apply result of `update_staged` means that the platform accepted the update
and requires a physical reconnect. Unplug and reconnect the dock USB-C cable.
Then run `status` and confirm the component versions. Do not interrupt power,
USB-C, or the host during an active update.

Use JSON output for automation:

```sh
./dockwarden --json scan
./dockwarden --json update
```

Use `--verbose` with text output to show the component list.

Exit codes are:

- `0`: the command succeeded;
- `1`: no supported dock was detected;
- `2`: the command or an explicit firmware apply failed.

## Firmware source

The updater uses Dell driver `389W0`, which publishes the Linux CAB for the
WD19 family. If Dell blocks the dynamic metadata page with HTTP 403,
`dockwarden` uses browser-compatible request headers and a pinned official CAB.
It still verifies the CAB SHA-256 before any firmware operation.

The candidate records package, embedded-controller, USB hub Gen1, USB hub Gen2,
and MST versions. A live Dell candidate inherits those values only when its
SHA-256 matches the pinned CAB. Missing or conflicting component evidence gives
`version_check_unavailable`; apply mode exits with code `2` and does not write.

The Windows `.exe` is never executed. The project does not bundle Dell
firmware blobs.

## Development

Run the test suite and static checks:

```sh
GOCACHE=/tmp/dockwarden-go-cache go test ./...
GOCACHE=/tmp/dockwarden-go-cache go vet ./...
CGO_ENABLED=1 GOCACHE=/tmp/dockwarden-go-cache go build ./cmd/dockwarden
GOOS=linux GOARCH=amd64 go build ./cmd/dockwarden
```

Tests use fake HID, HTTP, and command interfaces. They never flash a physical
dock. See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow.

## Documentation and credits

- [Adding support for another docking station](docs/ADDING-DOCK-SUPPORT.md)
- [Building the macOS fwupdtool port](tools/fwupd-macos/README.md)
- [Change log](CHANGELOG.md)
- [Credits and external projects](CREDITS.md)
- [Security policy](SECURITY.md)
- [MIT License](LICENSE)
