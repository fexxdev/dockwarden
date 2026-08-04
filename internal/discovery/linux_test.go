package discovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeCommandRunner struct {
	outputs map[string][]byte
	errors  map[string]error
	calls   []string
}

func (f *fakeCommandRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, key)
	if err := f.errors[key]; err != nil {
		return nil, err
	}
	return f.outputs[key], nil
}

func TestLinuxCollectorKeepsFwupdOptional(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "wd19-lsusb.txt"))
	if err != nil {
		t.Fatal(err)
	}

	runner := &fakeCommandRunner{
		outputs: map[string][]byte{
			"lsusb": input,
		},
		errors: map[string]error{
			"fwupdmgr get-devices": errors.New("executable file not found"),
		},
	}
	devices, warnings, err := (LinuxCollector{Runner: runner}).Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 4 {
		t.Fatalf("expected four devices, got %d", len(devices))
	}
	foundWarning := false
	for _, warning := range warnings {
		if strings.Contains(warning, "fwupdmgr unavailable") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("expected fwupdmgr warning, got %v", warnings)
	}
}
