# dockwarden

dockwarden is a read-only CLI for Dell docking stations on macOS and Linux.

The current target is the Dell Dock WD19.

## Status

The MVP can:

- identify a Dell Dock WD19;
- report USB identifiers, serial, and USB descriptor version;
- list dock components and downstream USB devices;
- report host-visible USB, Ethernet, and audio enumeration;
- read optional firmware versions from `fwupdmgr` on Linux;
- check official Dell firmware metadata.

The MVP cannot:

- flash dock firmware;
- download firmware payloads;
- test charging power, display output, network throughput, or audio quality;
- read the WD19 firmware version on this Mac.

A USB descriptor version is not a firmware version.

## Usage

Build the binary:

```sh
go build -o dockwarden ./cmd/dockwarden
```

Run the read-only checks:

```sh
./dockwarden scan
./dockwarden status
./dockwarden doctor
./dockwarden check-updates
```

Use JSON for automation:

```sh
./dockwarden --json scan
```

Use `--verbose` with text output to show the component list.

Exit codes are:

- `0`: the dock was detected;
- `1`: no supported dock was detected;
- `2`: the command failed.

## Platform adapters

On macOS, dockwarden reads:

```text
ioreg -p IOUSB -l -w 0
```

On Linux, it reads `lsusb`. It also probes `fwupdmgr get-devices` when available.

The Dell update check reads the official WD19 driver page (driver ID NKJG6). It records the package name, version, release date, compatibility, and SHA-256. It does not download or execute the package.

## Development

Run tests and static checks:

```sh
go test ./...
go vet ./...
GOOS=linux GOARCH=amd64 go build ./cmd/dockwarden
```

The first release uses only the Go standard library.
