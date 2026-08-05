# dockwarden

`dockwarden` is a safety-first command-line tool for Dell docking stations on
macOS and Linux. The current target is the Dell Dock WD19.

> **Safety notice:** Firmware updates can leave a dock unusable if power or
> USB-C connectivity is lost. Review the read-only plan first. Keep the dock
> on stable power. Never run an unattended firmware update.

[![CI](https://github.com/fexxdev/dockwarden/actions/workflows/ci.yml/badge.svg)](https://github.com/fexxdev/dockwarden/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Contents**

- [Install](#install)
- [Does it work?](#does-it-work)
- [Recovery](#recovery)
- [Dell resources](#dell-resources)
- [What it does](#what-it-does)
- [Safety model](#safety-model)
- [Firmware source](#firmware-source)
- [Development](#development)
- [Documentation and credits](#documentation-and-credits)

## Install

The simplest installation uses the published release checksums and the bootstrap
installer. It detects macOS or Linux and the host architecture, downloads the
matching archive, verifies its SHA-256, and installs the binary for the current
user.

```sh
curl -fsSL https://github.com/fexxdev/dockwarden/releases/latest/download/install.sh | sh
```

The installer places `dockwarden` in `$HOME/.local/bin`. Add that directory to
`PATH` if it is not already present. On macOS, the installer also installs the
complete managed `fwupdtool` runtime and checks the required Homebrew formulae.
If Homebrew is not installed, the installer prints the exact prerequisite
command and stops. It never writes firmware.

To install a specific release, pin the tag:

```sh
curl -fsSL https://github.com/fexxdev/dockwarden/releases/latest/download/install.sh \
  | DOCKWARDEN_VERSION=v0.3.0 sh
```

To install an archive that you downloaded manually:

```sh
mkdir dockwarden-v0.3.0-darwin-arm64
tar -xzf dockwarden-v0.3.0-darwin-arm64.tar.gz \
  -C dockwarden-v0.3.0-darwin-arm64
cd dockwarden-v0.3.0-darwin-arm64
./install.sh
```

Release `v0.3.0` provides archives for macOS arm64, macOS amd64, Linux amd64,
and Linux arm64, plus the `SHA256SUMS` file. Each archive contains the binary,
installer, README, changelog, and license. The macOS archives also contain the
managed `fwupd-2.2.1` prefix. Keep that complete prefix. Do not copy only its
`fwupdtool` executable.

On macOS, the installer runs a read-only fwupd HID permission check. If macOS
denies access, it prints the exact path to the Input Monitoring setting. Open
System Settings > Privacy & Security > Input Monitoring. Enable the terminal
or app that runs `dockwarden`, then quit and reopen that terminal or app. The
same check is available with `dockwarden doctor`.

On Linux, the installer installs the `fwupd` package when `fwupdmgr` is
missing. It supports `apt-get`, `dnf`, `yum`, `pacman`, and `zypper`. The
installer can ask for `sudo`. Dockwarden itself never invokes `sudo`.

Run these commands first. They are read-only:

```sh
dockwarden --version
dockwarden status
dockwarden doctor
dockwarden update
```

`dockwarden update` shows the plan and does not download or write firmware.
Only `dockwarden update --apply` can start an update. Use it only with the
dock connected, stable power, an attended computer, and an explicit decision
to proceed.

## Does it work?

Yes. The verified WD19 development run updated the dock. The update command
itself returned an error during progress, so the final result was accepted only
after a physical reconnect and a fresh read-only status check.

The observed versions were:

| Component | Before | After | Result |
| --- | --- | --- | --- |
| Package | `01.00.47.01` | `01.01.01.01` | Updated |
| Embedded controller | `01.01.00.13` | `01.01.00.15` | Updated |
| USB hub Gen1 | `01.23` | `01.23` | Already current |
| USB hub Gen2 | `01.62` | `01.62` | Already current |
| MST | `05.07.08` | `05.07.08` | Already current |

The install used the official Dell CAB
`DellDockFirmwarePackage_WD19_WD22_HD22_WD25_SD25_01.01.11.cab`. Dockwarden
verified its SHA-256 before handing it to upstream `fwupdtool`.

The relevant output stopped at:

```text
Writing…: 70.5%
fwupdtool: exit status 1
```

The old logger retained only the first 4096 bytes, so the final low-level
error is not available. The output contains several device restart and wait
phases. This is consistent with a transient HID or USB re-enumeration loss,
but it does not prove the cause. fwupd also printed a package validation
warning. The log does not prove that warning caused the interruption.

After the recovery reconnect, status reported the new package and embedded
controller versions, no checks, and no warnings. This confirms the effective
hardware result even though the original process did not exit cleanly.

The local evidence is preserved in `dockwarden-flash-20260805-0005.txt` and
`dockwarden-recovery-status-20260805.txt`. New logs preserve both the start and
the end of command output. Dockwarden performs one install attempt only. It
does not retry an install after an error; it performs a read-only verification
instead.

## Recovery

Use this section after an interrupted or failed update. Keep the dock on its
AC adapter. Prevent the computer from sleeping. Do not start a second flash
until `dockwarden status` proves the result of the first attempt.

First save a complete log and run the read-only checks:

```sh
dockwarden --log-file "$HOME/dockwarden-recovery.txt" status
dockwarden --log-file "$HOME/dockwarden-recovery.txt" doctor
```

The following table covers every state that Dockwarden can verify:

| Observed state | Safe action | Do not do |
| --- | --- | --- |
| `up_to_date` after reconnect | Keep the dock connected. Save the log. The candidate is not newer than the verified dock firmware. | Do not flash again to test the result. |
| `update_failed` before any write | Keep the dock connected. Read the reason. Fix the listed preflight, permission, package, or link problem. Run `status` again. | Do not force the update or use a different package. |
| `version_check_unavailable` | Stop. Keep the dock powered. Save the report and restore a readable USB/HID connection before any apply. | Do not treat missing versions as an update. |
| `update_staged` or `update-pending` | Wait for the process to exit. Disconnect the USB-C cable once. Wait ten seconds. Reconnect it. Run `status`. | Do not run `update --apply` again before this check. |
| Error during progress, such as `Writing…: 70.5%` | If progress is active, wait for the process to exit. After it exits, reconnect the USB-C cable once and run `status`. A new package and controller version confirms the write. | Do not unplug a dock while the writer is still active. Do not repeat the flash from the error message alone. |
| `update_verified` | The candidate versions match the dock. Keep the log. No recovery action is needed. | Do not flash again. |
| Mixed component versions | Stop. Save the log and the pre/post version report. Do not force another package. Use Dell recovery support. | Do not downgrade or skip component checks. |
| No dock in `status` after reconnect | Remove downstream USB devices. Disconnect the host cable. Remove dock power for 30 seconds. Reconnect power first, then the host cable. Run `status` and `doctor`. | Do not run raw `fwupdtool install` commands. |
| Dock has no LEDs or never enumerates | Repeat the power-first cycle once with another USB-C port and cable. If the dock still does not enumerate, stop software attempts. Use Dell service or a Dell recovery utility on a supported Windows system, when Dell lists one for the model. | Do not open the dock or apply an unverified image. |
| Computer, cable, or power failed during the write | Restore stable power. Use the power-first cycle. Run `status`. Treat missing or mixed versions as an unresolved failure. | Do not assume that a completed progress bar means that every component is valid. |
| Same failure repeats | Keep all logs and stop. Report the model, serial, versions, state, and exact error to Dell support. | Do not retry without new evidence. |

If the dock enumerates and all candidate versions match, the firmware write was
effective even if the writer returned an error. If the dock reports a pending
state, reconnect once and verify again. If the dock does not enumerate, the
software cannot guarantee recovery. Dell service or the official Dell recovery
path is required.

Never use a raw `fwupdtool install` command, a forced downgrade, or a CAB from
an unknown source. Keep the recovery log because it can contain dock
identifiers.

## Dell resources

These are official Dell links for the WD19. Dell can redirect a page to your
region. The product page is the source of truth when a direct file link changes.

### Downloads

- [WD19 Drivers & Downloads](https://www.dell.com/support/product-details/en-us/product/dell-wd19-130w-dock/drivers)
- [WD19/WD22/HD22/WD25/SD25 firmware utility, driver 389W0](https://www.dell.com/support/home/en-us/drivers/driversdetails?driverid=389w0)
- [Direct official CAB download](https://dl.dell.com/FOLDER14009221M/1/DellDockFirmwarePackage_WD19_WD22_HD22_WD25_SD25_01.01.11.cab) — SHA-256: `f476fda34db1299da1c251bf04144d892a897a81fad0a40ee0c9771471f41614`
- [Direct official Linux BIN download](https://dl.dell.com/FOLDER14009249M/1/DellDockFirmwarePackage_WD19_WD22_HD22_WD25_SD25_01.01.11.bin) — SHA-256: `f7ab798d4df984e966e64cd6dc8caaaff40f46ad7fe3044f761629ace7a6199b`
- [Latest WD19/WD22TB4 Windows firmware utility, driver NKJG6 (01.01.14.01)](https://www.dell.com/support/home/en-us/drivers/driversdetails?driverid=nkjg6)
- [Previous WD19/WD22TB4 Windows firmware utility, driver XVXN7 (01.01.13.01)](https://www.dell.com/support/home/en-us/drivers/driversdetails?driverid=xvxn7)

The CAB and BIN are Dell files. Dockwarden uses the verified CAB through its
fwupd backend. Do not run either file directly or mix files from different
releases. The Windows utilities are reference and recovery links only;
Dockwarden never executes a Dell `.exe`. Check the checksum on the Dell driver
page before use.

### Recovery and troubleshooting

- [WD19 firmware update failure and power-cycle procedure (KB 000184585)](https://www.dell.com/support/kbdoc/en-us/000184585/ec-firmware-of-dell-dock-station-wd19x-updated-failed-on-generation-8-platforms-with-windows-10-20h1-or-higher)
- [WD19 BIOS settings and driver installation troubleshooting (KB 000193792)](https://www.dell.com/support/kbdoc/en-us/000193792/dell-wd19-series-dock-bios-settings-and-driver-installation-for-troubleshooting)
- [Dell Dock cannot use LAN or update firmware (KB 000289260)](https://www.dell.com/support/kbdoc/en-us/000289260/dell-dock-cannot-use-lan-port-or-update-dock-firmware)
- [Dock no-power troubleshooting guide (KB 000223850)](https://www.dell.com/support/kbdoc/en-us/000223850/dell-usb-c-and-thunderbolt-docks-no-power-troubleshooting-guide)
- [Power-cycle guidance after a dock firmware update (KB 000137390)](https://www.dell.com/support/kbdoc/en-us/000137390/dell-wired-docks-how-long-can-you-wait-after-updating-the-firmware-to-power-cycle-the-dock)
- [SupportAssist dock diagnostics (KB 000203763)](https://www.dell.com/support/kbdoc/en-us/000203763/how-to-use-supportassist-docking-station-diagnostics)
- [How to check a wired dock firmware version (KB 000129828)](https://www.dell.com/support/kbdoc/en-us/000129828/dell-usb-type-c-thunderbolt-docks-checking-the-current-firmware-version-on-your-wired-dock)
- [USB devices affected after dock firmware updates (KB 000224626)](https://www.dell.com/support/kbdoc/en-us/000224626/usb-devices-connected-to-dell-docks-may-experience-issues-after-updating-the-dock-firmware)
- [USB audio issues after dock firmware updates (KB 000224068)](https://www.dell.com/support/kbdoc/en-us/000224068/usb-audio-issues-on-dell-docks-after-updating-firmware)
- [Dell Docking Stations Support Library](https://www.dell.com/support/contents/en-us/category/product-support/self-support-knowledgebase/docking-stations)
- [All WD19 support articles](https://www.dell.com/support/product-details/en-us/product/dell-wd19-130w-dock/resources/articles)

Use Dell's power-cycle procedure when the dock has no power or does not
enumerate. Use Dockwarden's recovery table first. It records the result before
you consider another update.

### Manuals and technical documentation

- [WD19 User Guide](https://www.dell.com/support/manuals/en-us/dell-wd19-130w-dock/wd19_userguide/)
- [WD19 firmware update chapter in the User Guide](https://www.dell.com/support/manuals/en-us/dell-wd19-130w-dock/wd19_userguide/dell-docking-station-firmware-update?guid=guid-a9c2c1cc-80be-4176-bba7-6c574ec91d88)
- [WD19 Manuals & Documents index](https://www.dell.com/support/product-details/en-us/product/dell-wd19-130w-dock/resources/manuals)
- [WD19 Administrator's Guide (PDF)](https://downloads.dell.com/topicspdf/dell-wd19-130w-dock_Administrator-Guide_en-us.pdf)
- [WD19 Quick Start Guide (PDF)](https://dl.dell.com/manuals/all-products/esuprt_electronics/esuprt_docking_stations/dell-wd19-130w-dock_setup-guide_en-us.pdf)
- [Guide to Dell docking stations (KB 000124295)](https://www.dell.com/support/kbdoc/en-us/000124295/guide-to-dell-docking-stations)
- [USB and USB-C dock compatibility list (KB 000125885)](https://www.dell.com/support/kbdoc/en-us/000125885/usb-and-usb-type-c-dock-compatibility-list)
- [Dell Commercial Docking Compatibility Guide (PDF)](https://www.delltechnologies.com/asset/en-in/products/electronics-and-accessories/technical-support/dell_docking_compatibility_guide.pdf)
- [SupportAssist for Home PCs](https://www.dell.com/support/contents/en-us/article/product-support/self-support-knowledgebase/software-and-downloads/support-assist/SupportAssist-for-Home)
- [Dell Support home and contact options](https://www.dell.com/support/home/en-us)

The Administrator's Guide documents the Dell update states, component version
checks, logs, error handling, and post-update reconnect. It is the best Dell
reference for service and recovery work.

## What it does

Dockwarden can:

- identify the WD19 USB device and its topology;
- report USB, Ethernet, audio, and downstream USB services;
- read component firmware versions through the upstream fwupd Dell plugin on
  macOS;
- check official Dell firmware metadata and the pinned CAB checksum;
- create a read-only update plan;
- update the WD19 through `fwupdmgr` on Linux;
- update the WD19 through the upstream standalone `fwupdtool` on macOS.

The macOS inventory and writer use the upstream Dell Dock plugin from the
fwupd Darwin branch. That plugin owns the Apple HID transport and the Dell
protocol. Dockwarden selects one exact WD19 DeviceId from JSON, compares the
USB serial, and verifies every candidate component after the install.

Dockwarden does not execute Dell Windows `.exe` packages. It does not accept
arbitrary payloads, forced downgrades, or unverified firmware files. A USB
descriptor version is not a firmware version.

## Safety model

Before an install, macOS checks the dock identity, one fwupd target, update
state, and all five component versions from a read-only JSON inventory. It
selects a single `dell_dock` DeviceId by plugin, embedded-controller instance
ID, and the USB serial prefix. The upstream Dell plugin owns the hardware
checks and write protocol. Any failed check stops the install.

The macOS permission probe is also fail-closed. A missing HID/Input Monitoring
permission can report status, but it blocks `update --apply` until you enable
the permission and restart the terminal or app.

An `update_staged` result means that the platform accepted the update and
requires a physical reconnect. Reconnect the dock USB-C cable, then run
`dockwarden status`. An `update_verified` result means that fwupd returned
success, or returned an error after the dock reported every candidate version.
If verification is not possible, the result remains `update_failed` or
`update_staged`.

Every command writes user-only text logs to `dockwarden-log.txt`. Use
`--log-file PATH` to select another file. Update logs include the preflight,
selected DeviceId, fwupd commands, command output head and tail, post-install
version verification, and the final state. Protect logs because they can
contain dock identifiers.

Exit codes are:

- `0`: the command succeeded;
- `1`: no supported dock was detected;
- `2`: the command or an explicit firmware apply failed.

## Firmware source

The updater uses Dell driver `389W0`, which publishes the Linux CAB for the
WD19 family. If Dell blocks its metadata page with HTTP 403, Dockwarden uses
browser-compatible request headers and a pinned official CAB. It verifies the
CAB SHA-256 before any firmware operation.

The candidate records package, embedded-controller, USB hub Gen1, USB hub
Gen2, and MST versions. A live Dell candidate inherits those values only when
its SHA-256 matches the pinned CAB. Missing or conflicting evidence gives
`version_check_unavailable`; apply mode exits with code `2` and does not write.

The Windows `.exe` is never executed. The project does not bundle Dell
firmware blobs.

## Development

Build a development binary with Go 1.22 or newer:

```sh
GOCACHE=/tmp/dockwarden-go-cache go test ./...
GOCACHE=/tmp/dockwarden-go-cache go vet ./...
CGO_ENABLED=1 GOCACHE=/tmp/dockwarden-go-cache go build ./cmd/dockwarden
```

Run the release packaging checks without hardware:

```sh
sh -n tools/release/*.sh
sh tools/release/install_test.sh
sh tools/release/package_test.sh
```

The macOS fwupd port is built from the pinned Darwin branch commit. See
[the macOS build guide](tools/fwupd-macos/README.md). The build runs upstream
non-hardware tests and does not update a dock.

Tests use fake fwupd, HTTP, and command interfaces. They never flash a
physical dock. See [CONTRIBUTING.md](CONTRIBUTING.md) for the development
workflow.

## Documentation and credits

- [Adding support for another docking station](docs/ADDING-DOCK-SUPPORT.md)
- [Building the macOS fwupdtool port](tools/fwupd-macos/README.md)
- [Change log](CHANGELOG.md)
- [Credits and external projects](CREDITS.md)
- [Security policy](SECURITY.md)
- [MIT License](LICENSE)
