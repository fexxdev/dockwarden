# Dockwarden Firmware Safety Hardening Design

**Goal:** Make the WD19 firmware path fail closed and report staged updates honestly.

## Scope

The change covers the native macOS writer, the Linux fwupd adapter, the Dell
catalog, and read-only firmware reporting. It does not run a physical update.

## Safety rules

- Accept only the exact Dell Dock WD19 identity `413c:b06e`.
- Bind every HID open to the detected USB location and serial when available.
- Require the Salomon dock type, board revision `>= 6`, a non-zero power supply
  reading, EC version `>= 01.01.00.01`, and complete update status.
- Verify the complete CAB and every supported component before the first write.
- Reject a newer MST payload because the native MST writer is not implemented.
- Retry HID input and output reports up to five times.
- Relock maintenance targets after any failed operation when the transport still
  accepts commands.
- Return `update_staged` after passive activation. A later read-only status check
  verifies the component versions after the USB-C reconnect.
- Linux accepts only a Dell CAB that lists WD19 and Linux support.

## Design

`domain.HIDTarget` carries the product, serial, and location identity selected
from the discovered USB topology. Both macOS updater and read-only firmware
reader use this selector. The Darwin HID backend rejects zero or multiple
matches.

`MacUpdater` performs all reads and payload checks before it unlocks a component.
It tracks unlocked targets and cleans them up on error. It uses the same HID
retry policy for reads, commands, erase, and flash writes. After the passive
command it reads the update status and reports that the update is staged.

`MacFirmwareReader` reads the EC, hub, MST, and package versions over HID. The
macOS inspector uses it for `status` and `doctor`, so the user can verify the
firmware after reconnecting the dock.

`FwupdUpdater` validates the candidate contract before download and returns a
staged result after `fwupdmgr local-install`. The catalog parser rejects an
unknown format and non-Linux candidate for the shared WD19 path.

## Acceptance criteria

- Unit tests cover every new refusal path and every retry/cleanup path.
- Tests prove that unsupported MST, invalid board, invalid power, ambiguous HID,
  and invalid candidate format produce no flash command.
- `go test -count=1 ./...`, `go vet ./...`, native macOS build, and Linux build
  pass.
- No command in the verification process uses `--apply` against physical hardware.
