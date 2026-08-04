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
- check official Dell firmware metadata.
- prepare a verified Linux firmware update for the WD19;
- invoke `fwupdmgr` only with explicit `update --apply`.

The current release cannot:

- write dock firmware on macOS;
- execute Dell Windows `.exe` packages;
- accept arbitrary or forced firmware payloads;
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

On Linux, apply the verified Dell CAB with an explicit write flag:

```sh
./dockwarden update --apply
```

The command does not invoke `sudo`. Run it with the privilege model used by
your fwupd installation. The command downloads the official Dell Linux CAB,
checks its SHA-256, and calls `fwupdmgr local-install`.

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

Linux firmware metadata uses Dell driver `4P6VJ`. `update --apply` verifies
the published CAB checksum before invoking fwupd.

On macOS, the Dell update check reads driver `NKJG6`. The page currently
publishes a Windows package, so macOS `update --apply` reports unsupported and
does not download or execute it.

## Development

Run tests and static checks:

```sh
go test ./...
go vet ./...
GOOS=linux GOARCH=amd64 go build ./cmd/dockwarden
```

The first release uses only the Go standard library.
