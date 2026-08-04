# Dockwarden Read-Only MVP Implementation Plan (Historical)

> Status: this plan describes the original read-only foundation. It is
> superseded by the guarded firmware update path. Its historical constraints
> do not describe the current `update --apply` behavior.

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

Goal: Build a Go CLI that identifies the connected Dell WD19 dock, reports host-observable state, and checks official Dell firmware metadata without writing to the dock.

Architecture: A platform-independent command layer consumes a discovery interface and a Dell catalog interface. macOS discovery parses ioreg output; Linux discovery parses lsusb output and reports optional fwupdmgr availability. Shared domain types drive text and JSON output. All first-version operations are read-only.

Tech Stack: Go 1.22+, standard library only, go test, go vet, ioreg, lsusb, and optional fwupdmgr.

## Global Constraints

- The first version must inspect the connected dock without changing it.
- The first version will not flash firmware.
- USB descriptor versions such as bcdDevice must never be labelled as dock firmware versions.
- All initial commands are read-only and need no administrator privileges.
- The CLI must return human-readable output and machine-readable JSON.
- The CLI must not distribute Dell firmware blobs.
- The live Mac WD19 is an integration fixture; no firmware write is allowed.
- Use the Go standard library only for the first version.
- Keep the repository on its existing main branch, as explicitly requested by the user.

---

### Task 1: Bootstrap the Go module and domain contract

Files:

- Create: go.mod
- Create: internal/domain/domain.go
- Create: internal/domain/domain_test.go
- Create: internal/cli/args.go
- Create: internal/cli/args_test.go
- Create: cmd/dockwarden/main.go

Interfaces:

- Produces domain.Report, domain.Dock, domain.USBDevice, domain.ServiceObservation, domain.FirmwareObservation, domain.FirmwareCandidate, and domain.UpdateCheck.
- Produces cli.Options and cli.Parse(args []string) (cli.Options, error).

- [ ] Step 1: Create the module file

Create go.mod with module path github.com/fexxdev/dockwarden and Go version 1.22.

- [ ] Step 2: Write domain contract tests

Test that a WD19 report marshals to JSON with stable field names and that an
unknown firmware version remains empty instead of using the USB descriptor
version.

~~~go
func TestReportJSONKeepsDescriptorSeparateFromFirmware(t *testing.T) {
    got, err := json.Marshal(domain.Report{
        Platform: "darwin",
        Dock: &domain.Dock{
            Model:             "Dell Dock WD19",
            VendorID:          0x413c,
            ProductID:         0xb06e,
            DescriptorVersion: "2.0",
            FirmwareVersion:   "",
        },
    })
    if err != nil {
        t.Fatal(err)
    }
    text := string(got)
    if !strings.Contains(text, "\"descriptor_version\":\"2.0\"") {
        t.Fatalf("descriptor version missing: %s", text)
    }
    if !strings.Contains(text, "\"firmware_version\":\"\"") {
        t.Fatalf("firmware version missing: %s", text)
    }
}
~~~

- [ ] Step 3: Run the domain test and confirm the intended failure

Run go test ./internal/domain -run TestReportJSONKeepsDescriptorSeparateFromFirmware -v.
Expected: FAIL because the domain types do not exist yet.

- [ ] Step 4: Write the minimal domain types

Define JSON-tagged types with these fields:

~~~go
type Report struct {
    SchemaVersion int
    Platform      string
    Command       string
    State         string
    Dock          *Dock
    Checks        []Check
    Update        *UpdateCheck
    Warnings      []string
}

type Dock struct {
    Manufacturer      string
    Model             string
    VendorID          uint16
    ProductID         uint16
    Serial            string
    DescriptorVersion string
    FirmwareVersion   string
    FirmwareSource    string
    Devices           []USBDevice
    Services          []ServiceObservation
}
~~~

Add USBDevice, ServiceObservation, Check, FirmwareCandidate, and UpdateCheck
with explicit string state fields. Add FirmwareObservation with component,
version, source, and confidence fields for optional fwupd inventory. Use uint16
for USB IDs and keep firmware version empty when it is not read from a
firmware-aware source.

- [ ] Step 5: Run the domain test and confirm it passes

Run go test ./internal/domain -run TestReportJSONKeepsDescriptorSeparateFromFirmware -v.
Expected: PASS.

- [ ] Step 6: Write CLI argument tests

Test --json scan, status, doctor, check-updates, --verbose, --version, help,
and an unknown command. The parser must accept global options before the
command and reject missing or unknown commands.

- [ ] Step 7: Run the CLI argument tests and confirm failure

