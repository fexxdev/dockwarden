package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReportJSONKeepsDescriptorSeparateFromFirmware(t *testing.T) {
	got, err := json.Marshal(Report{
		Platform: "darwin",
		Dock: &Dock{
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
	if !strings.Contains(text, `"descriptor_version":"2.0"`) {
		t.Fatalf("descriptor version missing: %s", text)
	}
	if !strings.Contains(text, `"firmware_version":""`) {
		t.Fatalf("firmware version missing: %s", text)
	}
}
