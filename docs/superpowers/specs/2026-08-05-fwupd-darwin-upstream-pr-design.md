# fwupd Darwin HID support upstream PR

## Goal

Prepare a focused upstream fwupd pull request for the reusable macOS HID
transport needed by Dell docking stations. The change must improve fwupd
itself and must not move Dockwarden application policy into fwupd.

## Scope

The pull request will target the current fwupd main branch. It will include:

- a Darwin HID transport based on `IOHIDManager` for feature, input, and
  output reports;
- deterministic HID selection by vendor ID, product ID, and device serial;
- safe close and reopen handling after a USB reset or re-enumeration;
- Meson linkage for the Apple `CoreFoundation` and `IOKit` frameworks;
- non-hardware regression tests and macOS build coverage;
- documentation of the supported macOS path and its hardware limits.

The pull request will not include:

- Dockwarden's Dell catalog, CAB hashes, or installer;
- Dockwarden's managed runtime manifest;
- Input Monitoring permission text or application-specific logging;
- the `DOCKWARDEN_EXPECTED_DELL_DOCK_SERIAL` environment variable;
- unattended-update policy, recovery policy, or Dockwarden CLI behavior;
- any firmware image or firmware write command.

## Design

The existing Linux HID path remains unchanged. On Darwin, the HID device
creates an `IOHIDManager`, matches the device by VID and PID, filters the
matching set by the fwupd device serial, and rejects zero or multiple matches.
The selected `IOHIDDeviceRef` is opened and retained until the fwupd device
closes. Reports use Apple's synchronous callback API through a short-lived
run-loop source. If a report fails because the dock re-enumerated, the code
closes the stale handle, opens the uniquely matching device again, and retries
the report once.

The Dell plugin keeps its normal fwupd device selection and transaction flow.
No environment variable or Dockwarden-specific target contract is added to
the upstream plugin.

## Verification

The implementation is complete only when:

1. The current fwupd source configures and builds `fwupdtool` on macOS.
2. Non-hardware tests pass on the development host and in CI.
3. Tests cover serial matching, ambiguous matching, report errors, close, and
   one re-open retry without accessing a physical dock.
4. A read-only macOS `fwupdtool --plugins dell_dock get-devices` check is run
   when hardware is available. It must not run `install`.
5. The PR body links issue #10564 and states that this is not a guarantee for
   every composite dock model.

No firmware flash is part of this work.
