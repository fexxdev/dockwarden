package discovery

import (
	"testing"

	"github.com/fexxdev/dockwarden/internal/domain"
)

func TestBuildReportMarksObservableServices(t *testing.T) {
	report := BuildReport("darwin", "status", []domain.USBDevice{
		{Product: "Dell Dock WD19", Vendor: "Dell Inc.", VendorID: 0x413c, ProductID: 0xb06e},
		{Product: "USB 10/100/1000 LAN", Vendor: "Realtek", VendorID: 0x0bda, ProductID: 0x8153},
		{Product: "USB Audio", Vendor: "Generic"},
		{Product: "Keychron K8", Vendor: "Keychron K8"},
	})
	if report.State != "detected" || report.Dock == nil {
		t.Fatalf("unexpected report: %+v", report)
	}
	if len(report.Dock.Services) != 4 {
		t.Fatalf("expected four service observations, got %d", len(report.Dock.Services))
	}
	if report.Dock.FirmwareVersion != "" {
		t.Fatalf("descriptor data must not become firmware: %+v", report.Dock)
	}
}
