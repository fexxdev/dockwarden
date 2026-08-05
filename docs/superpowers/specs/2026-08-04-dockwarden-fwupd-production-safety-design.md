# Dockwarden fwupd production safety design

Date: 2026-08-04

## Goal

Make the macOS WD19 update path deterministic and fail closed.

The change does not add support for another dock model.

## Verified fwupd contract

Dockwarden uses fwupd commit `74fcfabec244dc073aeb36669c5118fdfcd5107b`.
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
- installed binary SHA-256;
- every runtime file below `bin`, `etc/fwupd`, `lib`, and `share/fwupd`.

Before network access, Dockwarden verifies file modes, the exact runtime file
set, and all hashes. It then runs `fwupdtool --version --json`. Compile and
runtime versions must both be `2.2.1`. The process receives a minimal
environment. It does not inherit fwupd or dynamic-loader overrides.

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

Both writers reject a newer MST. The pinned fwupd plugin supports MST, but this
change does not expand the current safety policy. Both paths use the same CAB
parser and version checks.

## Exact fwupd target

Dockwarden runs `fwupdtool --plugins dell_dock --json get-devices` in isolated
state. It selects one device with all these properties:

- plugin `dell_dock`;
- WD19 embedded-controller instance ID;
- serial `<service-tag>/<module-serial>`;
- full 40-character hexadecimal device ID.

The module serial uses at least eight decimal digits, with leading zeroes.

Dockwarden rejects zero matches, multiple matches, missing fields, and malformed
JSON. It then passes the selected device ID to `install`. The install process
receives the expected serial. The Dell plugin reopens the EC, reads the serial
from hardware, and compares it before the first writer runs.

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

The macOS build enables upstream fwupd tests. The script starts from a fresh
pinned source tree and a pinned Jinja2 environment. It runs the configured
non-hardware test suite before installation. It excludes the USB backend test,
which requires an empty physical USB bus. It also excludes `fu-engine-test`:
macOS reports the CAB fixture as `dyn.age80g2pc`, so the upstream test cannot
select its CAB adapter.

The macOS CI job installs build dependencies. It builds the pinned port, runs
its tests, validates the full manifest, and executes read-only version and
enumeration commands.

No CI step accesses a physical dock or runs a firmware install command.

## Failure behavior

Every failed check stops before `install`. Temporary firmware and state files
are removed. Apply mode exits with a failure code for
`version_check_unavailable` and `update_failed`.
