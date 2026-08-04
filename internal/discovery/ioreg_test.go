package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseIORegistry(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "wd19-ioreg.txt"))
	if err != nil {
		t.Fatal(err)
	}

	devices, err := ParseIORegistry(string(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 5 {
		t.Fatalf("expected five USB devices, got %d", len(devices))
	}

	root, lan := -1, -1
	for i := range devices {
		if devices[i].Product == "Dell Dock WD19" {
			root = i
		}
		if devices[i].Product == "USB 10/100/1000 LAN" {
			lan = i
		}
	}
	if root < 0 || lan < 0 {
		t.Fatalf("missing WD19 root or LAN device: %+v", devices)
	}
	if devices[root].VendorID != 0x413c || devices[root].ProductID != 0xb06e {
		t.Fatalf("unexpected WD19 identifiers: %+v", devices[root])
	}
	if devices[root].Serial != "2000" || devices[root].DescriptorVersion != "2.0" {
		t.Fatalf("unexpected WD19 identity fields: %+v", devices[root])
	}
	if devices[lan].Vendor != "Realtek" || devices[lan].Depth <= devices[root].Depth {
		t.Fatalf("unexpected LAN fields or depth: %+v", devices[lan])
	}
	if devices[lan].ParentLocation != "00152000" {
		t.Fatalf("unexpected LAN parent location: %+v", devices[lan])
	}
}
