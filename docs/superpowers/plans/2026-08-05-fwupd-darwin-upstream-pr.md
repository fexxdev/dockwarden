# fwupd Darwin HID Upstream PR Implementation Plan

> For agentic workers: use the approved design in
> `docs/superpowers/specs/2026-08-05-fwupd-darwin-upstream-pr-design.md`.

**Goal:** Add a safe macOS HID transport to fwupd and make Dell Dock plugin
loading safe when Linux-only plugins are unavailable.

**Architecture:** Keep device discovery in the existing USB backend. Use
`IOHIDManager` only for HID open, close, and report I/O on Darwin. Match by
VID, PID, and serial. Retry one report after a device re-enumerates. Keep the
Linux libusb path unchanged. Guard plugin ordering rules by platform.

**Tech Stack:** C, GLib, IOKit, CoreFoundation, Meson, GTest, GitHub Actions.

## Global constraints

- Work in an isolated fwupd clone and target the current `main` branch.
- Do not add firmware files, CAB hashes, Dockwarden environment variables, or
  application-specific target selection.
- Do not run `fwupdtool install`, `fwupdmgr update`, or any flash operation.
- Tests must use synthetic data or no-device error paths. No physical dock is
  required.
- Preserve the existing Linux HID implementation and Dell Dock transaction
  flow.

## Tasks

### Task 1: Establish the upstream baseline

**Files:** fwupd clone only.

- Create a feature branch from current `main`.
- Record the existing macOS configure/build result and test inventory.
- Confirm the current Dell Dock device-type fix is already present.

**Verify:** `git status`, Meson configure, and the focused baseline tests.

### Task 2: Add test-first coverage

**Files:**

- `libfwupdplugin/fu-hid-device-test.c`
- `libfwupdplugin/meson.build`
- `plugins/dell-dock/fu-self-test.c`
- `plugins/dell-dock/meson.build`

- Test Darwin HID selection errors for no match and ambiguous VID/PID
  matches.
- Test report error mapping and the one-reopen retry decision through small
  platform helpers, without opening a real device.
- Parse a synthetic Dell Dock data record and assert the dock type and
  marketing name remain correct after the safe Rust struct parser migration.
- Assert the Dell Dock ordering rule points to `synaptics_mst` on Linux and is
  absent on Darwin.

**Verify:** Focused GTests fail before the implementation and pass after it.

### Task 3: Implement the Darwin HID transport

**Files:**

- `libfwupdplugin/fu-hid-device.c`
- `libfwupdplugin/fu-hid-device.h` if a private testable helper needs a
  declaration.
- `meson.build`

- Add Darwin-only `IOHIDManager` selection by VID, PID, and exact serial.
- Return a clear not-found or ambiguous-selection error.
- Open and close the selected `IOHIDDeviceRef` with balanced CoreFoundation
  ownership.
- Map input, output, and feature reports to the existing fwupd HID API.
- Reopen once after transient disconnect, timeout, or not-open errors.
- Link `CoreFoundation` and `IOKit` through Meson for shared and static
  builds.

**Verify:** Compile with warnings enabled on macOS and run the focused HID
tests. Confirm the Linux build path has no Darwin-only symbols.

### Task 4: Make plugin ordering platform-safe

**Files:**

- `plugins/dell-dock/fu-dell-dock-plugin.c`
- `plugins/intel-usb4/fu-intel-usb4-plugin.c`

- Do not add a Dell Dock rule for `synaptics_mst` on Darwin.
- Do not add an Intel USB4 rule for `thunderbolt` on Darwin.
- Keep the existing rules on platforms where the referenced plugins build.

**Verify:** Build the plugin set on macOS and inspect the resulting plugin
list. No missing-plugin dependency is allowed.

### Task 5: Add macOS non-hardware CI and documentation

**Files:**

- `.github/workflows/macos.yml`
- `plugins/dell-dock/README.md`
- `libfwupdplugin/meson.build` or test files as required by Meson.

- Run `meson test` in the macOS workflow after the build.
- Document that macOS uses IOKit HID access, requires the normal macOS input
  permission, and does not promise support for every composite dock.
- State that CI does not flash hardware.

**Verify:** YAML validation, Meson test discovery, and a clean documentation
diff review.

### Task 6: Review and publish the PR

- Run focused tests, the complete non-hardware test suite where dependencies
  allow, and a clean macOS build.
- Inspect the diff for unsafe ownership, Linux regressions, and flash commands.
- Run a secret sweep before commits.
- Push the feature branch to the authenticated fork and open a PR against
  `fwupd/fwupd:main`.
- Link issues #10564 and #10697 and the landed Dell Dock device-type fix.

**Final verification:** The PR description states the hardware limitation and
contains no claim that a physical firmware update was performed.
