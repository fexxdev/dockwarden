# Adding support for another docking station

This guide explains how to add a docking station to `dockwarden`.

The project currently has compiled-in support for the Dell Dock WD19. It does
not have a runtime plugin system. A new model is added to the Go code, its
vendor firmware source is validated, and its behavior is covered by fixtures
and tests.

## Support levels

Add only the level that the evidence supports:

1. **Detection**: identify the model and USB identifiers.
2. **Inspection**: report topology and observable services.
3. **Firmware read**: report component versions from `fwupdmgr` or a native
   protocol.
4. **Update planning**: compare the installed versions with a verified vendor
   package.
5. **Firmware update**: write, verify, and activate supported components.

Level 5 is optional. A dock can have useful support at levels 1–4 while its
write protocol is still unknown.

## Extension map

| Concern | Current extension point | Change it when |
| --- | --- | --- |
| Shared output model | `internal/domain/domain.go` | The new dock needs data that is useful for every backend. Keep fields generic. |
| Model matching | `internal/discovery/match.go` | Add the model name, vendor ID, product ID, and safe fallback names. |
| USB topology | `internal/discovery/report.go` | The model uses names or topology that the generic classifier cannot classify. |
| macOS USB parsing | `internal/discovery/ioreg.go` | `ioreg` exposes a property that the parser does not retain. |
| Linux USB parsing | `internal/discovery/lsusb.go` | `lsusb` output for the model needs a parser change. |
| Linux firmware read | `internal/discovery/fwupd.go` | `fwupdmgr` reports a component format that the parser does not read. |
| Vendor metadata | `internal/dell/catalog.go` or a new vendor package | The vendor page, package format, or URL rules differ. |
| Linux update | `internal/update/fwupd.go` | The model is supported by fwupd and needs a broader guard or a new adapter. |
| macOS transport | `internal/macos/hid/` | The model uses macOS HID and needs an OS-specific transport. |
| macOS protocol | `internal/update/` | The model uses a native HID, USB, I2C, serial, or vendor protocol. |
| CLI wiring | `cmd/dockwarden/main.go` | Add the new catalog, backend, or model-to-backend routing. |
| Orchestration | `internal/app/app.go` | Only when the generic command flow cannot represent the new backend. |

`internal/app` already separates inspection, metadata checks, and firmware
application through these interfaces:

```go
type Inspector interface {
    Inspect(context.Context, string) (domain.Report, error)
}

type UpdateChecker interface {
    Check(context.Context, *domain.Dock) domain.UpdateCheck
}

type FirmwareUpdater interface {
    Apply(context.Context, *domain.Dock, *domain.FirmwareCandidate) domain.UpdateCheck
}
```

Keep model-specific protocol code outside `internal/app`. The application
layer should decide when to inspect or apply. The backend should decide how to
communicate with the dock.

## Step 1: collect evidence before coding

Record the following for the dock. Do not guess a model from a product name.

On macOS:

```sh
ioreg -p IOUSB -l -w 0
```

On Linux:

```sh
lsusb
fwupdmgr get-devices
fwupdmgr get-updates
```

Also collect:

- the exact marketing model and hardware revision;
- USB vendor ID and product ID for the root dock device;
- USB product and vendor strings;
- serial and service-tag behavior;
- downstream USB, Ethernet, audio, and display topology;
- current firmware versions for every component;
- the official vendor package URL, format, version, release date, and SHA-256;
- the supported operating systems and the vendor update method;
- any official guide or existing fwupd plugin for the model.

Use read-only commands first. Do not run a vendor updater during discovery.
Do not commit serial numbers, service tags, captures with personal data, or
firmware binaries.

## Step 2: add a discovery fixture

Save a minimal redacted capture under `internal/discovery/testdata/`:

```text
<model>-ioreg.txt
<model>-lsusb.txt
```

Use the fixture to reproduce the real device tree. Replace serial numbers and
service tags with stable placeholders. Keep the USB identifiers and topology.

Add parser tests when the fixture exposes a new property. Use the existing
tests as the pattern:

