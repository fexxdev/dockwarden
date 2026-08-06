package update

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/fexxdev/dockwarden/internal/domain"
)

func TestSelectFwupdWD19DeviceMatchesUSBSerial(t *testing.T) {
	devices := []fwupdToolDevice{{
		Plugin:      "dell_dock",
		Serial:      "2000/00002000",
		DeviceID:    testFwupdDeviceID,
		InstanceIDs: []string{"USB\\VID_413C&PID_B06E&hub&embedded"},
	}}
	parent, err := selectFwupdWD19DeviceForDock(devices, &domain.Dock{Serial: "2000"})
	if err != nil {
		t.Fatal(err)
	}
	if parent.DeviceID != testFwupdDeviceID {
		t.Fatalf("selected DeviceId = %q", parent.DeviceID)
	}
}

func TestSelectFwupdWD19DeviceMatchesComponentUSBSerial(t *testing.T) {
	devices := []fwupdToolDevice{
		{
			Plugin:      "dell_dock",
			Serial:      "5YVWRV2/3157879355419892",
			DeviceID:    testFwupdDeviceID,
			InstanceIDs: []string{"USB\\VID_413C&PID_B06E&hub&embedded"},
		},
		{
			Plugin:         "dell_dock",
			Serial:         "2000",
			ParentDeviceID: testFwupdDeviceID,
			CompositeID:    testFwupdDeviceID,
			Name:           "RTS5487 in Dell dock",
		},
	}
	parent, err := selectFwupdWD19DeviceForDock(devices, &domain.Dock{Serial: "2000"})
	if err != nil {
		t.Fatal(err)
	}
	if parent.DeviceID != testFwupdDeviceID {
		t.Fatalf("selected DeviceId = %q", parent.DeviceID)
	}
}

func TestSelectFwupdWD19DeviceRejectsAmbiguousInventory(t *testing.T) {
	devices := []fwupdToolDevice{
		{Plugin: "dell_dock", Serial: "2000/00002000", DeviceID: testFwupdDeviceID, InstanceIDs: []string{"USB\\VID_413C&PID_B06E&hub&embedded"}},
		{Plugin: "dell_dock", Serial: "2000/00002000", DeviceID: "fedcba9876543210fedcba9876543210fedcba98", InstanceIDs: []string{"USB\\VID_413C&PID_B06E&hub&embedded"}},
	}
	if _, err := selectFwupdWD19DeviceForDock(devices, &domain.Dock{Serial: "2000"}); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("expected ambiguous inventory error, got %v", err)
	}
}

func TestSelectFwupdWD19DeviceRequiresUSBSerial(t *testing.T) {
	devices := []fwupdToolDevice{{
		Plugin:      "dell_dock",
		Serial:      "2000/00002000",
		DeviceID:    testFwupdDeviceID,
		InstanceIDs: []string{"USB\\VID_413C&PID_B06E&hub&embedded"},
	}}
	if _, err := selectFwupdWD19DeviceForDock(devices, &domain.Dock{}); err == nil || !strings.Contains(err.Error(), "USB serial") {
		t.Fatalf("expected missing USB serial error, got %v", err)
	}
}

func TestFwupdInventoryObservationsRequireAllComponents(t *testing.T) {
	devices := []fwupdToolDevice{{
		Plugin:      "dell_dock",
		DeviceID:    testFwupdDeviceID,
		Version:     "01.01.00.15",
		InstanceIDs: []string{"USB\\VID_413C&PID_B06E&hub&embedded"},
	}}
	if _, err := fwupdInventoryObservations(devices, testFwupdDeviceID); err == nil || !strings.Contains(err.Error(), "package") {
		t.Fatalf("expected missing component error, got %v", err)
	}
}

func TestPhysicalUSBHubGen1DeviceUsesTheSelectedDockTopology(t *testing.T) {
	dock := &domain.Dock{
		Serial: "2000",
		Devices: []domain.USBDevice{
			{VendorID: 0x413c, ProductID: 0xb06e, Location: "00100000", Product: "Dell dock"},
			{VendorID: 0x413c, ProductID: 0xb06e, Location: "00150000", ParentLocation: "00100000", Product: "Dell Dock WD19", Serial: "2000"},
			{VendorID: 0x413c, ProductID: 0xb06f, Location: "00135000", ParentLocation: "00100000", DescriptorVersion: "1.23", Product: "Dell dock"},
			{VendorID: 0x413c, ProductID: 0xb06e, Location: "00200000", Product: "Dell dock"},
			{VendorID: 0x413c, ProductID: 0xb06f, Location: "00235000", ParentLocation: "00200000", DescriptorVersion: "9.99", Product: "Dell dock"},
		},
	}
	device, ok := physicalUSBHubGen1Device(dock)
	if !ok {
		t.Fatal("expected physical Gen1 observation")
	}
	if device.DescriptorVersion != "1.23" || device.Location != "00135000" {
		t.Fatalf("unexpected physical Gen1 device: %+v", device)
	}
}

