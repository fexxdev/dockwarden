---
name: dockwarden-firmware-safety
description: Safe Dockwarden firmware workflow for Dell docks and fwupd. Use this skill whenever the user mentions firmware, fwupd, flash, update, install, apply, write, repair, recovery, a dock being connected, or asks whether a firmware operation is ready. Start with read-only checks, bind every action to the detected dock, and never infer permission to write from earlier messages.
compatibility: Requires the repository tools and shell. macOS checks may use codesign, sqlite3, jq, and /usr/bin/log.
---

# Dockwarden firmware safety

Protect the dock first. A firmware write can leave a dock unusable. The default
mode is read-only, even when the user asks to "try" an update.

## Safety gate

1. State the mode before running a command: read-only or write.
2. Treat every new write as requiring a current, explicit user confirmation.
   Do not reuse a confirmation from an earlier turn.
3. If the user says the computer will be unattended, stop at read-only checks.
4. Do not run install, update, apply, flash, write, or recovery commands
   during diagnosis. Do not hide them in a script.
5. Never use sudo, a TCC database write, a device reset, or a USB rebind to
   work around a permission failure without a separate user request.

## Read-only preflight

Collect and save the following evidence before any write:

- exact executable path (command -v, realpath, and stat);
- executable version and source commit, when the build embeds one;
- code signature and CDHash on macOS;
- one physical dock identity: manufacturer, model, serial, VID, PID, and location;
- current component versions from the native reader and fwupd;
- power, display, MST, USB, and downstream-device checks;
- the selected firmware package, checksum, source, and target device ID;
- permission state and the execution context (normal terminal or sandbox).

Use an isolated state/cache directory for test runs. Capture stdout, stderr, and
the exit code. A JSON error object is not a successful device list.

## Target binding

Before a write, require all of these conditions:

- exactly one compatible dock is present, or the user selects one by stable ID;
- the device ID passed to fwupd matches the preflight device ID;
- the package model and firmware component match the detected dock;
- no competing dock can be selected by a generic CAB-only command;
- the dock has stable external power and the host will not sleep;
- no display, USB, or MST change is in progress.

If any condition is unknown, report blocked: preflight incomplete. Do not guess.

## Write mode

Only after the current confirmation and a complete preflight:

1. Print the exact command, binary path, package checksum, target ID, and scope.
2. Ask for confirmation if the command or target changed after the last approval.
3. Start a timestamped text log before the writer starts.
4. Run one component or one fwupd transaction at a time, as supported by the
   writer. Do not start a second writer while the first one is alive.
5. Treat disconnect, timeout, permission error, or unexpected re-enumeration as a
   stop condition. Do not retry a write automatically.
6. Keep the host powered and awake. Do not unplug the dock or its power supply.

Progress is not proof of success. A stop at 70% does not say which component was
written. Record the writer's final exit code and component-level result.

## Post-write verification

After a write, without starting another write:

- wait for the documented re-enumeration window;
- run the read-only inventory again;
- compare every component version with the package manifest;
- verify the dock serial, model, services, and MST state;
- save the before/after JSON and log paths;
- label the result success, partial, failed, or unknown.

Never call an update successful from a progress bar, a changed package version,
or a process exit code alone. If evidence conflicts, use unknown and explain the
conflict.

## Recovery posture

If a write stops or the dock disappears:

- do not start another flash;
- keep power connected and record the last component and error;
- test detection only after the documented wait;
- use the Dell recovery procedure for the exact model;
- use a second host or Windows recovery only when the vendor procedure requires it;
- preserve logs before changing cables, ports, or firmware state.

## Report format

Use this short report:

    Mode: read-only | write
    Binary: <absolute path> (version, source commit, CDHash)
    Target: <model, serial, VID:PID, location, device ID>
    Preflight: pass | blocked (reason)
    Command: <exact command>
    Exit code: <number>
    Before/after: <component versions or unknown>
    Result: success | partial | failed | unknown
    Evidence: <JSON and log paths>
    Flash performed: yes | no
