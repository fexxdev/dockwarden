package app

import (
	"context"
	"fmt"
	"io"

	"github.com/fexxdev/dockwarden/internal/cli"
	"github.com/fexxdev/dockwarden/internal/domain"
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

type Dependencies struct {
	Inspector Inspector
	Updates   UpdateChecker
	Updater   FirmwareUpdater
	Out       io.Writer
	Err       io.Writer
}

func Run(ctx context.Context, options cli.Options, dependencies Dependencies) int {
	if dependencies.Inspector == nil {
		writeError(dependencies.Err, "inspector is not configured")
		return 2
	}

	report, err := dependencies.Inspector.Inspect(ctx, options.Command)
	if err != nil {
		writeError(dependencies.Err, err.Error())
		return 2
	}
	report.Command = options.Command

	if options.Command == "doctor" {
		report.Checks = append(report.Checks, doctorChecks(report)...)
	}
	if options.Command == "check-updates" {
		report.Update = checkUpdates(ctx, report, dependencies.Updates)
	}
	if options.Command == "update" {
		report.Update = runFirmwareUpdate(ctx, report, dependencies.Updates, dependencies.Updater, options.Apply)
	}

	if options.JSON {
		err = output.RenderJSON(dependencies.Out, report)
	} else {
		err = output.RenderText(dependencies.Out, report, options.Verbose)
	}
	if err != nil {
		writeError(dependencies.Err, err.Error())
		return 2
	}
	if report.State == "detected" {
		if options.Command == "update" && options.Apply && report.Update != nil {
			switch report.Update.State {
			case "vendor_metadata_unavailable", "unsupported", "update_failed":
				return 2
			}
		}
		return 0
	}
	return 1
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
	if updater == nil {
		result.State = "unsupported"
		result.Reason = "no firmware write backend is available for this platform"
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
