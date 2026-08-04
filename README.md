# dockwarden

dockwarden is a CLI for Dell docking stations on macOS and Linux.

The current target is the Dell Dock WD19.

## Status

The current release can:

- identify a Dell Dock WD19;
- report USB identifiers, serial, and USB descriptor version;
- list dock components and downstream USB devices;
- report host-visible USB, Ethernet, and audio enumeration;
- read optional firmware versions from `fwupdmgr` on Linux;
- check official Dell firmware metadata;
- update the WD19 through `fwupdmgr` on Linux;
- update the WD19 natively through HID on macOS;
- write firmware only with explicit `update --apply`.

The current release cannot:

- execute Dell Windows `.exe` packages;
- accept arbitrary or forced firmware payloads;
- update a newer MST firmware through the native macOS writer;
- test charging power, display output, network throughput, or audio quality.

A USB descriptor version is not a firmware version.

## Usage

Build the binary:

```sh
go build -o dockwarden ./cmd/dockwarden
```

Run the checks or prepare an update plan:

```sh
./dockwarden scan
./dockwarden status
./dockwarden doctor
./dockwarden check-updates
./dockwarden update
```

Apply the verified Dell CAB with an explicit write flag:

```sh
./dockwarden update --apply
```

On Linux, the command checks the SHA-256 and calls `fwupdmgr local-install`.
It does not invoke `sudo`; use the privilege model configured for fwupd.

On macOS, the command extracts the verified CAB with `bsdtar`, compares the
EC, hub, MST, and package versions, then writes supported WD19 components over
the native HID interface. Build the native macOS binary with cgo enabled.
After a successful update, unplug and reconnect the dock USB-C cable.

Use JSON for automation:

```sh
./dockwarden --json scan
```

Use `--verbose` with text output to show the component list.

Exit codes are:

- `0`: the dock was detected;
- `1`: no supported dock was detected;
- `2`: the command or an explicit firmware apply failed.

## Platform adapters

On macOS, dockwarden reads:

```text
ioreg -p IOUSB -l -w 0
```

On Linux, it reads `lsusb`. It also probes `fwupdmgr get-devices` when available.

Firmware metadata uses Dell driver `4P6VJ`, which publishes the Linux CAB for
the WD19 family. If Dell blocks the dynamic metadata page with HTTP 403,
dockwarden uses the pinned official CAB and still verifies its SHA-256. The
Windows `.exe` is never executed.

## Development

Run tests and static checks:

```sh
go test ./...
go vet ./...
GOOS=linux GOARCH=amd64 go build ./cmd/dockwarden
```

The first release uses only the Go standard library.
