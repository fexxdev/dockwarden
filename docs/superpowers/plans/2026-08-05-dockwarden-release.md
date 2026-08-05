# Dockwarden v0.3.0 Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish a reproducible `v0.3.0` release with an English README, safe bootstrap installer, prebuilt binaries, bundled macOS fwupd runtime, checksums, and full changelog notes.

**Architecture:** Keep the Go application unchanged at runtime except for a linker-injectable version variable. Build release archives with a shell package script. The archive installer installs the Go binary and, on macOS, the complete managed fwupd prefix. A tag-triggered GitHub Actions workflow builds each native target and creates the GitHub release.

**Tech Stack:** Go 1.22+, cgo on macOS, POSIX shell, macOS `otool`/`install_name_tool`, GitHub Actions, `tar`, SHA-256 tools.

## Global Constraints

- Source builds report `0.3.0-dev`; release builds inject the tag version.
- Release tags use `vMAJOR.MINOR.PATCH`, with the first release tag `v0.3.0`.
- The installer never invokes `dockwarden update --apply` or any firmware writer.
- macOS release archives include the complete managed `fwupd-2.2.1` prefix.
- The macOS prefix must keep the existing manifest and runtime validation contract.
- No physical dock is used by builds, packaging, tests, or release jobs.
- All user-facing README and release copy is written in English.

---

### Task 1: Make the application version injectable

**Files:**
- Modify: `cmd/dockwarden/main.go:20-22`
- Test: `cmd/dockwarden/main_test.go`

**Interfaces:**
- Produces: package variable `version` that accepts `-ldflags "-X main.version=..."`.

- [ ] **Step 1: Write the failing test**

Add a test that asserts the source default remains `0.3.0-dev` and that the
version variable is addressable as a package variable.

```go
func TestDefaultVersion(t *testing.T) {
	versionPointer := &version

	if *versionPointer != "0.3.0-dev" {
		t.Fatalf("unexpected default version: %q", version)
	}
}
```

- [ ] **Step 2: Run the focused test**

Run: `GOCACHE=/tmp/dockwarden-go-cache go test ./cmd/dockwarden -run TestDefaultVersion -count=1`

Expected: FAIL because the current version is a constant.

- [ ] **Step 3: Write the minimal implementation**

Change `const version = "0.3.0-dev"` to `var version = "0.3.0-dev"`. Keep the
existing `--version` output path unchanged.

- [ ] **Step 4: Run the focused test and linker check**

Run:

```sh
GOCACHE=/tmp/dockwarden-go-cache go test ./cmd/dockwarden -run TestDefaultVersion -count=1
tmp_bin="$(mktemp /tmp/dockwarden-version.XXXXXX)"
GOCACHE=/tmp/dockwarden-go-cache go build -ldflags '-X main.version=0.3.0' -o "$tmp_bin" ./cmd/dockwarden
test "$("$tmp_bin" --version)" = "0.3.0"
rm -f "$tmp_bin"
```

Expected: PASS and the built binary prints `0.3.0`.

- [ ] **Step 5: Commit**

```sh
git add cmd/dockwarden/main.go cmd/dockwarden/main_test.go
git commit -m "build: inject release version"
```

### Task 2: Add the archive and bootstrap installer

**Files:**
- Create: `tools/release/install.sh`
- Test: `tools/release/install_test.sh`

**Interfaces:**
- Consumes: an extracted archive containing `dockwarden`, `README.md`,
  `CHANGELOG.md`, `LICENSE`, and optionally `fwupd-2.2.1/`.
- Produces: `$HOME/.local/bin/dockwarden` and, on macOS, the complete managed
  prefix at `$HOME/Library/Application Support/dockwarden/fwupd-2.2.1`.
- Consumes: `DOCKWARDEN_VERSION` and `DOCKWARDEN_INSTALL_DIR` for tests and
  pinned installations.

- [ ] **Step 1: Write the shell test cases**

Create a test script that builds a fake local archive, runs the installer with
`DOCKWARDEN_INSTALL_DIR` set to a temporary directory, and checks:

```sh
test -x "$install_dir/dockwarden"
test "$("$install_dir/dockwarden" --version)" = "0.3.0"
test ! -e "$install_dir/firmware-was-written"
```

Add a macOS-only case that supplies fake `brew` and checks that the complete
`fwupd-2.2.1/manifest.json` is copied. The fake `brew` must record calls but
must not install or execute firmware commands.

