package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fexxdev/dockwarden/internal/domain"
)

func TestRenderJSONUsesStableReportKeys(t *testing.T) {
	report := domain.Report{
		SchemaVersion: 1,
		Platform:      "darwin",
		Command:       "scan",
		State:         "detected",
		Dock: &domain.Dock{
			Model:             "Dell Dock WD19",
			VendorID:          0x413c,
			ProductID:         0xb06e,
			DescriptorVersion: "2.0",
		},
	}
	var out bytes.Buffer
	if err := RenderJSON(&out, report); err != nil {
		t.Fatal(err)
	}
	var decoded domain.Report
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"\"schema_version\"", "\"descriptor_version\"", "\"firmware_version\""} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected key %s in output: %s", want, text)
		}
	}
	if decoded.Dock == nil || decoded.Dock.DescriptorVersion != "2.0" ||
		decoded.Dock.FirmwareVersion != "" {
		t.Fatalf("unexpected decoded report: %+v", decoded)
	}
}
