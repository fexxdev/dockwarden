# Changelog

All notable changes to `dockwarden` are recorded here.

## [Unreleased]

### Documentation

- Added a step-by-step guide for adding support for other docking stations.
- Added the firmware risk warning to the README.

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

- macOS now uses Dell driver `4P6VJ` and the official Linux CAB.
- The Windows Dell updater is never executed.

## [0.2.0-dev]

- Added the guarded firmware update command.
- Added Linux `fwupdmgr` integration.
- Added official Dell package metadata and SHA-256 verification.

## [0.1.0-dev]

- Added WD19 discovery, topology reporting, service checks, and JSON output.
