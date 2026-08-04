# macOS fwupd port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Use upstream fwupd's Dell Dock plugin for guarded macOS updates.

**Architecture:** Build standalone fwupdtool 2.2.1 with libusb and no Linux
daemon features. Dockwarden verifies the Dell CAB, then invokes fwupdtool
with isolated temporary state. Existing IOKit discovery stays read-only.

**Tech Stack:** Go standard library, upstream fwupd, Meson, Ninja, Rust,
Homebrew libusb.

## Global Constraints

- Never run a firmware install command against the physical dock.
- Never invoke `sudo` from Dockwarden or the build script.
- Keep plan-only behavior unchanged.
- Verify the CAB SHA-256 before starting fwupdtool.
- Use TDD for the new updater.
- Keep the upstream source outside the repository.

## Task 1: Define the fwupdtool bridge with tests

**Files:**
- Create: `internal/update/fwupdtool_test.go`
- Modify: `internal/update/fwupd.go`

- [x] Add a failing test for verified CAB download and fwupdtool arguments.
- [x] Add failing tests for invalid candidate, hash mismatch, and fwupdtool
  failure.
- [x] Run the focused tests and confirm the bridge is absent.
- [x] Add the updater with injectable command and HTTP runners.
- [x] Use `--plugins dell_dock`, `--assume-yes`, and `--no-reboot-check`.
- [x] Add temporary `FWUPD_LOCALSTATEDIR` and `CACHE_DIRECTORY` values.
- [x] Run the focused tests and confirm they pass.

## Task 2: Wire macOS and document the build

**Files:**
- Create: `tools/fwupd-macos/build-fwupdtool.sh`
- Create: `tools/fwupd-macos/README.md`
- Modify: `cmd/dockwarden/main.go`
- Modify: `README.md`

- [x] Add a pinned fwupd 2.2.1 build script.
- [x] Make the script fail when required tools are missing.
- [x] Wire `FwupdToolUpdater` on Darwin.
- [x] Keep the native HID reader for read-only status.
- [x] Document `DOCKWARDEN_FWUPDTOOL` and the no-flash verification rule.
- [x] Run `gofmt` and inspect the diff.

## Task 3: Verify and publish

**Files:**
- No additional files.

- [x] Build the pinned upstream fwupdtool.
- [x] Run `GOCACHE=/tmp/dockwarden-go-cache go test ./...`.
- [x] Run `GOCACHE=/tmp/dockwarden-go-cache go vet ./...`.
- [x] Run read-only fwupdtool enumeration.
- [x] Run read-only Dockwarden status and plan-only update.
- [x] Sweep for secret material and firmware write commands.
- [x] Run `git diff --check`.
- [ ] Commit and push on `main` without a co-author trailer.