- `internal/discovery/ioreg_test.go`
- `internal/discovery/lsusb_test.go`
- `internal/discovery/platform_test.go`

The parser must return `domain.USBDevice` values. It must not know how to flash
firmware.

## Step 3: register the model matcher

Add constants and a matcher in `internal/discovery/match.go`.

The preferred match is the exact vendor/product pair. Product or device names
are fallback evidence only, because names can change between operating systems.

The matcher must:

- detect the intended device;
- reject a different product from the same vendor;
- return a stable `domain.Dock.Model` value;
- preserve the USB IDs, serial, and descriptor version;
- return `unknown_dell_device` or `no_dock` for unrelated devices.

Add positive and negative tests in `internal/discovery/match_test.go`.

If the new model is not Dell, do not force it through the Dell matcher. Add a
vendor-neutral matcher path or a separate matcher that preserves the same
report contract.

## Step 4: validate topology and service checks

Run the new fixture through `BuildReport` and inspect:

- the dock root;
- internal dock components;
- downstream USB devices;
- Ethernet;
- audio;
- the service states in `domain.ServiceObservation`.

Only change `internal/discovery/report.go` if the generic rules fail. Prefer a
small model-specific predicate over a broad name match. Add a fixture test for
each classification change.

Do not treat `bcdDevice` or `DescriptorVersion` as firmware. A USB descriptor
version and a component firmware version are different values.

## Step 5: add the firmware catalog

The catalog must return a `domain.FirmwareCandidate` with:

- an official HTTPS source page;
- an official HTTPS download URL;
- a package name and format;
- a version or component version list;
- a release date;
- a 64-character SHA-256;
- compatible model names;
- supported operating systems.

For another Dell package, extend `internal/dell/catalog.go` only when its page
uses the same Dell rules. Add parser fixtures and tests in
`internal/dell/catalog_test.go`.

For another vendor, create a separate catalog package. Reuse
`domain.FirmwareCandidate`, but keep vendor-specific HTML, API, and URL rules
in that package. Wire it in `cmd/dockwarden/main.go`.

Never accept arbitrary user-provided firmware URLs. Keep these checks:

- HTTPS only;
- expected vendor host;
- no embedded URL credentials;
- bounded response size;
- SHA-256 verification before extraction or installation;
- model compatibility verification.

If the vendor page is unavailable, a pinned fallback is acceptable only when
the package URL, version, compatibility, and SHA-256 are checked into source.
Document the source and date in `CREDITS.md` or the catalog comments.

Do not bundle vendor firmware files in this repository.

## Step 6: choose the update backend

### fwupd on Linux

Use this path when fwupd already supports the dock. Extend the guard in
`internal/update/fwupd.go` only after confirming that the package is valid for
the new model. Add tests that prove:

- the wrong model is rejected before download;
- the package hash is checked before `fwupdmgr` runs;
- non-vendor URLs are rejected;
- fwupd errors are reported;
- the command uses the expected arguments.

If the fwupd plugin supports multiple model IDs but the current guard is too
specific, replace the single-model predicate with a small allowlist. Keep the
allowlist explicit.

### Native macOS update

Use a new backend when the vendor has no usable macOS updater and the protocol
is understood. Do not add a second vendor protocol to `internal/update/macos.go`.
That file contains the WD19 protocol and its component layout.

Create a separate protocol file, for example:

```text
internal/update/acme_hid.go
internal/update/acme_hid_test.go
internal/update/acme_macos.go
internal/update/acme_macos_test.go
```

Keep the protocol layer pure Go when possible. It should translate typed
operations into HID or USB reports and validate response sizes. Test packet
bytes with fake report devices.

Keep Apple API calls in `internal/macos/hid/`. The existing transport exposes:

```go
type HIDConnection interface {
    HIDReports
    Close()
}
```

If the new dock uses another transport, add a small OS adapter instead of
putting cgo calls into the protocol or application layer.

## Step 7: implement read-only firmware inspection first

Before writing anything, implement the ability to read the dock state and
component versions. Validate:

