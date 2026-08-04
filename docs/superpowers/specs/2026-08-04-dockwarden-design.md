# Dockwarden design
## Current implementation

The utility uses the Dell WD19 USB identifiers `0x413c:0xb06e` and parses
macOS IORegistry or Linux `lsusb` output.

The update check reads the official WD19 driver metadata page. It accepts a
candidate only when the page provides a compatible model, version, release
date, package name, HTTPS Dell download URL, and SHA-256.

Linux `update --apply` downloads the official Dell CAB, verifies its SHA-256,
and invokes `fwupdmgr local-install`. macOS verifies the same CAB and writes
supported WD19 components through the native HID/I2C protocol.

Date: 2026-08-04

## Context

The target device is a Dell Dock WD19.

The Mac exposes the main dock device as USB vendor `0x413c`, product `0xb06e`,
with USB serial `2000`. The dock also exposes USB hubs, a Realtek Ethernet
device, audio devices, and downstream peripherals.

No `fwupdmgr`, `fwupd`, or Dell dock updater is installed on this Mac.

Dell publishes a Linux updater for the WD19 family. The `fwupd` project also
contains an open source Dell dock plugin for the dock component protocols.

## Goal

Build a cross-platform command-line utility named `dockwarden` for macOS and
Linux. Discovery and plan-only commands inspect the dock without changing it.
The explicit apply command is the only write path.

The first version must:

- identify the dock model and stable USB identifiers;
- list the dock component tree and downstream devices;
- report observable USB, Ethernet, and audio state;
- report firmware versions when the host exposes them;
- check official Dell firmware availability when a reliable source is known;
- return human-readable output and machine-readable JSON;
- explain missing permissions, tools, or device interfaces.

USB descriptor versions such as `bcdDevice` must be reported as descriptor
versions. They must never be labelled or compared as dock firmware versions.

## Non-goals

The write path will not execute Dell Windows packages, accept arbitrary
payloads, or force a downgrade. The native macOS writer currently refuses a
newer MST payload. It will not run a throughput test, electrical power test,
display signal test, or audio loopback test.

It will not distribute Dell firmware blobs. Later update support will use
official Dell packages and verify their integrity before any write operation.

## Name and commands

The binary and project name are `dockwarden`.

Initial commands:

```text
dockwarden scan
dockwarden status
dockwarden check-updates
dockwarden doctor
```

Common options:

```text
dockwarden --json scan
dockwarden --verbose status
dockwarden --version
```

`scan` focuses on identity and topology. `status` focuses on current device
state. `check-updates` only reads vendor metadata and reports candidates.
`doctor` combines checks and prints actionable diagnostics.

`check-updates` will use official Dell support metadata for the matched model.
It will record the source URL, package name, release date, package version,
compatibility, and published checksums. If the page does not provide enough
structured data for a safe comparison, the command will report that the check
is unavailable instead of guessing.

## Architecture

The first implementation will use Go. Go is already available in the
workspace and produces one native binary per target without a runtime.

The command layer will be platform independent. Discovery will use small
platform adapters:

- macOS: IORegistry data from `ioreg` and native HID access through IOKit;
- Linux: `sysfs`, `udev`, `lsusb`, and `fwupdmgr` when installed;
- shared model matching: Dell VID/PID and component identifiers;
- shared output: typed data converted to text or JSON.

The discovery adapter may call trusted system tools. The macOS writer uses
IOHIDManager for vendor-defined HID reports and keeps protocol code separate
from the platform bridge.

## Data flow

1. Discover candidate Dell USB and Thunderbolt devices.
2. Group child devices under the dock root when the platform reports topology.
3. Match identifiers against the WD19 family.
4. Collect component descriptors and available version fields.
5. Collect host-visible service state for Ethernet, audio, and USB children.
6. Query official Dell metadata for `check-updates` and the `update` plan.
7. Render the result as text or JSON with stable field names.
8. With explicit `update --apply`, verify the Dell CAB and use the platform
   writer: fwupd on Linux or HID/I2C on macOS.

## Safety and errors

The scan, status, doctor, check-updates, and plan-only update commands are
read-only. The Linux apply command needs the privilege model configured for
fwupd. The macOS apply command requires native HID access. Neither command
invokes `sudo`.

The CLI must distinguish these states:

- no Dell dock detected;
- Dell device detected but model unknown;
- WD19 detected with incomplete topology;
- component present but firmware version unavailable;
- vendor metadata unavailable;
- update candidate found;
- no update candidate found.

The CLI must never label a dock as fully healthy when it only saw a USB
descriptor. It will report observable functionality and clearly mark tests
that it cannot perform.

Observable functionality means that the host enumerates the relevant USB
interfaces and exposes the expected network, audio, or child-device service.
It does not prove link speed, charging power, display output, or audio quality.

## Testing

Tests will cover:

- parsing representative macOS `ioreg` output;
- matching WD19 identifiers;
- grouping child components;
- parsing Linux command output;
- stable JSON output;
- no-device and partial-device states;
- vendor metadata failure without a false update result.

The live Mac WD19 will be used as an integration fixture. Firmware writes are
tested only with fake HTTP responses and fake command runners.

## Success criteria

On this Mac, the following must identify the dock as a WD19 and show its
observable components:

```text
dockwarden scan
dockwarden status
dockwarden doctor
```

`dockwarden check-updates` must either report a verified Dell update candidate
or state why the check could not be completed. It must not claim success from
the USB device descriptor version alone.

`dockwarden update` must print a plan without downloading or writing.
`dockwarden update --apply` must use the verified Linux CAB and fwupd on Linux,
and the verified CAB plus native HID/I2C on macOS. It must refuse unsupported
components before writing any component.

## Later phases

Later phases may add native MST writes, richer component-level status, external-
power checks, logs, and recovery guidance. Hardware-backed recovery tests are
still required before claiming recovery from an interrupted flash.
