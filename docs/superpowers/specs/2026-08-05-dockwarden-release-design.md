# Dockwarden v0.3.0 Release Design

## Goal

Publish a first usable `v0.3.0` release with clear English documentation,
prebuilt binaries, a small installer, checksums, and the complete changelog.

## User-facing design

The README starts with a generated-style table of contents. Its first section
is `Install`; its second section is `Does it work?`. The second section gives
the verified WD19 development update report, including the pre-update versions,
the `70.5%` fwupd interruption, the recovery reconnect, and the post-update
versions. It states the confirmed facts separately from the probable but
unproven HID/USB re-enumeration explanation.

Installation is one command after downloading the release archive:

```sh
./install.sh
```

The installer selects the current OS and architecture, installs the
`dockwarden` executable, preserves user-only log permissions, and prints the
next read-only command. It never runs `update --apply`.

## Release artifacts

GitHub Actions runs on a `v*.*.*` tag and creates one GitHub release. The
workflow publishes:

- `dockwarden-v0.3.0-darwin-arm64.tar.gz`;
- `dockwarden-v0.3.0-darwin-amd64.tar.gz`;
- `dockwarden-v0.3.0-linux-amd64.tar.gz`;
- `dockwarden-v0.3.0-linux-arm64.tar.gz`;
- `SHA256SUMS`;
- the full `CHANGELOG.md` as release notes.

Every archive contains the versioned `dockwarden` binary, `install.sh`,
`README.md`, `CHANGELOG.md`, and `LICENSE`. The macOS archives also contain
the managed `fwupd-2.2.1` prefix. The workflow builds that prefix on the
matching macOS architecture, runs its upstream non-hardware tests, and
validates `fwupdtool --version --json` before packaging it.

The macOS installer checks for the Homebrew runtime libraries required by the
managed fwupd prefix. It installs missing formulae with `brew install` when
Homebrew is available; otherwise it stops with the exact prerequisite command.
It copies the complete prefix to
`~/Library/Application Support/dockwarden/fwupd-2.2.1` and never copies only
the `fwupdtool` executable.

## Versioning and build

The source version remains `0.3.0-dev` for unreleased builds. Release builds
inject the tag version with Go linker flags, so `dockwarden --version` reports
`0.3.0`. The tag must match the changelog heading and the archive names.

The release workflow builds with `CGO_ENABLED=1` on macOS and native Go builds
on Linux. It runs `go test ./...`, `go vet ./...`, and a native build before
creating archives. It runs `git diff --check` and writes SHA-256 checksums for
all archives.

## Safety constraints

- The release workflow never connects to or flashes a physical dock.
- The installer never invokes `dockwarden update --apply`.
- The macOS fwupd prefix is validated by its existing manifest before use.
- The installer uses a temporary directory and an atomic destination move.
- The release notes keep the firmware incident factual and do not claim an
  exact low-level error that the old truncated log did not preserve.

## Files

- Modify `README.md` for the English table of contents, installation guide,
  and WD19 verification report.
- Modify `CHANGELOG.md` to promote `0.3.0` from development to release.
- Modify `cmd/dockwarden/main.go` to expose a linker-injectable version value.
- Create `tools/release/install.sh` for local archive installation.
- Create `tools/release/package.sh` for deterministic archive assembly.
- Create `.github/workflows/release.yml` for tagged release builds.
- Add release tests for archive layout, checksum generation, and installer
  safety where shell behavior can be tested without hardware.