func TestEnrichFwupdInventoryErrorKeepsWriteEvidenceReadOnly(t *testing.T) {
	dock := &domain.Dock{
		Serial: "2000",
		Devices: []domain.USBDevice{
			{VendorID: 0x413c, ProductID: 0xb06e, Location: "00100000", Product: "Dell dock"},
			{VendorID: 0x413c, ProductID: 0xb06e, Location: "00150000", ParentLocation: "00100000", Product: "Dell Dock WD19", Serial: "2000"},
			{VendorID: 0x413c, ProductID: 0xb06f, Location: "00135000", ParentLocation: "00100000", DescriptorVersion: "1.23", Product: "Dell dock"},
		},
	}
	err := enrichFwupdInventoryError(errors.New("selected WD19 has no usb_hub_gen1 version in fwupdtool output"), dock)
	if !strings.Contains(err.Error(), "00135000") || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("missing physical read-only evidence: %v", err)
	}
}

func TestPhysicalUSBHubGen1DeviceHandlesIntermediateUSBHub(t *testing.T) {
	dock := &domain.Dock{
		Serial: "2000",
		Devices: []domain.USBDevice{
			{VendorID: 0x413c, ProductID: 0xb06e, Location: "00100000", Product: "Dell dock"},
			{VendorID: 0x413c, ProductID: 0xb06e, Location: "00150000", ParentLocation: "00100000", Product: "Dell Dock WD19", Serial: "2000"},
			{VendorID: 0x0bda, ProductID: 0x0413, Location: "00130000", ParentLocation: "00100000", Product: "Dell dock"},
			{VendorID: 0x413c, ProductID: 0xb06f, Location: "00135000", ParentLocation: "00130000", DescriptorVersion: "1.01", Product: "Dell dock"},
		},
	}
	device, ok := physicalUSBHubGen1Device(dock)
	if !ok || device.Location != "00135000" {
		t.Fatalf("selected Gen1 device = %+v", device)
	}
}

func TestFwupdToolPreflightReadsOnlyInventory(t *testing.T) {
	configDir := t.TempDir()
	toolPath := writeManagedFwupdTool(t, configDir)
	runner := &fakeEnvironmentCommandRunner{respond: func(_ string, args []string) ([]byte, error) {
		switch {
		case isVersionCommand(args):
			return []byte(fwupdVersionJSON("2.2.1", "2.2.1")), nil
		case isGetDevicesCommand(args):
			return []byte(fwupdDevicesJSON(fwupdVerifiedDevicesJSON(testFwupdDeviceID))), nil
		default:
			return nil, errors.New("unexpected write command")
		}
	}}
	candidate := domain.FirmwareCandidate{ComponentVersions: testCandidateVersions()}
	candidate.ComponentVersions[domain.FirmwareComponentPackage] = "01.01.02.01"
	result, err := (FwupdToolPreflight{Client: FwupdToolClient{
		Runner:    runner,
		ToolPath:  toolPath,
		ConfigDir: configDir,
		TempDir:   t.TempDir(),
	}}).Check(context.Background(), &domain.Dock{Model: "Dell Dock WD19", VendorID: 0x413c, ProductID: 0xb06e, Serial: "2000"}, &candidate)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeviceID != testFwupdDeviceID || !result.UpdateAvailable {
		t.Fatalf("unexpected preflight result: %+v", result)
	}
	for _, call := range runner.calls {
		if isInstallCommand(call[1:]) {
			t.Fatalf("preflight invoked install: %v", call)
		}
	}
}

