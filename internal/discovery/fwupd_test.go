package discovery

import (
	"testing"

	"github.com/fexxdev/dockwarden/internal/domain"
)

func TestParseFwupdDevices(t *testing.T) {
	input := "Devices\n└─WD19:\n  │   Current version: 01.01.00.13\n  │   Vendor: Dell Inc.\n"
	observations := ParseFwupdDevices(input)
	if len(observations) != 1 {
		t.Fatalf("expected one firmware observation, got %d", len(observations))
	}
	if observations[0].Component != domain.FirmwareComponentEmbeddedController ||
		observations[0].Version != "01.01.00.13" ||
		observations[0].Source != "fwupdmgr" {
		t.Fatalf("unexpected firmware observation: %+v", observations[0])
	}
}

func TestParseFwupdDevicesNormalizesWD19Components(t *testing.T) {
	input := "Devices\n" +
		"Dell Dock WD19\n  Current version: 01.00.47.01\n" +
		"Dell WD19 Embedded Controller\n  Current version: 01.01.00.13\n" +
		"Dell WD19 USB Hub Gen1\n  Current version: 01.23\n" +
		"Dell WD19 USB Hub Gen2\n  Current version: 01.62\n" +
		"Dell WD19 MST\n  Current version: 05.07.08\n"
	observations := ParseFwupdDevices(input)
	if len(observations) != 5 {
		t.Fatalf("observations = %d, want 5: %+v", len(observations), observations)
	}
	wantComponents := []string{
		domain.FirmwareComponentPackage,
		domain.FirmwareComponentEmbeddedController,
		domain.FirmwareComponentUSBHubGen1,
		domain.FirmwareComponentUSBHubGen2,
		domain.FirmwareComponentMST,
	}
	for index, want := range wantComponents {
		if observations[index].Component != want {
			t.Fatalf("observation %d component = %q, want %q", index, observations[index].Component, want)
		}
	}
}

func TestParseFwupdDevicesUsesUpstreamDellDockNames(t *testing.T) {
	input := "Devices\n" +
		"└─WD19:\n  │   Current version: 01.01.00.13\n" +
		"├─Package level of Dell dock:\n│  Current version: 01.00.47.01\n" +
		"├─RTS5413 in Dell dock:\n│  Current version: 01.23\n" +
		"├─RTS5487 in Dell dock:\n│  Current version: 01.62\n" +
		"└─VMM5331 in Dell dock:\n   Current version: 05.07.08\n"
	observations := ParseFwupdDevices(input)
	if len(observations) != 5 {
		t.Fatalf("observations = %d, want 5: %+v", len(observations), observations)
	}
	wantComponents := []string{
		domain.FirmwareComponentEmbeddedController,
		domain.FirmwareComponentPackage,
		domain.FirmwareComponentUSBHubGen1,
		domain.FirmwareComponentUSBHubGen2,
		domain.FirmwareComponentMST,
	}
	for index, want := range wantComponents {
		if observations[index].Component != want {
			t.Fatalf("observation %d component = %q, want %q", index, observations[index].Component, want)
		}
	}
}
