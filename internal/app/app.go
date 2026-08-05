package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/fexxdev/dockwarden/internal/cli"
	"github.com/fexxdev/dockwarden/internal/domain"
	"github.com/fexxdev/dockwarden/internal/logging"
	"github.com/fexxdev/dockwarden/internal/output"
)

type Inspector interface {
	Inspect(context.Context, string) (domain.Report, error)
}

type UpdateChecker interface {
	Check(context.Context, *domain.Dock) domain.UpdateCheck
}

type FirmwareUpdater interface {
	Apply(context.Context, *domain.Dock, *domain.FirmwareCandidate) domain.UpdateCheck
}

type FirmwareUpdaterReadiness interface {
	CheckReady(context.Context) error
}

type Dependencies struct {
	Inspector       Inspector
	Updates         UpdateChecker
	Updater         FirmwareUpdater
	PermissionCheck func(context.Context) error
	Logger          logging.Logger
	Out             io.Writer
	Err             io.Writer
}

func Run(ctx context.Context, options cli.Options, dependencies Dependencies) int {
	logEvent(dependencies.Logger, "INFO", "command.start", map[string]string{
		"command": options.Command,
		"apply":   fmt.Sprintf("%t", options.Apply),
		"json":    fmt.Sprintf("%t", options.JSON),
	})
	if dependencies.Inspector == nil {
		logEvent(dependencies.Logger, "ERROR", "command.error", map[string]string{"reason": "inspector is not configured"})
		writeError(dependencies.Err, "inspector is not configured")
		return 2
	}

	report, err := dependencies.Inspector.Inspect(ctx, options.Command)
	if err != nil {
		logEvent(dependencies.Logger, "ERROR", "inspect.failed", map[string]string{"error": err.Error()})
		writeError(dependencies.Err, err.Error())
		return 2
	}
	report.Command = options.Command
	logEvent(dependencies.Logger, "INFO", "inspect.complete", map[string]string{
		"state":    report.State,
		"platform": report.Platform,
		"dock":     reportDockName(report),
	})
	var permissionErr error
	if dependencies.PermissionCheck != nil {
		permissionErr = dependencies.PermissionCheck(ctx)
		if permissionErr != nil {
			warning := macOSPermissionWarning(permissionErr)
			report.Warnings = append(report.Warnings, warning)
			logEvent(dependencies.Logger, "WARN", "permissions.failed", map[string]string{"error": permissionErr.Error()})
		}
	}

	if options.Command == "doctor" {
		report.Checks = append(report.Checks, doctorChecks(report)...)
		if report.Platform == "darwin" {
			report.Checks = append(report.Checks, macOSPermissionCheck(report))
		}
	}
	if options.Command == "check-updates" {
		report.Update = checkUpdates(ctx, report, dependencies.Updates)
	}
	if options.Command == "update" && options.Apply && permissionErr != nil && report.Platform == "darwin" && report.State == "detected" {
		reason := "macOS fwupd inventory check is required before a firmware apply: " + permissionErr.Error()
		if isMacOSPermissionError(permissionErr) {
			reason = "macOS HID/Input Monitoring permission is required before a firmware apply: " + permissionErr.Error()
		}
		report.Update = &domain.UpdateCheck{
			State:  "update_failed",
			Reason: reason,
		}
	} else if options.Command == "update" {
		report.Update = runFirmwareUpdate(ctx, report, dependencies.Updates, dependencies.Updater, options.Apply)
	}

	if options.JSON {
		err = output.RenderJSON(dependencies.Out, report)
	} else {
		err = output.RenderText(dependencies.Out, report, options.Verbose)
	}
	if err != nil {
		logEvent(dependencies.Logger, "ERROR", "render.failed", map[string]string{"error": err.Error()})
		writeError(dependencies.Err, err.Error())
		return 2
	}
	if report.Update != nil {
		logEvent(dependencies.Logger, updateLogLevel(report.Update.State), "update.result", map[string]string{
			"state":  report.Update.State,
			"reason": report.Update.Reason,
		})
	}
	completionFields := map[string]string{
		"command": options.Command,
		"state":   report.State,
	}
	if snapshot, marshalErr := json.Marshal(report); marshalErr == nil {
		completionFields["report"] = string(snapshot)
	}
	logEvent(dependencies.Logger, "INFO", "command.complete", completionFields)
	if report.State == "detected" {
		if options.Command == "update" && options.Apply && report.Update != nil {
			switch report.Update.State {
			case "vendor_metadata_unavailable", "version_check_unavailable", "unsupported", "update_failed":
				return 2
			}
		}
		return 0
	}
	return 1
}

const macOSPermissionHelp = "Open System Settings > Privacy & Security > Input Monitoring, enable the terminal or app that runs dockwarden, then quit and reopen it."

func macOSPermissionWarning(err error) string {
	details := strings.TrimSpace(err.Error())
	if details == "" {
		details = "the HID permission probe failed"
	}
	if !isMacOSPermissionError(err) {
		return "macOS fwupd inventory check is not available: " + details
	}
	if !strings.Contains(details, "Input Monitoring") {
		details += ". " + macOSPermissionHelp
	}
	return "macOS HID/Input Monitoring permission is not available: " + details
}

