# Dockwarden firmware update design

Date: 2026-08-04

## Goal

Add an explicit firmware update command for the detected Dell Dock WD19.

The first write-capable backend will run on Linux through `fwupdmgr`. The
utility will download the official Dell Linux CAB, verify its SHA-256, and
pass the verified local archive to fwupd. It will not port the Dell HID/I2C
protocol in this phase.

## User experience

The command is:

```text
dockwarden update
dockwarden update --apply
```

`update` is a plan-only operation. It discovers the dock and reads the
official Dell metadata. It does not download a payload or change a device.

`update --apply` is the only write path. It requires a detected WD19, a Dell
candidate with a compatible model, an HTTPS download URL, and a valid SHA-256.
It downloads the Linux CAB to a temporary file, verifies the hash, then runs:

```text
fwupdmgr local-install <verified-cab> --assume-yes
```

The temporary file is removed after fwupd exits. Dockwarden will not invoke
`sudo`; polkit or the caller's privilege model handles authorization.

## Platform behavior

Linux uses Dell driver page `4P6VJ`, which publishes the Linux CAB and a
self-contained Linux updater. The CAB is preferred because fwupd contains the
open Dell dock plugin and can target the devices described by the archive.

macOS continues to support discovery and metadata checks. `update --apply`
returns an explicit unsupported result and never downloads or executes the
Windows `.exe` package. A later native macOS backend may use IOHIDManager and
the documented open fwupd Dell dock protocol, but it needs hardware-backed
read and write tests before it is safe.

## Safety rules

- No firmware write occurs without `--apply`.
- Only the exact detected WD19 identity `0x413c:0xb06e` is accepted.
- Only HTTPS Dell download hosts are accepted.
- The downloaded bytes must match the published SHA-256.
- The Dell executable package is never executed by dockwarden.
- A failed download or hash check stops before fwupd is invoked.
- Downgrade, force, and arbitrary local payloads are out of scope.
- Tests use fake HTTP responses and fake command runners. No test flashes a
  real device.

## Result states

The existing update result reports:

- `update_available`: a verified metadata candidate is available;
- `unsupported`: the platform has no write backend;
- `update_applied`: fwupdmgr accepted the archive;
- `update_failed`: download, verification, authorization, or fwupd failed;
- `vendor_metadata_unavailable`: Dell metadata could not be read;
- `not_checked`: no supported dock was detected.

The result reason includes the next action or the relevant fwupd error.

## Testing

Tests cover CLI parsing, plan-only behavior, macOS write blocking, Dell
download URL parsing, HTTPS host validation, SHA-256 verification, temporary
payload cleanup, and fwupdmgr invocation. Live verification only runs the
plan path on this Mac. It never runs `--apply` against the physical dock.