- [ ] **Step 2: Run the shell tests to verify they fail**

Run: `sh tools/release/install_test.sh`

Expected: FAIL because `tools/release/install.sh` does not exist.

- [ ] **Step 3: Implement local archive installation**

Implement `set -eu` shell code that:

1. Resolves its own directory without using `eval`.
2. Uses `DOCKWARDEN_INSTALL_DIR` or `$HOME/.local/bin` for the binary.
3. Installs the binary through a temporary file and an atomic `mv`.
4. On macOS, checks `brew` and the formulas `glib gnutls json-glib libjcat
   libusb libxmlb`, installs missing formulas, then atomically replaces the
   complete managed fwupd prefix.
5. Verifies `dockwarden --version` after installation.
6. Prints `dockwarden status` as the next read-only command.
7. Never contains or invokes `update --apply`, `fwupdtool install`, or an
   arbitrary executable from `PATH`.

- [ ] **Step 4: Add standalone bootstrap mode**

When no local `dockwarden` binary is next to the script, discover the latest
tag from the GitHub releases API, map `uname -s` and `uname -m` to one of the
four archive names, download the archive and `SHA256SUMS`, verify the selected
archive with `shasum -a 256` or `sha256sum`, extract into a temporary directory,
and re-run the local installer. Support `DOCKWARDEN_VERSION=v0.3.0` to pin a
release. Fail closed when the OS or architecture is unsupported.

- [ ] **Step 5: Run shell syntax and behavior tests**

Run:

```sh
sh -n tools/release/install.sh tools/release/install_test.sh
sh tools/release/install_test.sh
```

Expected: PASS, with no firmware command in the fake command log.

- [ ] **Step 6: Commit**

```sh
git add tools/release/install.sh tools/release/install_test.sh
git commit -m "feat: add safe release installer"
```

### Task 3: Package relocatable macOS fwupd archives

**Files:**
- Create: `tools/release/package.sh`
- Test: `tools/release/package_test.sh`

**Interfaces:**
- Consumes: `package.sh VERSION GOOS GOARCH BINARY OUTPUT_DIR [FWUPD_PREFIX]`.
- Produces: `dockwarden-VERSION-GOOS-GOARCH.tar.gz` in `OUTPUT_DIR`.

- [ ] **Step 1: Write package test cases**

Create a test that supplies a fake versioned binary and a minimal fake Linux
package input, runs `package.sh`, extracts the archive, and checks exactly:

```sh
test -x extracted/dockwarden
test -f extracted/install.sh
test -f extracted/README.md
test -f extracted/CHANGELOG.md
test -f extracted/LICENSE
```

Add a missing-input case that must fail before producing an archive.

- [ ] **Step 2: Run package tests to verify they fail**

Run: `sh tools/release/package_test.sh`

Expected: FAIL because `package.sh` does not exist.

- [ ] **Step 3: Implement deterministic archive assembly**

Create a temporary staging directory, copy the binary and the three project
documents plus `install.sh`, set the binary mode to `0755`, and create the
versioned archive with stable file ordering. Reject unsupported target pairs
and missing inputs. For macOS, copy the complete fwupd prefix under
`fwupd-2.2.1/` and require `manifest.json` and `bin/fwupdtool`.

- [ ] **Step 4: Relocate internal macOS Mach-O paths**

Before copying the macOS prefix, use `otool -L` and `install_name_tool` to
replace dependencies that point inside the build prefix with
`@loader_path/<relative-path>`. Replace internal dylib IDs so no build-machine
absolute path remains. Leave `/usr/lib`, `/System/Library`, and the matching
default Homebrew prefix unchanged. Recompute `manifest.json` hashes after the
Mach-O edits, including all files under `bin`, `etc/fwupd`, `lib`, and
`share/fwupd`.

- [ ] **Step 5: Verify the packaged macOS runtime without hardware**

Run the packaged `fwupdtool --version --json` with isolated state and scan all
Mach-O dependencies for the build prefix. Run the existing managed-prefix Go
test against the relocated prefix. Expected: version `2.2.1`, no build-prefix
path, and no firmware write command.

- [ ] **Step 6: Run package tests and commit**

Run:

```sh
sh -n tools/release/package.sh tools/release/package_test.sh
sh tools/release/package_test.sh
```

Expected: PASS.

```sh
git add tools/release/package.sh tools/release/package_test.sh
git commit -m "build: package release archives"
```