func isMacOSPermissionError(err error) bool {
	if err == nil {
		return false
	}
	details := strings.ToLower(err.Error())
	for _, marker := range []string{
		"input monitoring",
		"permission",
		"access denied",
		"not permitted",
		"not authorized",
		"hid access",
	} {
		if strings.Contains(details, marker) {
			return true
		}
	}
	return false
}

func macOSPermissionCheck(report domain.Report) domain.Check {
	check := domain.Check{
		Name:    "macos_hid_permission",
		State:   "pass",
		Details: "read-only fwupd HID inventory probe succeeded",
	}
	for _, warning := range report.Warnings {
		if strings.HasPrefix(warning, "macOS HID/Input Monitoring permission is not available:") ||
			strings.HasPrefix(warning, "macOS fwupd inventory check is not available:") {
			check.State = "warning"
			check.Details = warning
			break
		}
	}
	return check
}

func logEvent(logger logging.Logger, level, event string, fields map[string]string) {
	if logger != nil {
		_ = logger.Log(level, event, fields)
	}
}

func reportDockName(report domain.Report) string {
	if report.Dock == nil {
		return ""
	}
	return report.Dock.Model
}

func updateLogLevel(state string) string {
	if state == "update_failed" {
		return "ERROR"
	}
	return "INFO"
}

func checkUpdates(ctx context.Context, report domain.Report, updates UpdateChecker) *domain.UpdateCheck {
	if report.State != "detected" || report.Dock == nil {
		return &domain.UpdateCheck{
			State:  "not_checked",
			Reason: "dock not detected",
		}
	}
	if updates == nil {
		return &domain.UpdateCheck{
			State:  "vendor_metadata_unavailable",
			Reason: "update checker is not configured",
		}
	}
	result := updates.Check(ctx, report.Dock)
	return &result
}

func runFirmwareUpdate(ctx context.Context, report domain.Report, updates UpdateChecker, updater FirmwareUpdater, apply bool) *domain.UpdateCheck {
	if report.State != "detected" || report.Dock == nil {
		return checkUpdates(ctx, report, updates)
	}
	if apply {
		if updater == nil {
			return &domain.UpdateCheck{
				State:  "unsupported",
				Reason: "no firmware write backend is available for this platform",
			}
		}
		if readiness, ok := updater.(FirmwareUpdaterReadiness); ok {
			if err := readiness.CheckReady(ctx); err != nil {
				return &domain.UpdateCheck{
					State:  "update_failed",
					Reason: err.Error(),
				}
			}
		}
	}
	result := checkUpdates(ctx, report, updates)
	if !apply {
		if result.State == "update_available" {
			result.Reason = "plan only; re-run with --apply to download and install the candidate"
		}
		return result
	}
	if result.State != "update_available" || result.Candidate == nil {
		return result
	}
	applied := updater.Apply(ctx, report.Dock, result.Candidate)
	if applied.SourceURL == "" {
		applied.SourceURL = result.SourceURL
	}
	if applied.Candidate == nil {
		applied.Candidate = result.Candidate
	}
	return &applied
}

func doctorChecks(report domain.Report) []domain.Check {
	checks := []domain.Check{
		{
			Name:    "model_identity",
			State:   "missing",
			Details: "WD19 was not detected",
		},
		{
			Name:    "usb_enumeration",
			State:   "missing",
			Details: "no dock USB devices were enumerated",
		},
		{
			Name:    "ethernet",
			State:   "missing",
			Details: "Ethernet interface was not enumerated",
		},
		{
			Name:    "audio",
			State:   "missing",
			Details: "audio interface was not enumerated",
		},
		{
			Name:    "downstream_usb",
			State:   "missing",
			Details: "no downstream USB device was enumerated",
		},
		{
			Name:    "firmware",
			State:   "unavailable",
			Details: "no firmware-aware source reported a version",
		},
	}
	if report.Dock == nil {
		return checks
	}

	checks[0] = domain.Check{
		Name:    "model_identity",
		State:   "pass",
		Details: report.Dock.Model,
	}
	if len(report.Dock.Devices) > 0 {
		checks[1] = domain.Check{
			Name:    "usb_enumeration",
			State:   "pass",
			Details: fmt.Sprintf("%d USB devices", len(report.Dock.Devices)),
		}
	}
	for index := 2; index <= 4; index++ {
		serviceName := checks[index].Name
		for _, service := range report.Dock.Services {
			if service.Name == serviceName {
				checks[index] = domain.Check{
					Name:    service.Name,
					State:   service.State,
					Details: service.Evidence,
				}
				break
			}
		}
	}
	if report.Dock.FirmwareVersion != "" || len(report.Dock.Firmware) > 0 {
		checks[5] = domain.Check{
			Name:    "firmware",
			State:   "pass",
			Details: "version reported by a firmware-aware source",
		}
	}
	return checks
}

func writeError(w io.Writer, message string) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "dockwarden: %s\n", message)
}
