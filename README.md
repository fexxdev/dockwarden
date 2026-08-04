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
- update supported WD19 components through native HID on macOS.

The native macOS writer updates the embedded controller, USB Gen1 hub, USB
Gen2 hub, and package metadata. It reads and compares MST firmware. It refuses
a newer MST payload until a native MST writer is available. Before any write,
it checks the dock type, board revision, power reading, EC baseline, update
status, target identity, package hash, and every component payload.

The utility does not execute Dell Windows `.exe` packages. It does not accept
arbitrary payloads, forced downgrades, or unverified firmware files.

A USB descriptor version is not a firmware version.

## Install

Build a native binary with Go 1.22 or newer:

```sh
go build -o dockwarden ./cmd/dockwarden
```

On macOS, build with cgo enabled. The native writer uses IOKit, CoreFoundation,
and the system `bsdtar` command. The dock must be connected before the command
runs. macOS may require HID or Input Monitoring permission for the terminal.
If `status` reports denied HID access, grant that permission and run it again.
The updater refuses to continue when direct HID access is unavailable.

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
the temporary file after use. It binds macOS HID access to the detected dock
location and serial. It refuses an ambiguous target.

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
- [Change log](CHANGELOG.md)
- [Credits and external projects](CREDITS.md)
- [Security policy](SECURITY.md)
- [MIT License](LICENSE)