Run go test ./internal/cli -v.
Expected: FAIL because cli.Parse does not exist.

- [ ] Step 8: Implement the minimal argument parser and entry point

Implement cli.Options and cli.Parse. Add cmd/dockwarden/main.go with a version
constant and a clear command-dispatch hook for Task 4.

- [ ] Step 9: Run all bootstrap tests

Run go test ./internal/domain ./internal/cli -v.
Expected: PASS.

- [ ] Step 10: Commit the bootstrap

~~~bash
git add go.mod internal/domain internal/cli cmd/dockwarden
git commit -m "feat: bootstrap dockwarden domain and CLI"
~~~

### Task 2: Parse macOS IORegistry data and identify the WD19

Files:

- Create: internal/discovery/runner.go
- Create: internal/discovery/ioreg.go
- Create: internal/discovery/ioreg_test.go
- Create: internal/discovery/match.go
- Create: internal/discovery/match_test.go
- Create: internal/discovery/report.go
- Create: internal/discovery/report_test.go
- Create: internal/discovery/testdata/wd19-ioreg.txt

Interfaces:

- type CommandRunner interface { Run(context.Context, string, ...string) ([]byte, error) }.
- func ParseIORegistry(string) ([]domain.USBDevice, error).
- func Identify(devices []domain.USBDevice) (*domain.Dock, string).
- func BuildReport(platform, command string, devices []domain.USBDevice) domain.Report.

- [ ] Step 1: Save a minimal real IORegistry fixture

Create a fixture with the WD19 root, its Dell child nodes, the Realtek LAN
node, the audio node, one downstream peripheral, and representative numeric
properties. Keep the fixture free of user names and unrelated devices.

- [ ] Step 2: Write the failing IORegistry parser test

Assert that the fixture produces a WD19 root with vendor 0x413c, product
0xb06e, serial 2000, descriptor version 2.0, and at least one Realtek LAN
child. Assert that the parser preserves child depth.

- [ ] Step 3: Run the parser test and confirm failure

Run go test ./internal/discovery -run TestParseIORegistry -v.
Expected: FAIL because ParseIORegistry does not exist.

- [ ] Step 4: Implement the line parser

Parse +-o node lines and the quoted IORegistry properties. Decode decimal and
hexadecimal numeric values. Keep only USB device nodes, ignore the root
registry entry, and never interpret bcdDevice as firmware.

- [ ] Step 5: Run the parser test and confirm it passes

Run go test ./internal/discovery -run TestParseIORegistry -v.
Expected: PASS.

- [ ] Step 6: Write the failing WD19 matcher test

Test that VID 0x413c/PID 0xb06e and product name Dell Dock WD19 produce model
Dell Dock WD19. Test that a Dell device with an unknown product ID returns
state unknown_dell_device, and an empty list returns no_dock.

- [ ] Step 7: Run the matcher test and confirm failure

Run go test ./internal/discovery -run TestIdentify -v.
Expected: FAIL because Identify does not exist.

- [ ] Step 8: Implement model matching and report construction

Match the WD19 root by both stable USB identifiers and product name. Build
service observations from enumerated devices:

- usb: pass when a WD19 root is present;
- ethernet: pass when a Realtek or LAN product is enumerated;
- audio: pass when an audio product is enumerated;
- downstream_usb: pass when child devices exist.

Use firmware_unavailable when no firmware-aware source supplied a version. Set
state to detected, unknown_dell_device, or no_dock.

- [ ] Step 9: Run discovery tests and confirm they pass

Run go test ./internal/discovery -v.
Expected: PASS.

- [ ] Step 10: Commit macOS discovery

~~~bash
git add internal/discovery
git commit -m "feat: detect Dell WD19 from macOS IORegistry"
~~~

### Task 3: Add Linux discovery and optional fwupd inventory

Files:

- Create: internal/discovery/lsusb.go
- Create: internal/discovery/lsusb_test.go
- Create: internal/discovery/linux.go
- Create: internal/discovery/linux_test.go
- Create: internal/discovery/fwupd.go
- Create: internal/discovery/fwupd_test.go
- Modify: internal/discovery/runner.go

Interfaces:

- func ParseLsusb(string) ([]domain.USBDevice, error).
- type LinuxCollector struct { Runner CommandRunner }.
- func (LinuxCollector) Collect(context.Context) ([]domain.USBDevice, []string, error).
- func ParseFwupdDevices(string) []domain.FirmwareObservation.

- [ ] Step 1: Write the failing lsusb parser test

Use fixture lines for the WD19 root, a Dell hub, and a Realtek LAN device.
Assert decimal and hexadecimal IDs map to the same uint16 values and that the
parser preserves product text.

