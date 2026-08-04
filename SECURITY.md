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

## Reporting

Please report security issues privately through GitHub. Do not disclose an
unfixed vulnerability in a public issue. Include the commit, platform, command
used, and a minimal reproduction when possible.
