package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLsusb(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "wd19-lsusb.txt"))
	if err != nil {
		t.Fatal(err)
	}

	devices, err := ParseLsusb(string(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 4 {
		t.Fatalf("expected four USB devices, got %d", len(devices))
	}

	root := devices[0]
	if root.VendorID != 0x413c || root.ProductID != 0xb06e {
		t.Fatalf("unexpected root IDs: %+v", root)
	}
	if root.Vendor != "Dell Inc." || root.Product != "Dell Dock WD19" {
		t.Fatalf("unexpected root description: %+v", root)
	}
	if devices[2].VendorID != 0x0bda || devices[2].ProductID != 0x8153 {
		t.Fatalf("unexpected Realtek IDs: %+v", devices[2])
	}
	if devices[2].Product != "USB 10/100/1000 LAN" {
		t.Fatalf("product text not preserved: %+v", devices[2])
	}
}
