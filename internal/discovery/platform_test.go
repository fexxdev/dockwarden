package discovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMacInspectorBuildsLiveStyleReport(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "wd19-ioreg.txt"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeCommandRunner{
		outputs: map[string][]byte{
			"ioreg -p IOUSB -l -w 0": input,
		},
		errors: map[string]error{},
	}
	report, err := (MacInspector{Runner: runner}).Inspect(context.Background(), "status")
	if err != nil {
		t.Fatal(err)
	}
	if report.Platform != "darwin" || report.Command != "status" || report.State != "detected" {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestLinuxInspectorCarriesOptionalWarnings(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "wd19-lsusb.txt"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeCommandRunner{
		outputs: map[string][]byte{
			"lsusb": input,
		},
		errors: map[string]error{
			"fwupdmgr get-devices": errors.New("not found"),
		},
	}
	report, err := (LinuxInspector{Runner: runner}).Inspect(context.Background(), "scan")
	if err != nil {
		t.Fatal(err)
	}
	if report.Platform != "linux" || report.State != "detected" {
		t.Fatalf("unexpected report: %+v", report)
	}
	foundWarning := false
	for _, warning := range report.Warnings {
		if strings.Contains(warning, "fwupdmgr unavailable") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("expected fwupdmgr warning: %v", report.Warnings)
	}
}

func TestLinuxInspectorIncludesFwupdVersions(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "wd19-lsusb.txt"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeCommandRunner{
		outputs: map[string][]byte{
			"lsusb":                input,
			"fwupdmgr get-devices": []byte("Devices\nDell Dock WD19\n  Current version: 01.01.14.01\n"),
		},
		errors: map[string]error{},
	}
	report, err := (LinuxInspector{Runner: runner}).Inspect(context.Background(), "status")
	if err != nil {
		t.Fatal(err)
	}
	if report.Dock == nil || len(report.Dock.Firmware) != 1 || report.Dock.Firmware[0].Version != "01.01.14.01" {
		t.Fatalf("expected fwupd firmware inventory: %+v", report.Dock)
	}
}
