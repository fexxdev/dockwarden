package update

import (
	"context"
	"testing"

	"github.com/fexxdev/dockwarden/internal/domain"
)

func TestMacFirmwareReaderReportsComponentVersions(t *testing.T) {
	base := &fakeHIDConnection{
		fakeHIDReportDevice: fakeHIDReportDevice{inputs: wd19ReadOnlyInputs()},
	}
	var opened domain.HIDTarget
	reader := MacFirmwareReader{
		Open: func(target domain.HIDTarget) (HIDConnection, error) {
			opened = target
			return base, nil
		},
	}

	observations, err := reader.Read(context.Background(), matchingDock())
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if opened.ProductID != wd19Gen2ProductID || opened.LocationID != 0x00150000 {
		t.Fatalf("reader opened the wrong target: %+v", opened)
	}
	if len(observations) != 8 {
		t.Fatalf("observations = %d, want package plus seven components: %+v", len(observations), observations)
	}
	if observations[0].Component != "package" || observations[0].Version != "01.00.47.01" {
		t.Fatalf("unexpected package observation: %+v", observations[0])
	}
	if observations[1].Component != "embedded_controller" || observations[1].Version != "01.01.00.13" {
		t.Fatalf("unexpected EC observation: %+v", observations[1])
	}
}

func TestMacFirmwareReaderRejectsPendingUpdate(t *testing.T) {
	inputs := wd19ReadOnlyInputs()
	inputs[2][1] = 0
	base := &fakeHIDConnection{
		fakeHIDReportDevice: fakeHIDReportDevice{inputs: inputs},
	}
	reader := MacFirmwareReader{
		Open: func(domain.HIDTarget) (HIDConnection, error) {
			return base, nil
		},
	}
	if _, err := reader.Read(context.Background(), matchingDock()); err == nil {
		t.Fatal("pending firmware update must not be reported as verified")
	}
}