- identity and protocol family;
- bootloader or update status;
- component list and version encoding;
- package layout and component blob sizes;
- version offsets and endianness;
- current versus candidate version comparison.

Return an error when a version is missing or malformed. Do not silently treat a
missing version as zero.

The current WD19 updater reads its state through `DellHID.ReadDockData`,
`DellHID.ReadDockInfo`, and `DellHID.ReadUpdateStatus`. A different dock should
have equivalent methods in its own protocol type.

## Step 8: build the update plan before the first write

The backend must complete all checks before it erases or writes a component:

1. validate the detected model;
2. validate the candidate URL, format, compatibility, and hash;
3. download and verify the package;
4. extract every required component;
5. validate every component size and version;
6. compare all installed and candidate versions;
7. reject unsupported newer components;
8. only then start the write sequence.

This ordering prevents a newer unsupported component from causing a partial
update. The WD19 backend uses this rule for its unsupported MST update.

Do not implement forced downgrades unless the protocol and user contract
explicitly require them. Do not flash a component when its current version is
unknown.

## Step 9: implement write, verify, and activation

For each component, document and test:

- unlock or maintenance mode;
- clock or bootloader preparation;
- erase command and bank selection;
- write address and maximum chunk size;
- verify command and response;
- lock or cleanup behavior;
- reboot or passive activation flow;
- the state after a failed step.

Use `context.Context` inside long write loops. Close every transport. Restore
temporary device state when possible. Return `update_failed` when a write or
activation step fails.

The command must never claim success before verification and activation finish.
If activation fails after a write, report that the components were written and
that activation failed. This is different from a clean preflight failure.

## Step 10: wire the backend

The current `main.go` creates one inspector, one catalog, and one updater. For a
second model, choose the smallest safe wiring change:

- same vendor and same fwupd path: extend the explicit model allowlist;
- same vendor and different native protocol: add a model-aware updater router;
- different vendor: add a model-aware catalog and updater router;
- inspection only: add discovery support and leave firmware update unsupported.

The router should reject an unknown model. It must not select a backend from a
user-provided URL or from a loose product-name substring.

If more models are added, introduce a compiled registry with explicit match
functions and backend ownership. Keep `internal/app` dependent on interfaces,
not on vendor packages.

## Step 11: add the test matrix

Every new model should add tests for:

- exact USB identity detection;
- same-vendor wrong-product rejection;
- macOS fixture parsing;
- Linux fixture parsing;
- topology and service classification;
- firmware metadata parsing;
- invalid URL and hash rejection;
- no-op when the dock is current;
- unsupported component rejection before the first write;
- component writes in the correct order;
- chunk boundaries and byte-level protocol packets;
- verification failure;
- activation failure;
- cancellation through `context.Context`.

Use fake command runners, HTTP clients, firmware extractors, and HID devices.
Tests must not need a physical dock and must not download firmware.

Run the full checks:

```sh
GOCACHE=/tmp/dockwarden-go-cache go test -count=1 ./...
GOCACHE=/tmp/dockwarden-go-cache go vet ./...
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build ./cmd/dockwarden
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/dockwarden
```

## Step 12: update project documentation

Before opening a pull request:

- update the supported-model list in `README.md`;
- state the support level and platform limits;
- add the vendor source and external project credits to `CREDITS.md`;
- add a user-visible entry to `CHANGELOG.md`;
- document safety limits and recovery steps;
- avoid committing firmware blobs or proprietary tools;
- review the JSON output for accidental schema changes.

## Definition of done

A model is ready for release when:

- it has a stable exact identity match;
- macOS and Linux behavior is covered by redacted fixtures;
- supported services are reported correctly;
- firmware metadata is official and hash-verified;
- unsupported update paths fail before any write;
- all write operations have fake-device tests;
- a physical update was tested only with a recovery plan and stable power;
- README, changelog, credits, and security notes are current.

When the protocol is not known well enough to meet the last points, ship
detection and inspection support only. A safe partial feature is better than an
unverified firmware writer.
