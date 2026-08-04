package discovery

import (
	"testing"
)

func TestParseFwupdDevices(t *testing.T) {
	input := "Devices\nDell Dock WD19\n  Current version: 01.00.00\n  Vendor: Dell Inc.\n"
	observations := ParseFwupdDevices(input)
	if len(observations) != 1 {
		t.Fatalf("expected one firmware observation, got %d", len(observations))
	}
	if observations[0].Component != "Dell Dock WD19" ||
		observations[0].Version != "01.00.00" ||
		observations[0].Source != "fwupdmgr" {
		t.Fatalf("unexpected firmware observation: %+v", observations[0])
	}
}