- [ ] Step 2: Run the Linux parser test and confirm failure

Run go test ./internal/discovery -run TestParseLsusb -v.
Expected: FAIL because ParseLsusb does not exist.

- [ ] Step 3: Implement lsusb parsing

Parse lines matching ID vvvv:pppp, manufacturer text, and product text. Assign
a flat depth of zero because plain lsusb has no topology. Use lsusb -v only when
a later adapter needs descriptor details.

- [ ] Step 4: Run the Linux parser test and confirm it passes

Run go test ./internal/discovery -run TestParseLsusb -v.
Expected: PASS.

- [ ] Step 5: Write the failing Linux collector test

Inject a runner that returns fixture output for lsusb and an error for
fwupdmgr. Assert that the collector returns devices and a warning saying
fwupdmgr unavailable, without failing the entire discovery.

- [ ] Step 6: Implement the Linux collector

Run lsusb through the injected runner. Probe fwupdmgr get-devices only if the
command exists. Parse usable version lines when present and expose a warning
when the optional command is absent or fails.

- [ ] Step 7: Run Linux collector tests

Run go test ./internal/discovery -run 'Test(ParseLsusb|LinuxCollector|ParseFwupd)' -v.
Expected: PASS.

- [ ] Step 8: Commit Linux discovery

~~~bash
git add internal/discovery
git commit -m "feat: add Linux dock discovery"
~~~

### Task 4: Connect commands and render text or JSON

Files:

- Create: internal/app/app.go
- Create: internal/app/app_test.go
- Create: internal/output/text.go
- Create: internal/output/text_test.go
- Create: internal/output/json.go
- Create: internal/output/json_test.go
- Modify: cmd/dockwarden/main.go

Interfaces:

- type Inspector interface { Inspect(context.Context, string) (domain.Report, error) }.
- type UpdateChecker interface { Check(context.Context, *domain.Dock) domain.UpdateCheck }.
- type Dependencies struct { Inspector Inspector; Updates UpdateChecker; Out, Err io.Writer }.
- func Run(context.Context, cli.Options, Dependencies) int.
- func RenderText(io.Writer, domain.Report, bool) error.
- func RenderJSON(io.Writer, domain.Report) error.

- [ ] Step 1: Write failing command tests

Use a real fake inspector with deterministic reports. Test that scan, status,
and doctor return exit code zero for a detected WD19. Test that a no-dock report
returns exit code one. Test that check-updates calls the update checker only
when a dock is detected.

- [ ] Step 2: Run command tests and confirm failure

Run go test ./internal/app -v.
Expected: FAIL because app.Run does not exist.

- [ ] Step 3: Implement command dispatch

Call the inspector once per command. Set report.Command. For doctor, add
checks for model identity, USB enumeration, Ethernet, audio, downstream USB,
and firmware availability. For missing optional functionality, emit a warning
instead of marking the whole dock unhealthy.

- [ ] Step 4: Write failing renderer tests

Assert that text output includes model, USB ID, serial, descriptor version,
firmware state, component names, service states, and warnings. Assert that
JSON output can be decoded into domain.Report and keeps stable keys.

- [ ] Step 5: Run renderer tests and confirm failure

Run go test ./internal/output -v.
Expected: FAIL because the render functions do not exist.

- [ ] Step 6: Implement text and JSON renderers

Use encoding/json.Encoder with indentation for JSON. In text mode, label
descriptor versions and firmware versions separately and print an explicit
line that functionality checks are host-observable only.

- [ ] Step 7: Run app and renderer tests

Run go test ./internal/app ./internal/output -v.
Expected: PASS.

- [ ] Step 8: Connect main.go to real platform discovery

Select the macOS collector on runtime.GOOS == darwin, the Linux collector on
runtime.GOOS == linux, and return a clear unsupported-platform error otherwise.
Use the real command runner and keep all commands read-only.

- [ ] Step 9: Run all tests and build the binary

Run go test ./..., go vet ./..., and go build ./cmd/dockwarden.
Expected: PASS with a dockwarden binary.

- [ ] Step 10: Commit the CLI

~~~bash
git add cmd internal
git commit -m "feat: add dockwarden scan status and diagnostics"
~~~

### Task 5: Check official Dell firmware metadata

Files:

- Create: internal/dell/catalog.go
- Create: internal/dell/catalog_test.go
- Create: internal/dell/testdata/wd19-driver-page.html
- Modify: internal/app/app.go
- Modify: internal/discovery/report.go

Interfaces:

