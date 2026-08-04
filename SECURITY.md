# Security Policy

## Scope

Security reports are welcome for the CLI, firmware download validation, HID
transport, and update safety checks.

## Firmware safety

`dockwarden` supports only the detected Dell WD19 identity. It accepts official
Dell HTTPS downloads and verifies the published SHA-256 before writing. It does
not execute Dell Windows packages or accept arbitrary local firmware files.

The native macOS writer is hardware-facing code. Review the plan with
`dockwarden update` before using `dockwarden update --apply`.

An `update_staged` result is not proof that the final firmware is active.
Unplug and reconnect the dock, then run `dockwarden status`. Do not run an
apply while the dock has unstable power, an unstable USB-C link, or a newer
MST payload.

The repository test suite, static checks, and build checks never apply a
firmware update to physical hardware. A first hardware run still needs a
recovery plan and direct observation of the dock.

## Reporting

Please report security issues privately through GitHub. Do not disclose an
unfixed vulnerability in a public issue. Include the commit, platform, command
used, and a minimal reproduction when possible.
