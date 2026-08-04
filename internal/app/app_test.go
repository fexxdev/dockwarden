package app

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/fexxdev/dockwarden/internal/cli"
	"github.com/fexxdev/dockwarden/internal/domain"
)

type fakeInspector struct {
	report domain.Report
	err    error
	calls  int
}

func (f *fakeInspector) Inspect(_ context.Context, _ string) (domain.Report, error) {
	f.calls++
	return f.report, f.err
}

type fakeUpdateChecker struct {
	calls  int
	result domain.UpdateCheck
}

type fakeFirmwareUpdater struct {
	calls     int
	dock      *domain.Dock
	candidate *domain.FirmwareCandidate
	result    domain.UpdateCheck
}

func (f *fakeFirmwareUpdater) Apply(_ context.Context, dock *domain.Dock, candidate *domain.FirmwareCandidate) domain.UpdateCheck {
	f.calls++
	f.dock = dock
	f.candidate = candidate
	return f.result
}

func (f *fakeUpdateChecker) Check(_ context.Context, _ *domain.Dock) domain.UpdateCheck {
	f.calls++
	return f.result
}

func detectedReport() domain.Report {
	return domain.Report{
		SchemaVersion: 1,
		Platform:      "darwin",
		State:         "detected",
		Dock: &domain.Dock{
			Model:     "Dell Dock WD19",
			VendorID:  0x413c,
			ProductID: 0xb06e,
			Services: []domain.ServiceObservation{
				{Name: "usb", State: "pass"},
				{Name: "ethernet", State: "pass"},
				{Name: "audio", State: "pass"},
				{Name: "downstream_usb", State: "pass"},
			},
		},
	}
}

func TestRunDetectedCommands(t *testing.T) {
	for _, command := range []string{"scan", "status", "doctor"} {
		t.Run(command, func(t *testing.T) {
			inspector := &fakeInspector{report: detectedReport()}
			var out bytes.Buffer
			code := Run(context.Background(), cli.Options{
				Command: command,
				JSON:    true,
			}, Dependencies{
				Inspector: inspector,
				Out:       &out,
				Err:       &bytes.Buffer{},
			})
			if code != 0 {
				t.Fatalf("expected success, got %d: %s", code, out.String())
			}
			var report domain.Report
			if err := json.Unmarshal(out.Bytes(), &report); err != nil {
				t.Fatalf("expected JSON output: %v", err)
			}
			if report.Command != command {
				t.Fatalf("expected command %q, got %q", command, report.Command)
			}
			if command == "doctor" && len(report.Checks) == 0 {
				t.Fatal("doctor should add checks")
			}
		})
	}
}

func TestRunNoDockReturnsOne(t *testing.T) {
	inspector := &fakeInspector{report: domain.Report{
		Platform: "darwin",
		State:    "no_dock",
	}}
	var out bytes.Buffer
	code := Run(context.Background(), cli.Options{
		Command: "status",
		JSON:    true,
	}, Dependencies{
		Inspector: inspector,
		Out:       &out,
		Err:       &bytes.Buffer{},
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
}

func TestRunChecksUpdatesOnlyForDetectedDock(t *testing.T) {
	t.Run("detected", func(t *testing.T) {
		inspector := &fakeInspector{report: detectedReport()}
		updates := &fakeUpdateChecker{result: domain.UpdateCheck{State: "no_update"}}
		var out bytes.Buffer
		code := Run(context.Background(), cli.Options{
			Command: "check-updates",
			JSON:    true,
		}, Dependencies{
			Inspector: inspector,
			Updates:   updates,
			Out:       &out,
			Err:       &bytes.Buffer{},
		})
		if code != 0 || updates.calls != 1 {
			t.Fatalf("unexpected result: code=%d update_calls=%d", code, updates.calls)
		}
	})

	t.Run("no dock", func(t *testing.T) {
		inspector := &fakeInspector{report: domain.Report{State: "no_dock"}}
		updates := &fakeUpdateChecker{}
		var out bytes.Buffer
		code := Run(context.Background(), cli.Options{
			Command: "check-updates",
			JSON:    true,
		}, Dependencies{
			Inspector: inspector,
			Updates:   updates,
			Out:       &out,
			Err:       &bytes.Buffer{},
		})
		if code != 1 || updates.calls != 0 {
			t.Fatalf("unexpected result: code=%d update_calls=%d", code, updates.calls)
		}
	})
}

func TestRunUpdatePlansWithoutApply(t *testing.T) {
	inspector := &fakeInspector{report: detectedReport()}
	updates := &fakeUpdateChecker{result: domain.UpdateCheck{
		State:     "update_available",
		SourceURL: "https://www.dell.com/support/drivers",
		Candidate: &domain.FirmwareCandidate{PackageName: "wd19.cab"},
	}}
	updater := &fakeFirmwareUpdater{}
	var out bytes.Buffer
	code := Run(context.Background(), cli.Options{
		Command: "update",
		JSON:    true,
	}, Dependencies{
		Inspector: inspector,
		Updates:   updates,
		Updater:   updater,
		Out:       &out,
		Err:       &bytes.Buffer{},
	})
	if code != 0 || updater.calls != 0 {
		t.Fatalf("expected plan-only update, code=%d updater_calls=%d", code, updater.calls)
	}
	var report domain.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Update == nil || report.Update.State != "update_available" || report.Update.Reason == "" {
		t.Fatalf("unexpected update plan: %+v", report.Update)
	}
}

func TestRunUpdateAppliesOnlyWithApply(t *testing.T) {
	inspector := &fakeInspector{report: detectedReport()}
	updates := &fakeUpdateChecker{result: domain.UpdateCheck{
		State:     "update_available",
		SourceURL: "https://www.dell.com/support/drivers",
		Candidate: &domain.FirmwareCandidate{PackageName: "wd19.cab"},
	}}
	updater := &fakeFirmwareUpdater{result: domain.UpdateCheck{State: "update_applied", Reason: "fwupdmgr accepted the package"}}
	var out bytes.Buffer
	code := Run(context.Background(), cli.Options{
		Command: "update",
		Apply:   true,
		JSON:    true,
	}, Dependencies{
		Inspector: inspector,
		Updates:   updates,
		Updater:   updater,
		Out:       &out,
		Err:       &bytes.Buffer{},
	})
	if code != 0 || updater.calls != 1 {
		t.Fatalf("expected one applied update, code=%d updater_calls=%d", code, updater.calls)
	}
	if updater.dock == nil || updater.candidate == nil {
		t.Fatal("expected updater to receive the detected dock and candidate")
	}
}
