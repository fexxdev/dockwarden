# Security Policy

## Scope

Security reports are welcome for the CLI, firmware download validation, HID
transport, and update safety checks.

## Firmware safety

`dockwarden` supports only the detected Dell WD19 identity. It accepts official
Dell HTTPS downloads and verifies the published SHA-256 before writing. It does
not execute Dell Windows packages or accept arbitrary local firmware files.

The macOS production writer delegates to the pinned upstream Dell Dock plugin
in standalone `fwupdtool`. Before network access, Dockwarden verifies the
managed runtime file set, file modes, hashes, and fwupd 2.2.1 compile/runtime
versions. It gives fwupd a minimal environment. The upstream Dell plugin
performs the HID and Dell protocol work. Dockwarden reads a JSON inventory and
performs a read-only target and version preflight before the writer can run.
Review the plan with `dockwarden update`. Use `dockwarden update --apply` only
after that review.

Dockwarden selects one `dell_dock` device by its WD19 embedded-controller
instance ID and USB serial prefix. It passes the full DeviceId to fwupdtool.
Missing, multiple, malformed, or changed targets stop the update. The upstream
plugin then owns the HID selection and hardware validation in the fwupd
process.

An `update_staged` result is not proof that the final firmware is active.
Unplug and reconnect the dock, then run `dockwarden status`. `update_verified`
means that the reported component versions match the candidate. Dockwarden
never retries a failed install. Do not run an apply while the dock has unstable
power, an unstable USB-C link, or a newer MST payload.

Each command writes an append-only text log with mode `0600`. The log records
the selected device, update phases, fwupd output, and failures. Treat it as
diagnostic data because it can contain dock identifiers.

The repository test suite, static checks, and build checks never apply a
firmware update to physical hardware. The macOS CI build runs upstream tests,
validates the runtime manifest, and runs read-only version and enumeration
commands. A first hardware run still needs a recovery plan and direct
observation of the dock.

## Reporting

Please report security issues privately through GitHub. Do not disclose an
unfixed vulnerability in a public issue. Include the commit, platform, command
used, and a minimal reproduction when possible.
