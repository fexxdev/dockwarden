# Dockwarden fwupd production safety design

Date: 2026-08-04

## Goal

Make the macOS WD19 update path deterministic and fail closed.

The change does not add support for another dock model.

## Verified fwupd contract

Dockwarden uses fwupd commit `61c7cf1873fedd78fa031e8a8829cb3413aaef46`.
This commit reports version `2.2.1`.

`fwupdtool install FILE` can select all compatible devices. Dockwarden must not
use this form.

`fwupdtool install FILE DEVICE-ID` selects one device. fwupd then includes only
devices with the same composite ID. This is correct for a WD19 CAB package.

Dockwarden passes a full 40-character device ID. It does not pass a GUID or an
ID prefix.

## Managed tool

The default tool path is inside the user's application configuration directory:

`dockwarden/fwupd-2.2.1/bin/fwupdtool`

On macOS, this resolves below `~/Library/Application Support`.

`DOCKWARDEN_FWUPDTOOL` can override this path. The override must be absolute.
Dockwarden never searches `PATH`.

The build script installs the complete fwupd prefix. It also writes a manifest.
The manifest records these values:

- fwupd version;
- pinned source commit;
- Darwin patch SHA-256;
- installed binary SHA-256.

Before network access, Dockwarden verifies the file mode, manifest, and binary
hash. It then runs `fwupdtool --version --json`. Compile and runtime versions
must both be `2.2.1`.

## Native read-only preflight

The active macOS writer reuses the existing HID protocol before fwupd writes.
It performs these read-only checks:

- exact WD19 HID target;
- Salomon dock type;
- board revision 6 or later;
- available power-supply wattage;
- EC version at or above `01.01.00.01`;
- completed prior update state;
- current package, EC, hub, and MST versions;
- valid WD19 blobs and version fields in the verified CAB.

The preflight returns the service tag and module serial. It also reports if any
CAB component is newer. A zero-update plan returns `up_to_date`. It does not run
the install command.

The native writer still rejects a newer MST because it cannot write MST. The
fwupdtool writer can accept that plan because the pinned Dell plugin supports
MST. Both paths use the same CAB parser and version checks.

## Exact fwupd target

Dockwarden runs `fwupdtool --plugins dell_dock --json get-devices` in isolated
state. It selects one device with all these properties:

- plugin `dell_dock`;
- WD19 embedded-controller instance ID;
- serial `<service-tag>/<module-serial>`;
- full 40-character hexadecimal device ID.

The module serial uses eight decimal digits, with leading zeroes.

Dockwarden rejects zero matches, multiple matches, missing fields, and malformed
JSON. It then passes the selected device ID to `install`.

Exit code 2 means `nothing to do`. Dockwarden treats it as an error after a plan
that requires an update. This result can indicate a changed target or stale
state.

## Version decision

The Dell web page supplies one package label. It does not supply reliable
versions for each WD19 component.

The pinned candidate therefore records these CAB component versions:

- package: `01.01.01.01`;
- embedded controller: `01.01.00.15`;
- USB hub Gen1: `01.23`;
- USB hub Gen2: `01.62`;
- MST: `05.07.08`.

A live Dell candidate inherits this data only when its SHA-256 matches the
pinned candidate. Dockwarden otherwise returns `version_check_unavailable`.

Dockwarden compares each known candidate component with the detected value. It
returns `update_available` only when one or more components are newer. It
returns `up_to_date` when none are newer. Missing or conflicting versions cause
`version_check_unavailable`.

Linux fwupd names are normalized to the same component names before this check.

## Build and CI

The macOS build enables upstream fwupd tests. The script runs the configured
non-hardware test suite before installation.

The macOS CI job installs build dependencies. It builds the pinned port, runs
its tests, checks the manifest, and executes the read-only version command.

No CI step accesses a physical dock or runs a firmware install command.

## Failure behavior

Every failed check stops before `install`. Temporary firmware and state files
are removed. Apply mode exits with a failure code for
`version_check_unavailable` and `update_failed`.

