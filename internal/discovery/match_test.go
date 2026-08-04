package discovery

import (
	"testing"

	"github.com/fexxdev/dockwarden/internal/domain"
)

func TestIdentifyWD19(t *testing.T) {
	dock, state := Identify([]domain.USBDevice{{
		Product:           "Dell Dock WD19",
		Vendor:            "Dell Inc.",
		VendorID:          0x413c,
		ProductID:         0xb06e,
		Serial:            "2000",
		DescriptorVersion: "2.0",
	}})
	if state != "detected" {
		t.Fatalf("expected detected, got %q", state)
	}
	if dock == nil || dock.Model != "Dell Dock WD19" {
		t.Fatalf("unexpected dock: %+v", dock)
	}
}

func TestIdentifyUnknownDellDevice(t *testing.T) {
	dock, state := Identify([]domain.USBDevice{{
		Product:   "Dell dock",
		Vendor:    "Dell Inc.",
		VendorID:  0x413c,
		ProductID: 0xffff,
	}})
	if dock != nil || state != "unknown_dell_device" {
		t.Fatalf("unexpected result: dock=%+v state=%q", dock, state)
	}
}

func TestIdentifyNoDock(t *testing.T) {
	dock, state := Identify(nil)
	if dock != nil || state != "no_dock" {
		t.Fatalf("unexpected result: dock=%+v state=%q", dock, state)
	}
}
