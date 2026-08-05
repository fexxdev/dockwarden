# Dockwarden fwupd transport migration

## Goal

Use the verified Darwin HID transport from the fwupd branch as Dockwarden's
only macOS firmware transport. Remove the Dockwarden HID and Dell protocol
writer. Keep recognition and firmware inventory read-only through
`fwupdtool --plugins dell_dock --json get-devices`.

## Safety boundary

- `status`, `doctor`, and inventory use only `get-devices`.
- The updater passes the selected fwupd `DeviceId` to `install` only from the
  existing explicit `--apply` path.
- No test invokes `install` against a physical dock.
- A missing, ambiguous, or untrusted fwupd device stops before any download or
  install operation.

## Implementation steps

1. Add a managed fwupd inventory client for macOS.
   - Verify the managed prefix, manifest, source commit, and fwupd version.
   - Run the tool in the isolated environment already used by the writer.
   - Match one WD19 parent and its child components.
   - Convert component versions into Dockwarden observations.
2. Replace the Darwin inspector, permission probe, and updater preflight with
   the inventory client.
   - Use the selected parent `DeviceId` as the install target.
   - Compare candidate component versions before downloading the CAB.
   - Keep post-install verification through fwupd JSON.
3. Remove the native macOS HID and Dell HID-I2C implementation, its tests, and
   the old fwupd patch.
4. Pin the macOS build to the fwupd Darwin branch and remove patch handling
   from the build and manifest.
5. Run Go tests, static checks, cross-builds, the fwupd build checks, and a
   read-only `status` probe when the managed binary and dock are available.

## Verification

- Unit tests cover inventory parsing, serial matching, duplicate and missing
  devices, version comparison, permission errors, and target binding.
- `go test ./...` and `go vet ./...` pass.
- Darwin and Linux cross-builds pass.
- `status` performs no install or write command.