func TestFwupdToolFirmwareReaderReadsSelectedInventory(t *testing.T) {
	configDir := t.TempDir()
	toolPath := writeManagedFwupdTool(t, configDir)
	runner := &fakeEnvironmentCommandRunner{respond: func(_ string, args []string) ([]byte, error) {
		switch {
		case isVersionCommand(args):
			return []byte(fwupdVersionJSON("2.2.1", "2.2.1")), nil
		case isGetDevicesCommand(args):
			return []byte(fwupdDevicesJSON(fwupdVerifiedDevicesJSON(testFwupdDeviceID))), nil
		default:
			return nil, errors.New("unexpected command")
		}
	}}
	observations, err := (FwupdToolFirmwareReader{Client: FwupdToolClient{
		Runner: runner, ToolPath: toolPath, ConfigDir: configDir, TempDir: t.TempDir(),
	}}).Read(context.Background(), &domain.Dock{
		Model: "Dell Dock WD19", VendorID: 0x413c, ProductID: 0xb06e, Serial: "2000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != len(fwupdRequiredWD19Components) {
		t.Fatalf("firmware observations = %d, want %d: %+v", len(observations), len(fwupdRequiredWD19Components), observations)
	}
	for _, observation := range observations {
		if observation.Source != "fwupdtool" || observation.Confidence != "direct" {
			t.Fatalf("unexpected observation provenance: %+v", observation)
		}
	}
}

func TestFwupdToolFirmwareReaderReportsPhysicalGen1EvidenceOnIncompleteFwupdInventory(t *testing.T) {
	configDir := t.TempDir()
	toolPath := writeManagedFwupdTool(t, configDir)
	devices := mustFwupdDevices(t, fwupdVerifiedDevicesJSON(testFwupdDeviceID))
	devices = slices.DeleteFunc(devices, func(device fwupdToolDevice) bool {
		return fwupdDeviceComponent(device, testFwupdDeviceID) == domain.FirmwareComponentUSBHubGen1
	})
	devicesJSON, err := json.Marshal(fwupdToolDevicesOutput{Devices: devices})
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeEnvironmentCommandRunner{respond: func(_ string, args []string) ([]byte, error) {
		switch {
		case isVersionCommand(args):
			return []byte(fwupdVersionJSON("2.2.1", "2.2.1")), nil
		case isGetDevicesCommand(args):
			return devicesJSON, nil
		default:
			return nil, errors.New("unexpected write command")
		}
	}}
	dock := &domain.Dock{
		Model:     "Dell Dock WD19",
		VendorID:  0x413c,
		ProductID: 0xb06e,
		Serial:    "2000",
		Devices: []domain.USBDevice{
			{VendorID: 0x413c, ProductID: 0xb06e, Location: "00100000", Product: "Dell dock"},
			{VendorID: 0x413c, ProductID: 0xb06e, Location: "00150000", ParentLocation: "00100000", Product: "Dell Dock WD19", Serial: "2000"},
			{VendorID: 0x413c, ProductID: 0xb06f, Location: "00135000", ParentLocation: "00130000", DescriptorVersion: "1.01", Product: "Dell dock"},
			{VendorID: 0x0bda, ProductID: 0x0413, Location: "00130000", ParentLocation: "00100000", Product: "Dell dock"},
		},
	}
	_, err = (FwupdToolFirmwareReader{Client: FwupdToolClient{
		Runner: runner, ToolPath: toolPath, ConfigDir: configDir, TempDir: t.TempDir(),
	}}).Read(context.Background(), dock)
	if err == nil || !strings.Contains(err.Error(), "00135000") || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("missing physical Gen1 evidence: %v", err)
	}
}

func TestFwupdToolPermissionCheckerReportsPermissionFailure(t *testing.T) {
	configDir := t.TempDir()
	toolPath := writeManagedFwupdTool(t, configDir)
	runner := &fakeEnvironmentCommandRunner{respond: func(_ string, args []string) ([]byte, error) {
		if isVersionCommand(args) {
			return []byte(fwupdVersionJSON("2.2.1", "2.2.1")), nil
		}
		if isGetDevicesCommand(args) {
			return []byte(`{"Error":{"Message":"Input Monitoring permission denied"}}`), errors.New("exit status 1")
		}
		return nil, errors.New("unexpected command")
	}}
	err := (FwupdToolPermissionChecker{Client: FwupdToolClient{
		Runner: runner, ToolPath: toolPath, ConfigDir: configDir, TempDir: t.TempDir(),
	}}).Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "permission probe failed") {
		t.Fatalf("permission failure was not reported: %v", err)
	}
}