### Task 4: Rewrite the English README and promote the changelog

**Files:**
- Modify: `README.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces: an English README whose first section is `Install` and second
  section is `Does it work?`.

- [ ] **Step 1: Replace the README structure**

Keep the project title and safety warning, then add a plain table of contents
without a heading so `## Install` is the first section. Include every later
heading in the index. The Install section must show the one-line bootstrap
installer, archive installation, prerequisites, and the read-only first-run
commands. The Does it work? section must contain the verified WD19 incident
report and the pre/post version table.

- [ ] **Step 2: Promote the release changelog**

Change `## [0.3.0-dev] - 2026-08-04` to `## [0.3.0] - 2026-08-05`, move the
release-worthy Unreleased entries into that heading, and leave a new empty
`[Unreleased]` section for future changes. Keep all historical entries.

- [ ] **Step 3: Check documentation**

Run:

```sh
git diff --check
rg -n "^## |^### " README.md CHANGELOG.md
```

Expected: the README heading order starts with `Install`, then `Does it work?`,
and every heading has a matching table-of-contents link.

- [ ] **Step 4: Commit**

```sh
git add README.md CHANGELOG.md
git commit -m "docs: publish English install and update report"
```

### Task 5: Add the tag-triggered release workflow

**Files:**
- Create: `.github/workflows/release.yml`

**Interfaces:**
- Consumes: pushed tags matching `v*.*.*`.
- Produces: four archives, `SHA256SUMS`, standalone `install.sh`, and a
  GitHub release whose notes are the full `CHANGELOG.md`.

- [ ] **Step 1: Add matrix build jobs**

Use native runners:

```yaml
- os: macos-14,  goos: darwin, goarch: arm64
- os: macos-13,  goos: darwin, goarch: amd64
- os: ubuntu-latest, goos: linux, goarch: amd64
- os: ubuntu-latest, goos: linux, goarch: arm64
```

Install Homebrew formulas and build the managed fwupd prefix only in the
macOS jobs. Build Go with `-ldflags "-X main.version=${TAG#v}"`, package the
archive, and upload it as a uniquely named artifact.

- [ ] **Step 2: Add release assembly job**

Download all matrix artifacts, run `sha256sum` over every archive, copy the
standalone `install.sh`, and call `gh release create` with `CHANGELOG.md` as
the notes file. Grant only `contents: write` to the workflow.

- [ ] **Step 3: Add workflow safety checks**

Validate that the pushed tag equals the changelog version, run Go tests, vet,
and a native build in each relevant job, and assert that no step contains an
apply command. Keep the existing CI workflow unchanged.

- [ ] **Step 4: Validate YAML and commit**

Run: `git diff --check`

Then:

```sh
git add .github/workflows/release.yml
git commit -m "ci: publish tagged release artifacts"
```

### Task 6: Full verification and publish v0.3.0

**Files:**
- Verify: all changed files and generated release assets.

- [ ] **Step 1: Run the complete local verification**

Run:

```sh
GOCACHE=/tmp/dockwarden-go-cache go test -count=1 ./...
GOCACHE=/tmp/dockwarden-go-cache go vet ./...
CGO_ENABLED=1 GOCACHE=/tmp/dockwarden-go-cache go build -o /tmp/dockwarden-native ./cmd/dockwarden
GOOS=linux GOARCH=amd64 GOCACHE=/tmp/dockwarden-go-cache go build -o /tmp/dockwarden-linux-amd64 ./cmd/dockwarden
sh -n tools/release/*.sh
sh tools/release/install_test.sh
sh tools/release/package_test.sh
git diff --check
rm -f /tmp/dockwarden-native /tmp/dockwarden-linux-amd64
```

Expected: every command exits `0`; no binary or temporary directory remains
in the repository.

- [ ] **Step 2: Run a secret sweep**

Search tracked and untracked project files for private keys, GitHub tokens,
AWS access keys, and hard-coded credentials. Expected: no matches.

- [ ] **Step 3: Push commits and tag**

```sh
git push origin main
git tag -a v0.3.0 -m "Release v0.3.0"
git push origin v0.3.0
```

- [ ] **Step 4: Verify the release workflow**

Use GitHub Actions to confirm all four matrix jobs and the release assembly job
pass. Confirm the release contains all four archives, `SHA256SUMS`,
`install.sh`, and the full changelog notes. Do not run any firmware command.
