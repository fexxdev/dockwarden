package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fexxdev/dockwarden/internal/cli"
	"github.com/fexxdev/dockwarden/internal/domain"
)

type fakeLogger struct {
	events []string
	fields []map[string]string
}

func (f *fakeLogger) Log(_ string, event string, fields map[string]string) error {
	f.events = append(f.events, event)
	f.fields = append(f.fields, fields)
	return nil
}

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

type fakeReadyFirmwareUpdater struct {
	fakeFirmwareUpdater
	readyCalls int
	readyErr   error
}

func (f *fakeReadyFirmwareUpdater) CheckReady(context.Context) error {
	f.readyCalls++
	return f.readyErr
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

func TestRunLogsCommandLifecycle(t *testing.T) {
	inspector := &fakeInspector{report: detectedReport()}
	logger := &fakeLogger{}
	var out bytes.Buffer
	code := Run(context.Background(), cli.Options{Command: "status", JSON: true}, Dependencies{
		Inspector: inspector,
		Logger:    logger,
		Out:       &out,
		Err:       &bytes.Buffer{},
	})
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	for _, event := range []string{"command.start", "inspect.complete", "command.complete"} {
		found := false
		for _, got := range logger.events {
			if got == event {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing log event %q in %v", event, logger.events)
		}
	}
	for index, event := range logger.events {
		if event == "command.complete" && logger.fields[index]["report"] == "" {
			t.Fatal("command completion must include the full report in the log")
		}
	}
}

func TestRunReportsMacOSPermissionInstructions(t *testing.T) {
	inspector := &fakeInspector{report: detectedReport()}
	var checkedDock *domain.Dock
	var out bytes.Buffer
	code := Run(context.Background(), cli.Options{Command: "doctor", JSON: true}, Dependencies{
		Inspector: inspector,
		PermissionCheckForDock: func(_ context.Context, dock *domain.Dock) error {
			checkedDock = dock
			return errors.New("macOS denied direct HID access: IOKit 0x2c")
		},
		Out: &out,
		Err: &bytes.Buffer{},
	})
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	var report domain.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	foundWarning := false
	foundCheck := false
	for _, warning := range report.Warnings {
		if warning == "macOS HID/Input Monitoring permission is not available: macOS denied direct HID access: IOKit 0x2c. Open System Settings > Privacy & Security > Input Monitoring, enable the terminal or app that runs dockwarden, then quit and reopen it." {
			foundWarning = true
		}
	}
	for _, check := range report.Checks {
		if check.Name == "macos_hid_permission" && check.State == "warning" {
			foundCheck = true
		}
	}
	if checkedDock == nil || !foundWarning || !foundCheck {
		t.Fatalf("permission warning/check missing: warnings=%v checks=%v", report.Warnings, report.Checks)
	}
}

func TestRunBlocksMacOSApplyWithoutPermission(t *testing.T) {
	inspector := &fakeInspector{report: detectedReport()}
	updates := &fakeUpdateChecker{result: domain.UpdateCheck{State: "update_available"}}
	updater := &fakeFirmwareUpdater{}
	var out bytes.Buffer
	code := Run(context.Background(), cli.Options{Command: "update", Apply: true, JSON: true}, Dependencies{
		Inspector: inspector,
		Updates:   updates,
		Updater:   updater,
		PermissionCheck: func(context.Context) error {
			return errors.New("macOS denied direct HID access")
		},
		Out: &out,
		Err: &bytes.Buffer{},
	})
	if code != 2 || updates.calls != 0 || updater.calls != 0 {
		t.Fatalf("permission failure did not stop apply: code=%d updates=%d applies=%d", code, updates.calls, updater.calls)
	}
	var report domain.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Update == nil || report.Update.State != "update_failed" {
		t.Fatalf("expected permission failure update result: %+v", report.Update)
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

func TestRunUpdateStagesOnlyWithApply(t *testing.T) {
	inspector := &fakeInspector{report: detectedReport()}
	updates := &fakeUpdateChecker{result: domain.UpdateCheck{
		State:     "update_available",
		SourceURL: "https://www.dell.com/support/drivers",
		Candidate: &domain.FirmwareCandidate{PackageName: "wd19.cab"},
	}}
	updater := &fakeFirmwareUpdater{result: domain.UpdateCheck{State: "update_staged", Reason: "fwupdmgr accepted the package; reconnect the dock"}}
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

func TestRunUpdateApplyAttestsWriterBeforeCatalogNetwork(t *testing.T) {
	inspector := &fakeInspector{report: detectedReport()}
	updates := &fakeUpdateChecker{result: domain.UpdateCheck{State: "update_available"}}
	updater := &fakeReadyFirmwareUpdater{readyErr: errors.New("managed writer is not trusted")}
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
	if code != 2 || updater.readyCalls != 1 || updates.calls != 0 || updater.calls != 0 {
		t.Fatalf("writer readiness did not stop before catalog: code=%d ready=%d catalog=%d apply=%d", code, updater.readyCalls, updates.calls, updater.calls)
	}
	var report domain.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Update == nil || report.Update.State != "update_failed" || report.Update.Reason != "managed writer is not trusted" {
		t.Fatalf("unexpected readiness result: %+v", report.Update)
	}
}

func TestRunUpdateReportsUnsupportedWithoutBackend(t *testing.T) {
	inspector := &fakeInspector{report: detectedReport()}
	updates := &fakeUpdateChecker{result: domain.UpdateCheck{
		State:     "update_available",
		SourceURL: "https://www.dell.com/support/drivers",
		Candidate: &domain.FirmwareCandidate{PackageName: "wd19.cab"},
	}}
	var out bytes.Buffer
	code := Run(context.Background(), cli.Options{
		Command: "update",
		Apply:   true,
		JSON:    true,
	}, Dependencies{
		Inspector: inspector,
		Updates:   updates,
		Out:       &out,
		Err:       &bytes.Buffer{},
	})
	if code != 2 {
		t.Fatalf("expected unsupported apply exit code 2, got %d", code)
	}
	var report domain.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Update == nil || report.Update.State != "unsupported" {
		t.Fatalf("unexpected unsupported result: %+v", report.Update)
	}
}

func TestRunUpdateApplyRejectsUnavailableVersionCheck(t *testing.T) {
	inspector := &fakeInspector{report: detectedReport()}
	updates := &fakeUpdateChecker{result: domain.UpdateCheck{
		State:  "version_check_unavailable",
		Reason: "detected dock has no mst version",
	}}
	updater := &fakeFirmwareUpdater{}
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
	if code != 2 || updater.calls != 0 {
		t.Fatalf("expected unavailable version check to stop apply, code=%d updater_calls=%d", code, updater.calls)
	}
}
