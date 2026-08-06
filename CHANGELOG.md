# Changelog

All notable changes to `dockwarden` are recorded here.

## [Unreleased]

- Use the fwupd Darwin branch for the managed macOS `fwupdtool` runtime.
- Avoid the persistent Darwin HID input callback that made WD19 output reports
  fail with macOS `kIOReturnBusy`.
- Remove the Dockwarden Darwin patch and duplicate native Dell HID writer.
- Read macOS dock inventory and permission state through fwupd JSON.
- Bind the explicit install path to the read-only selected fwupd `DeviceId`.
- Add tests for serial matching, component inventory, version comparison, and
  read-only preflight.
- Report the physical Gen1 USB endpoint when fwupd cannot expose its HID
  serial, while keeping firmware apply fail-closed.
- Pin the managed fwupdtool runtime to the tested Darwin USB topology
  diagnostics commit.
- Enumerate macOS USB HID endpoints through IOKit when libusb returns no
  devices, and bind each writable HID session to its serial and LocationID.
- Report a stale macOS HID permission when native discovery sees a dock but
  fwupd reports no devices.

## [0.3.0] - 2026-08-05

### Added

- Native macOS HID access through IOKit and CoreFoundation.
- Dell HID-I2C packet support for WD19 reads and writes.
- Native macOS updates for the WD19 embedded controller, USB hubs, and package metadata.
- Pre-write version checks and MST safety guards.
- Pinned Dell CAB fallback when the Dell metadata page returns HTTP 403.
- Tests for HID packets, firmware version comparison, and guarded writes.
- Tests for the macOS fwupdtool bridge and its temporary state isolation.
- Cross-platform CI, security guidance, contribution guidance, and project credits.
- Prebuilt release archives for macOS arm64/amd64 and Linux amd64/arm64.
- A checksum-verified bootstrap installer and deterministic archive packaging.
- A read-only macOS HID permission probe and clear Input Monitoring instructions.
- Automatic Linux `fwupd` installation through the detected distribution package manager.
- A recovery matrix for staged, interrupted, partial, non-enumerating, and repeated-failure states.
- Official Dell download, recovery, support, and WD19 documentation links in the README.

### Changed

- macOS and Linux now use Dell driver `389W0` and the official Linux CAB.
- The Windows Dell updater is never executed.
- Release builds inject the tag version into `dockwarden --version`.
- macOS release archives include the complete managed `fwupdtool` runtime.
- Release installation runs a read-only macOS permission check and verifies `fwupdmgr` on Linux.

### Documentation

- Added a step-by-step guide for adding support for other docking stations.
- Added the firmware risk warning to the README.
- Added the verified WD19 update incident report with pre- and post-update versions.
- Added English installation, release, and recovery guidance.

### Firmware safety

- Made the macOS fwupdtool path managed and independent from `PATH`.
- Added full fwupd runtime hashes and compile/runtime version checks.
- Added native HID preflight and exact fwupd DeviceId selection before install.
- Added a same-process Dell serial check immediately before the fwupd writer.
- Removed inherited writer environment and the CAB extractor `PATH` lookup.
- Compared verified WD19 component versions and failed closed on incomplete data.
- Normalized the actual upstream Dell dock component names on Linux.
- Blocked newer MST payloads in both macOS firmware writers.
- Added upstream fwupd port tests and a macOS CI fwupdtool build.
- Bound native HID opens to the detected WD19 location and serial.
- Added board, power, EC baseline, update-status, retry, and relock guards.
- Added read-only component firmware reporting on macOS.
- Reported accepted updates as `update_staged` until the dock is reconnected
  and checked again.
- Added private text logs with fwupd command tails and post-install checks.
- Reclassified fwupd errors only when all candidate versions are verified.
- Reopened the macOS HID handle after transient dock re-enumeration.
- Updated the pinned Dell candidate to driver `389W0` and its verified CAB.
- Added Dell Akamai request headers and a pinned fallback for metadata HTTP 403.
- Fixed extraction of root-level firmware members from the 389W0 CAB.
- Added a macOS bridge to upstream standalone `fwupdtool` 2.2.1.
- Added an isolated macOS fwupdtool build with the libusb backend.

## [0.2.0-dev]

- Added the guarded firmware update command.
- Added Linux `fwupdmgr` integration.
- Added official Dell package metadata and SHA-256 verification.

## [0.1.0-dev]

- Added WD19 discovery, topology reporting, service checks, and JSON output.
