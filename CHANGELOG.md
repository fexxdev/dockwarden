# Changelog

All notable changes to `dockwarden` are recorded here.

## [Unreleased]

### Documentation

- Added a step-by-step guide for adding support for other docking stations.
- Added the firmware risk warning to the README.

### Firmware safety

- Bound native HID opens to the detected WD19 location and serial.
- Added board, power, EC baseline, update-status, retry, and relock guards.
- Added read-only component firmware reporting on macOS.
- Reported accepted updates as `update_staged` until the dock is reconnected
  and checked again.
- Updated the pinned Dell candidate to driver `389W0` and its verified CAB.
- Added Dell Akamai request headers and a pinned fallback for metadata HTTP 403.
- Fixed extraction of root-level firmware members from the 389W0 CAB.

## [0.3.0-dev] - 2026-08-04

### Added

- Native macOS HID access through IOKit and CoreFoundation.
- Dell HID-I2C packet support for WD19 reads and writes.
- Native macOS updates for the WD19 embedded controller, USB hubs, and package metadata.
- Pre-write version checks and MST safety guards.
- Pinned Dell CAB fallback when the Dell metadata page returns HTTP 403.
- Tests for HID packets, firmware version comparison, and guarded writes.
- Cross-platform CI, security guidance, contribution guidance, and project credits.

### Changed

- macOS and Linux now use Dell driver `389W0` and the official Linux CAB.
- The Windows Dell updater is never executed.

## [0.2.0-dev]

- Added the guarded firmware update command.
- Added Linux `fwupdmgr` integration.
- Added official Dell package metadata and SHA-256 verification.

## [0.1.0-dev]

- Added WD19 discovery, topology reporting, service checks, and JSON output.
