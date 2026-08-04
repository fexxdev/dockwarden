# Dockwarden design

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
Linux. The first version must inspect the connected dock without changing it.

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

The first version will not flash firmware. It will not run a throughput test,
electrical power test, display signal test, or audio loopback test.

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

- macOS: IORegistry data from `ioreg`, with a later native IOKit/HID adapter;
- Linux: `sysfs`, `udev`, `lsusb`, and `fwupdmgr` when installed;
- shared model matching: Dell VID/PID and component identifiers;
- shared output: typed data converted to text or JSON.

The first adapter may call trusted system tools. Direct USB/HID access is a
later step, after the read-only model and version checks are verified.

## Data flow

1. Discover candidate Dell USB and Thunderbolt devices.
2. Group child devices under the dock root when the platform reports topology.
3. Match identifiers against the WD19 family.
4. Collect component descriptors and available version fields.
5. Collect host-visible service state for Ethernet, audio, and USB children.
6. Query official Dell metadata only for `check-updates`.
7. Render the result as text or JSON with stable field names.

## Safety and errors

All initial commands are read-only and need no administrator privileges.

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

The live Mac WD19 will be used as an integration fixture. No firmware write
will be tested in this phase.

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

## Later phases

After the read-only MVP passes, add firmware version reads through the Dell
HID protocol and Linux `fwupd` integration. Only after that phase will we
consider a write path, with model matching, component-level comparison,
external-power checks, logs, recovery guidance, and an explicit confirmation.
