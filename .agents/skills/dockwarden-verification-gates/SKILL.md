---
name: dockwarden-verification-gates
description: Verification gates for Dockwarden changes and firmware reports. Use this skill before saying fixed, working, ready, tested, updated, successful, safe, release-ready, or complete. It catches stale binaries, suppressed exit codes, JSON errors mistaken for success, sandbox-only tests, and claims that are not supported by hardware evidence.
compatibility: Requires shell tools, git, the project test commands, jq when JSON is used, and macOS diagnostics when hardware is involved.
---

# Dockwarden verification gates

Make each claim traceable to a command and an artifact. Separate code evidence,
binary evidence, runtime evidence, and physical dock evidence.

## Gate A: source and diff

Before testing:

- inspect git status and the complete diff;
- confirm every changed line serves the request;
- check the intended branch and commit;
- scan for secrets before committing;
- run git diff --check.

Do not use a dirty or stale worktree as proof of a clean result.

## Gate B: build and unit tests

Run the repository's build, unit tests, integration tests, and vet/lint checks as
appropriate. Capture each command's exit code. Do not use || true, ; true, or
discarded output to turn a failure into a pass.

Record:

- command and working directory;
- toolchain and platform;
- exit code;
- test count and failures;
- artifact path and checksum.

If a test is skipped because it needs hardware, say not exercised; do not say
passed.

## Gate C: artifact identity

The binary under test must be the binary just built. Compare:

- absolute path and realpath;
- file timestamp and size;
- architecture;
- version;
- source commit marker;
- dynamic library/data directory;
- macOS code signature and CDHash.

When two runtimes exist, name both. A passing test against an old runtime does
not validate a new runtime.

## Gate D: JSON and command results

For every JSON command:

1. save stdout and stderr separately;
2. save the exit code before parsing;
3. validate the expected JSON shape with jq -e;
4. reject an Error object as success;
5. report empty arrays as empty, not detected.

For fwupdtool get-devices, success requires exit code zero and a meaningful
Devices array. For a physical dock, include model, serial, location, plugin,
and component versions.

## Gate E: hardware claims

A build or unit test does not prove HID access or firmware safety. A hardware
claim needs:

- the exact binary path and execution context;
- the physical dock identity;
- a read-only detection result;
- permission evidence when macOS is used;
- before/after component versions for update claims;
- saved logs.

If only a sandbox test ran, label the hardware result not verified. If a probe
works only after approved unsandboxed execution, record that context.

## Gate F: firmware result

Never claim updated or successful from a progress percentage, a writer start,
or a package download. Require:

- writer exit code and final status;
- component-level result;
- re-enumeration after the documented wait;
- read-only post-inventory;
- version comparison against the intended package;
- stable dock identity after re-enumeration.

If any item is missing or conflicts, use partial, failed, or unknown.

## Completion report

Use this compact evidence table:

    Claim: <what is being asserted>
    Source commit: <commit>
    Binary: <absolute path, version, CDHash>
    Context: sandbox | normal terminal | approved unsandboxed
    Command: <exact command>
    Exit code: <number>
    Evidence: <JSON, logs, test output>
    Status: verified | not exercised | partial | failed | unknown

Before the final response, check that no command in the session performed a
write when the report says Flash performed: no.
