package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fexxdev/dockwarden/internal/domain"
)

func TestRenderTextIncludesObservableState(t *testing.T) {
	report := domain.Report{
		Platform: "darwin",
		Command:  "status",
		State:    "detected",
		Dock: &domain.Dock{
			Manufacturer:      "Dell Inc.",
			Model:             "Dell Dock WD19",
			VendorID:          0x413c,
			ProductID:         0xb06e,
			Serial:            "2000",
			DescriptorVersion: "2.0",
			FirmwareVersion:   "",
			Devices: []domain.USBDevice{{
				Kind:    "ethernet",
				Product: "USB 10/100/1000 LAN",
			}},
			Services: []domain.ServiceObservation{
				{Name: "ethernet", State: "pass", Evidence: "USB 10/100/1000 LAN"},
			},
		},
		Warnings: []string{"firmware version unavailable from USB descriptors"},
	}
	var out bytes.Buffer
	if err := RenderText(&out, report, true); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"Dell Dock WD19",
		"413c:b06e",
		"2000",
		"Descriptor version: 2.0",
		"Firmware version: unavailable",
		"USB 10/100/1000 LAN",
		"ethernet: pass",
		"host-observable",
		"firmware version unavailable",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in output:\n%s", want, text)
		}
	}
}