func TestFwupdToolPermissionCheckerAllowsNoDock(t *testing.T) {
	configDir := t.TempDir()
	toolPath := writeManagedFwupdTool(t, configDir)
	runner := &fakeEnvironmentCommandRunner{respond: func(_ string, args []string) ([]byte, error) {
		if isVersionCommand(args) {
			return []byte(fwupdVersionJSON("2.2.1", "2.2.1")), nil
		}
		if isGetDevicesCommand(args) {
			return []byte(`{"Error":{"Message":"No detected devices"}}`), errors.New("exit status 2")
		}
		return nil, errors.New("unexpected command")
	}}
	if err := (FwupdToolPermissionChecker{Client: FwupdToolClient{
		Runner: runner, ToolPath: toolPath, ConfigDir: configDir, TempDir: t.TempDir(),
	}}).Check(context.Background()); err != nil {
		t.Fatalf("no attached dock was treated as a permission failure: %v", err)
	}
}

func TestFwupdToolPermissionCheckerReportsNoDevicesWithDock(t *testing.T) {
	configDir := t.TempDir()
	toolPath := writeManagedFwupdTool(t, configDir)
	runner := &fakeEnvironmentCommandRunner{respond: func(_ string, args []string) ([]byte, error) {
		if isVersionCommand(args) {
			return []byte(fwupdVersionJSON("2.2.1", "2.2.1")), nil
		}
		if isGetDevicesCommand(args) {
			return []byte(`{"Error":{"Message":"No detected devices"}}`), errors.New("exit status 2")
		}
		return nil, errors.New("unexpected command")
	}}
	dock := &domain.Dock{Model: "Dell Dock WD19", Serial: "2000"}
	err := (FwupdToolPermissionChecker{Client: FwupdToolClient{
		Runner: runner, ToolPath: toolPath, ConfigDir: configDir, TempDir: t.TempDir(),
	}}).CheckForDock(context.Background(), dock)
	if err == nil || !strings.Contains(err.Error(), "permission probe failed") ||
		!strings.Contains(err.Error(), "Input Monitoring") {
		t.Fatalf("no-device result with a dock was not reported as a permission failure: %v", err)
	}
}

func TestFwupdToolPreflightReportsEqualVersions(t *testing.T) {
	configDir := t.TempDir()
	toolPath := writeManagedFwupdTool(t, configDir)
	runner := &fakeEnvironmentCommandRunner{respond: func(_ string, args []string) ([]byte, error) {
		if isVersionCommand(args) {
			return []byte(fwupdVersionJSON("2.2.1", "2.2.1")), nil
		}
		if isGetDevicesCommand(args) {
			return []byte(fwupdDevicesJSON(fwupdVerifiedDevicesJSON(testFwupdDeviceID))), nil
		}
		return nil, errors.New("unexpected command")
	}}
	candidate := domain.FirmwareCandidate{ComponentVersions: testCandidateVersions()}
	observations, err := fwupdInventoryObservations(mustFwupdDevices(t, fwupdVerifiedDevicesJSON(testFwupdDeviceID)), testFwupdDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	candidate.ComponentVersions[domain.FirmwareComponentPackage] = observations[0].Version
	candidate.ComponentVersions[domain.FirmwareComponentEmbeddedController] = observations[1].Version
	candidate.ComponentVersions[domain.FirmwareComponentUSBHubGen1] = observations[2].Version
	candidate.ComponentVersions[domain.FirmwareComponentUSBHubGen2] = observations[3].Version
	candidate.ComponentVersions[domain.FirmwareComponentMST] = observations[4].Version
	result, err := (FwupdToolPreflight{Client: FwupdToolClient{
		Runner: runner, ToolPath: toolPath, ConfigDir: configDir, TempDir: t.TempDir(),
	}}).Check(context.Background(), &domain.Dock{Model: "Dell Dock WD19", VendorID: 0x413c, ProductID: 0xb06e, Serial: "2000"}, &candidate)
	if err != nil {
		t.Fatal(err)
	}
	if result.UpdateAvailable {
		t.Fatalf("equal candidate was reported as update: %+v", result)
	}
}

func mustFwupdDevices(t *testing.T, text string) []fwupdToolDevice {
	t.Helper()
	var response fwupdToolDevicesOutput
	if err := json.Unmarshal([]byte(fwupdDevicesJSON(text)), &response); err != nil {
		t.Fatal(err)
	}
	return response.Devices
}
