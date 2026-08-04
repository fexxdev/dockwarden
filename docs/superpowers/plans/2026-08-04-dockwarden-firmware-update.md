# Dockwarden firmware update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a guarded Linux firmware update command for the Dell Dock WD19.

**Architecture:** The CLI builds a read-only update plan through the existing
Dell catalog client. With `--apply`, a separate fwupd backend downloads and
hash-checks the official Linux CAB, then invokes `fwupdmgr local-install`.
macOS has no write backend and returns a clear unsupported result.

**Tech Stack:** Go standard library, Dell HTTPS metadata, `fwupdmgr` on Linux.

## Global Constraints

- Keep the project standard-library only.
- Never flash without the explicit `--apply` flag.
- Accept only Dell HTTPS metadata and download hosts.
- Verify the published SHA-256 before invoking fwupdmgr.
- Do not invoke `sudo` from dockwarden.
- Use TDD: write one failing behavior test, run it, implement the minimum,
  and run the test again.
- Tests must not write to or update the physical dock.

### Task 1: Add the update command contract

**Files:**
- Modify: `internal/cli/args.go`
- Test: `internal/cli/args_test.go`
- Modify: `internal/domain/domain.go`
- Modify: `internal/app/app.go`
- Test: `internal/app/app_test.go`

**Interfaces:**
- `cli.Options` produces `Command == "update"` and `Apply`.
- `app.Dependencies` consumes `FirmwareUpdater`.
- `FirmwareUpdater` consumes a detected dock and firmware candidate, and
  returns `domain.UpdateCheck`.

- [ ] Write a failing parser test for `update --apply` and rejection of
  `--apply` on `scan`.
- [ ] Run `GOCACHE=/tmp/dockwarden-go-cache go test ./internal/cli` and verify
  the new parser behavior fails because it is not implemented.
- [ ] Add the `update` command, `Apply` option, and usage text.
- [ ] Run the CLI tests and verify they pass.
- [ ] Write a failing app test proving `update` is plan-only without `Apply`,
  and a second test proving `--apply` calls the updater once.
- [ ] Run `GOCACHE=/tmp/dockwarden-go-cache go test ./internal/app` and verify
  the new tests fail because the update path is not wired.
- [ ] Add the updater interface and update-command branch. Preserve existing
  `check-updates` behavior.
- [ ] Run the app tests and verify they pass.

### Task 2: Parse verified Dell payload metadata

**Files:**
- Modify: `internal/domain/domain.go`
- Modify: `internal/dell/catalog.go`
- Modify: `internal/dell/catalog_test.go`
- Modify: `internal/dell/testdata/wd19-driver-page.html`

**Interfaces:**
- `domain.FirmwareCandidate.DownloadURL` stores the verified package URL.
- `dell.ParseDriverPage` returns a candidate only with a valid Dell download
  URL and published SHA-256.

- [ ] Write a failing test for parsing a `dl.dell.com` URL matching the CAB
  package name.
- [ ] Run `GOCACHE=/tmp/dockwarden-go-cache go test ./internal/dell` and verify
  the test fails because `DownloadURL` is not populated.
- [ ] Extract the matching HTTPS download link from the original HTML and
  validate `dl.dell.com` or a Dell subdomain.
- [ ] Run the Dell tests and verify they pass.
- [ ] Write a failing test for rejecting an HTTP or non-Dell download host.
- [ ] Run the focused test and verify the failure is the expected validation.
- [ ] Keep the candidate unavailable when the URL or checksum is missing.
- [ ] Run the full Dell test package and verify it passes.

### Task 3: Implement the Linux fwupd backend

**Files:**
- Create: `internal/update/fwupd.go`
- Test: `internal/update/fwupd_test.go`

**Interfaces:**
- `FwupdUpdater.Apply(context.Context, *domain.Dock, *domain.FirmwareCandidate) domain.UpdateCheck`.
- The backend uses injectable `HTTPDoer` and `CommandRunner` interfaces.

- [ ] Write a failing test proving a matching Dell candidate is downloaded,
  verified, and passed to `fwupdmgr local-install`.
- [ ] Run `GOCACHE=/tmp/dockwarden-go-cache go test ./internal/update` and
  verify it fails because the backend does not exist.
- [ ] Add temporary-file streaming download with a 64 MiB limit and SHA-256
  verification.
- [ ] Invoke `fwupdmgr local-install <path> --assume-yes` only after the hash
  check succeeds.
- [ ] Run the focused backend test and verify it passes.
- [ ] Write failing tests for hash mismatch, download failure, fwupdmgr
  failure, and non-WD19 input.
- [ ] Implement each guard with a result state and actionable reason.
- [ ] Run all backend tests and verify the temporary file is removed.

### Task 4: Wire platform-specific sources and docs

**Files:**
- Modify: `cmd/dockwarden/main.go`
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-08-04-dockwarden-design.md`

**Interfaces:**
- Linux uses Dell driver `4p6vj` for the CAB candidate.
- macOS keeps Dell driver `nkjg6` for metadata and has no updater backend.

- [ ] Add the Linux and macOS source URLs and instantiate `FwupdUpdater`
  only on Linux.
- [ ] Update README usage, supported platforms, safety rules, and result
  states.
- [ ] Update the original read-only spec so its current implementation and
  later phases match the new guarded write path.
- [ ] Run `gofmt` on changed Go files and inspect the diff for unrelated edits.

### Task 5: Verify and commit

**Files:**
- No new files.

- [ ] Run `GOCACHE=/tmp/dockwarden-go-cache go test ./...`.
- [ ] Run `GOCACHE=/tmp/dockwarden-go-cache go vet ./...`.
- [ ] Build native macOS and Linux binaries with the task cache.
- [ ] Run live `scan`, `doctor`, and plan-only `update` on this Mac.
- [ ] Run a secret and write-path scan.
- [ ] Inspect `git diff --check` and `git status --short --branch`.
- [ ] Commit all feature changes on `main` without a co-author trailer.
