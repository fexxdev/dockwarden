# dockwarden

`dockwarden` is a command-line utility for Dell docking stations on macOS and
Linux. The current target is the Dell Dock WD19.

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
a newer MST payload until a native MST writer is available.

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
runs.

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
the temporary file after use.

After a successful update, unplug and reconnect the dock USB-C cable.

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

The updater uses Dell driver `4P6VJ`, which publishes the Linux CAB for the
WD19 family. If Dell blocks the dynamic metadata page with HTTP 403,
`dockwarden` uses a pinned official CAB and still verifies its SHA-256.

The Windows `.exe` is never executed. The project does not bundle Dell
firmware blobs.

## Development

Run the test suite and static checks:

```sh
GOCACHE=/tmp/dockwarden-go-cache go test ./...
GOCACHE=/tmp/dockwarden-go-cache go vet ./...
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
