# Dockwarden Firmware Safety Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the WD19 firmware update path fail closed, verify its target, and report staged firmware honestly.

**Architecture:** Add a topology-bound HID selector in the domain layer. Reuse it for the Darwin HID backend, the macOS updater, and read-only firmware reporting. Keep Linux behind fwupd, but validate its candidate contract before download.

**Tech Stack:** Go 1.22, standard library, IOKit/CoreFoundation on macOS, fwupdmgr on Linux.

## Global Constraints

- Never run `--apply` against the physical dock during development.
- Preserve the existing exact WD19 identity check.
- Keep tests independent of network, fwupd, IOKit devices, and firmware binaries.
- Use `apply_patch` for source edits.
- Do not add third-party dependencies.

---

### Task 1: Add topology-bound HID targets

**Files:**
- Modify: `internal/domain/domain.go`
- Modify: `internal/update/macos.go`
- Modify: `internal/update/macos_test.go`
- Modify: `internal/update/fwupd_test.go`

- [x] Add `domain.HIDTarget` with vendor, product, serial, and location ID.
- [x] Add a failing test for selecting the exact base and Gen1 HID devices.
- [x] Run the focused test and confirm it fails because selector support is absent.
- [x] Implement target selection from `domain.Dock` and its related USB devices.
- [x] Run the focused test and confirm it passes.

### Task 2: Make Darwin HID selection exact

**Files:**
- Modify: `internal/macos/hid/hid_darwin.go`
- Modify: `internal/macos/hid/hid_stub.go`
- Modify: `cmd/dockwarden/main.go`

- [x] Add a failing build-level API test or compile check for the selector signature.
- [x] Run the Darwin compile check and confirm the old opener signature fails.
- [x] Match HID devices by VID, PID, location, and serial, and reject ambiguity.
- [x] Run the Darwin build and confirm it passes.

### Task 3: Add preflight guards, retry, and lock cleanup

**Files:**
- Modify: `internal/update/dell_hid.go`
- Modify: `internal/update/macos.go`
- Modify: `internal/update/dell_hid_test.go`
- Modify: `internal/update/macos_test.go`

- [x] Add failing tests for board, power, EC baseline, retry, and relock behavior.
- [x] Run the focused tests and confirm each fails for the missing behavior.
- [x] Implement the minimum guards and five-attempt report retry policy.
- [x] Track unlocked targets and relock them after failures.
- [x] Run the focused tests and confirm they pass.

### Task 4: Add staged status and read-only firmware reporting

**Files:**
- Modify: `internal/update/macos.go`
- Modify: `internal/update/macos_test.go`
- Modify: `internal/update/firmware.go`
- Create: `internal/update/firmware_test.go`
- Modify: `internal/discovery/platform.go`
- Modify: `internal/discovery/platform_test.go`
- Modify: `cmd/dockwarden/main.go`

- [x] Add a failing test for `update_staged` and component version observations.
- [x] Run the focused tests and confirm they fail because the reader is absent.
- [x] Implement the HID reader and connect it to macOS inspection.
- [x] Verify the staged status after passive activation.
- [x] Run the focused tests and confirm they pass.

### Task 5: Harden Linux candidates and update the fallback

**Files:**
- Modify: `internal/dell/catalog.go`
- Modify: `internal/update/fwupd.go`
- Modify: `internal/dell/catalog_test.go`
- Modify: `internal/update/fwupd_test.go`

- [x] Add failing tests for non-CAB, non-Linux, and non-WD19 candidates.
- [x] Run the focused tests and confirm they fail.
- [x] Enforce CAB, Linux, WD19, and package-name requirements.
- [x] Pin the verified Dell 389W0 CAB and SHA-256.
- [x] Run the focused tests and confirm they pass.

### Task 6: Update documentation and verify the release gate

**Files:**
- Modify: `README.md`
- Modify: `CHANGELOG.md`
- Modify: `SECURITY.md`
- Modify: `docs/superpowers/specs/2026-08-04-dockwarden-design.md`
- Modify: `docs/superpowers/plans/2026-08-04-dockwarden-read-only-mvp.md`

- [x] Document `update_staged`, reconnect, and status verification.
- [x] Remove stale claims that the first version has no write path.
- [x] Run the full tests, vet, macOS build, Linux build, diff check, and secret scan.
- [x] Confirm the physical dock was only read during verification.