- type HTTPDoer interface { Do(*http.Request) (*http.Response, error) }.
- type CatalogClient struct { HTTP HTTPDoer; Sources map[string]string }.
- func (c CatalogClient) Check(context.Context, *domain.Dock) domain.UpdateCheck.
- func ParseDriverPage(sourceURL string, []byte) (domain.FirmwareCandidate, error).

- [ ] Step 1: Write the failing Dell page parser test

Create a fixture with the official page fields Version, Release Date, File Name,
SHA-256, supported operating systems, and compatible systems. Assert that the
parser extracts the package name, version, date, hash, and WD19 compatibility.
Assert that a missing hash returns an error.

- [ ] Step 2: Run the parser test and confirm failure

Run go test ./internal/dell -run TestParseDriverPage -v.
Expected: FAIL because ParseDriverPage does not exist.

- [ ] Step 3: Implement conservative page parsing

Strip HTML tags, decode entities, normalize whitespace, and extract only fields
with strict patterns. Accept a candidate only when the source uses HTTPS on
dell.com, the package name contains a Dell dock firmware name, the version is
present, and a 64-character SHA-256 is present. Never infer a firmware version
from USB descriptors.

- [ ] Step 4: Run the parser test and confirm it passes

Run go test ./internal/dell -run TestParseDriverPage -v.
Expected: PASS.

- [ ] Step 5: Write the failing catalog client test

Inject an HTTP client that serves the fixture. Configure the WD19 source as
https://www.dell.com/support/home/en-us/drivers/driversdetails?driverid=389w0.
Assert that CatalogClient.Check returns update_available with the source URL
and candidate metadata. Assert that a timeout returns
vendor_metadata_unavailable and no candidate.

- [ ] Step 6: Implement the catalog client

Use an HTTP timeout of 15 seconds and a bounded response body of 4 MiB. Send a
clear user agent. Map the WD19 model to the official Dell source. Return an
unavailable state for network errors, non-2xx responses, parser errors, or
missing security fields.

- [ ] Step 7: Run catalog tests

Run go test ./internal/dell -v.
Expected: PASS.

- [ ] Step 8: Connect check-updates

Run the catalog check only for the check-updates command and only after a WD19
has been detected. Do not download or execute a firmware payload.

- [ ] Step 9: Run all tests and verify the update path is read-only

Run go test ./..., go vet ./..., and inspect the code for calls to os.WriteFile,
exec.Command with sudo, or firmware installation commands. Expected: no
firmware write path exists.

- [ ] Step 10: Commit update checking

~~~bash
git add internal/dell internal/app internal/discovery
git commit -m "feat: check official Dell dock firmware metadata"
~~~

### Task 6: Document and verify against the live WD19

Files:

- Create: README.md
- Create: internal/discovery/testdata/README.md
- Modify: docs/superpowers/specs/2026-08-04-dockwarden-design.md

- [ ] Step 1: Write README usage requirements

Document build commands, supported macOS/Linux commands, read-only behavior,
the WD19 identifiers, and the difference between descriptor and firmware
versions.

- [ ] Step 2: Run the complete static verification

Run:

~~~bash
go test ./...
go vet ./...
go build -o dockwarden ./cmd/dockwarden
go build -o dockwarden-linux ./cmd/dockwarden
~~~

Expected: all commands pass. Remove generated binaries before committing.

- [ ] Step 3: Run live read-only commands on the Mac

Run:

~~~bash
go run ./cmd/dockwarden scan
go run ./cmd/dockwarden --json scan
go run ./cmd/dockwarden status
go run ./cmd/dockwarden doctor
go run ./cmd/dockwarden check-updates
~~~

Expected: the first four commands identify the WD19 and list observable USB,
Ethernet, audio, and downstream devices. The update command reports an
official candidate or a precise unavailable reason.

- [ ] Step 4: Test Linux cross-compilation

Run GOOS=linux GOARCH=amd64 go build -o /tmp/dockwarden-linux ./cmd/dockwarden.
Expected: PASS without requiring Linux libraries.

- [ ] Step 5: Remove temporary artifacts and run a secret sweep

Run git status --short, git diff --check, and:

~~~bash
git grep -n -I -E '(api[_-]?key|secret|password|token|BEGIN [A-Z ]+PRIVATE KEY|ghp_|sk-[A-Za-z0-9])' || true
~~~

Expected: only intended source and documentation files remain, with no secret
matches.

- [ ] Step 6: Commit the verified MVP

~~~bash
git add README.md internal docs/superpowers/specs/2026-08-04-dockwarden-design.md
git commit -m "feat: deliver dockwarden read-only MVP"
~~~
