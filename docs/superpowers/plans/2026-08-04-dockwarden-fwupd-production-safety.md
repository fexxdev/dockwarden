# Dockwarden fwupd production safety implementation plan

**Goal:** Make the macOS WD19 apply path deterministic, targeted, and guarded.

**Architecture:** Keep fwupdtool as the only active macOS writer. Add a native
read-only HID preflight before it. Require the managed fwupd 2.2.1 build and an
exact WD19 DeviceId. Let the catalog publish an update only after component
version comparison.

**Tech stack:** Go 1.22, shell, Meson, fwupd 2.2.1, GitHub Actions.

---

## Task 1: Add shared firmware version comparison

**Files:**

- Create: `internal/firmwareversion/version.go`
- Create: `internal/firmwareversion/version_test.go`
- Modify: `internal/update/macos.go`

- [ ] Add tests for equal, newer, older, leading-zero, and invalid versions.
- [ ] Run the new tests and confirm that they fail.
- [ ] Move the current comparison rules into `firmwareversion.Compare`.
- [ ] Make the macOS planner use the shared comparison.
- [ ] Run `go test ./internal/firmwareversion ./internal/update`.

## Task 2: Compare catalog component versions

**Files:**

- Modify: `internal/domain/domain.go`
- Modify: `internal/dell/catalog.go`
- Modify: `internal/dell/catalog_test.go`
- Modify: `internal/discovery/fwupd.go`
- Modify: `internal/discovery/fwupd_test.go`
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`

- [ ] Add candidate component versions to the domain model.
- [ ] Add tests for newer, equal, older, missing, and conflicting versions.
- [ ] Add a test that a live candidate inherits data only after a hash match.
- [ ] Add Linux fwupd component-name normalization tests.
- [ ] Confirm the new tests fail for the current unconditional decision.
- [ ] Add the five verified versions to the pinned WD19 candidate.
- [ ] Return `update_available` only when one component is newer.
- [ ] Return `up_to_date` when no component is newer.
- [ ] Return `version_check_unavailable` for incomplete evidence.
- [ ] Make apply mode return exit code 2 for unavailable version checks.
- [ ] Run `go test ./internal/dell ./internal/discovery ./internal/app`.

## Task 3: Extract the native macOS preflight

**Files:**

- Modify: `internal/update/macos.go`
- Modify: `internal/update/macos_test.go`

- [ ] Add read-only preflight tests for safe, unsafe, pending, and no-update cases.
- [ ] Confirm the tests fail because no reusable preflight exists.
- [ ] Read the exact HID target and all current component versions.
- [ ] Reuse board, power, EC baseline, state, CAB, and MST checks.
- [ ] Return the WD19 service tag, module serial, and update decision.
- [ ] Keep native-writer behavior unchanged.
- [ ] Run `go test ./internal/update`.

## Task 4: Attest and bind fwupdtool

**Files:**

- Modify: `internal/update/fwupdtool.go`
- Modify: `internal/update/fwupdtool_test.go`
- Modify: `cmd/dockwarden/main.go`

- [ ] Add tests that reject an empty, relative, missing, or changed tool.
- [ ] Add tests for wrong compile and runtime fwupd versions.
- [ ] Add tests for zero, one, and multiple matching WD19 devices.
- [ ] Add a test that install receives the full selected DeviceId.
- [ ] Add tests that preflight failure and no-update stop before install.
- [ ] Confirm these tests fail against the current PATH fallback.
- [ ] Resolve the managed default path without searching `PATH`.
- [ ] Verify executable mode, manifest values, binary hash, and version JSON.
- [ ] Select one Dell plugin device by EC instance ID and HID serial.
- [ ] Run install with the verified CAB and exact DeviceId.
- [ ] Treat every nonzero install exit as `update_failed`.
- [ ] Wire the native preflight into the Darwin application.
- [ ] Run `go test ./internal/update ./cmd/dockwarden`.

## Task 5: Build and test the macOS fwupd port

**Files:**

- Modify: `tools/fwupd-macos/build-fwupdtool.sh`
- Modify: `tools/fwupd-macos/README.md`
- Modify: `.github/workflows/ci.yml`

- [ ] Add a script syntax check before changes.
- [ ] Replace the temporary default prefix with the managed path.
- [ ] Enable upstream tests and run the non-hardware suite.
- [ ] Write the build manifest after installation.
- [ ] Add a macOS CI build and read-only version smoke test.
- [ ] Run `sh -n tools/fwupd-macos/build-fwupdtool.sh`.
- [ ] Build the port locally without running `install FILE`.
- [ ] Run the tool version and device-enumeration commands only.

## Task 6: Update operator documentation

**Files:**

- Modify: `README.md`
- Modify: `SECURITY.md`
- Modify: `CHANGELOG.md`

- [ ] Document the managed tool path and build command.
- [ ] Document the component-version and exact-target gates.
- [ ] State that build and enumeration do not flash firmware.
- [ ] Keep the physical update and recovery warning explicit.

## Task 7: Review and release the change

**Files:** All changed files.

- [ ] Run focused tests after each red-green cycle.
- [ ] Run `go test -count=1 ./...`.
- [ ] Run `go test -race -count=1 ./...`.
- [ ] Run `go vet ./...`.
- [ ] Build the native binary and the Linux cross-build.
- [ ] Run shell syntax and `git diff --check` checks.
- [ ] Run a fresh safety review and resolve its findings.
- [ ] Sweep tracked changes for secrets.
- [ ] Commit without a co-author trailer and push `main`.

