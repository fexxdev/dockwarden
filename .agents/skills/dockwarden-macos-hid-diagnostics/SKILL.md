---
name: dockwarden-macos-hid-diagnostics
description: Diagnose Dockwarden and fwupd HID access on macOS. Use this skill whenever fwupdtool get-devices reports no devices, IOKit returns 0xe00002e2, Input Monitoring is mentioned, a binary was rebuilt or moved, or a test behaves differently in a sandbox and a real terminal. Verify the exact executable, TCC identity, and execution context before changing code or blaming the dock.
compatibility: macOS tools include codesign, sqlite3, jq, hidutil, ioreg, and /usr/bin/log. Use an approved unsandboxed context only for read-only hardware checks.
---

# Dockwarden macOS HID diagnostics

Separate three questions: can macOS enumerate the HID, can this exact process open
it, and can fwupd match it to the Dell dock plugin. A failure in one layer does
not prove a failure in the others.

## 1. Identify the binary that actually runs

Resolve the path from the command being tested. Do not assume the managed runtime
is the newly built runtime.

Record:

- command -v and realpath;
- file size, modification time, and architecture;
- codesign -dv --verbose=4 output, including CDHash and team ID;
- embedded source commit, if present;
- linked library directory and data/config directories.

Compare this identity with the path shown in the permission UI. A temporary build
and a managed user runtime are different applications to macOS. A rebuilt file at
the same path can also have a new CDHash.

## 2. Check TCC without changing it

Input Monitoring is path- and code-identity-sensitive. Read the relevant TCC
entry, when permitted, and compare:

- service kTCCServiceListenEvent;
- exact client path;
- authorization value;
- code requirement or CDHash;
- user and system database location.

Never edit TCC databases and never reset permissions as a diagnostic shortcut. If
the entry is absent or stale, give the user the exact executable path and the
System Settings > Privacy & Security > Input Monitoring steps.

## 3. Separate sandbox from macOS permission

Run the same read-only command in the same context as the user will run it. A
repository agent sandbox can deny HID even when the user's Terminal is allowed.

When the default context cannot access the real HID, request an approved
unsandboxed execution for the read-only probe. Do not use that approval for a
write. Report both results:

    Sandbox probe: blocked | passed
    Real-terminal probe: blocked | passed
    Conclusion: sandbox limitation | macOS permission | driver/device issue

Do not label a sandbox-only failure as a TCC failure.

## 4. Run a detection-only probe

Use a fresh isolated runtime directory. Capture stdout, stderr, and the exit code.
The probe must use get-devices only:

    prefix=<absolute-fwupd-runtime>
    runtime=$(mktemp -d /private/tmp/dockwarden-fwupd-probe.XXXXXX)
    mkdir -p "$runtime"/{state,cache,config,data}
    FWUPD_DATADIR="$prefix/share/fwupd" \
    FWUPD_SYSCONFDIR="$prefix/etc/fwupd" \
    FWUPD_LOCALSTATEDIR="$runtime/state" \
    XDG_CACHE_HOME="$runtime/cache" \
    XDG_CONFIG_HOME="$runtime/config" \
    XDG_DATA_HOME="$runtime/data" \
    CACHE_DIRECTORY="$runtime/cache" \
    "$prefix/bin/fwupdtool" --plugins dell_dock --json get-devices \
      >"$runtime/get-devices.json" 2>"$runtime/get-devices.log"
    rc=$?
    jq -e '.Devices | type == "array"' "$runtime/get-devices.json"

Do not append || true before recording rc. Do not treat { "Error": ... } as a
valid inventory. A successful probe has exit code zero and a non-empty Devices
array for a connected compatible dock.

## 5. Interpret the layers

- HID enumeration works, open fails with 0xe00002e2: inspect TCC, sandbox,
  process identity, and driver access.
- HID enumeration works and open works, but no Dell devices appear: inspect
  serial matching, location IDs, plugin selection, and fwupd data files.
- JSON contains an error and exit code is non-zero: report detection failure.
- The probe works only outside the agent sandbox: report the context difference;
  do not change the fwupd transport until a real-terminal probe also fails.
- Results alternate between pass and fail: inspect competing HID clients and
  recent IOHIDLibUserClient, AppleUserUSBHostHIDDevice, and
  AppleUSBHostUserClient logs. Do not start a firmware write.

## 6. Evidence and safe next steps

Use hidutil list and ioreg for read-only physical evidence. Use
/usr/bin/log show for recent HID and TCC messages when the environment permits.
Preserve the exact command and log path.

If access is denied, tell the user which exact path to add. If the path is a
temporary build, warn that the next rebuild may change the CDHash and require a
new permission entry. Prefer a stable, user-owned runtime for future tests.

Never call install, update, apply, flash, or a device reset in this skill.
