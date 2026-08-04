package discovery

import (
	"context"
	"fmt"

	"github.com/fexxdev/dockwarden/internal/domain"
)

type MacInspector struct {
	Runner   CommandRunner
	Firmware FirmwareReader
}

type FirmwareReader interface {
	Read(context.Context, *domain.Dock) ([]domain.FirmwareObservation, error)
}

func (i MacInspector) Inspect(ctx context.Context, command string) (domain.Report, error) {
	runner := i.Runner
	if runner == nil {
		runner = SystemRunner{}
	}

	raw, err := runner.Run(ctx, "ioreg", "-p", "IOUSB", "-l", "-w", "0")
	if err != nil {
		return domain.Report{}, fmt.Errorf("ioreg: %w", err)
	}
	devices, err := ParseIORegistry(string(raw))
	if err != nil {
		return domain.Report{}, fmt.Errorf("parse ioreg: %w", err)
	}
	report := BuildReport("darwin", command, devices)
	if report.Dock != nil && i.Firmware != nil {
		firmware, firmwareErr := i.Firmware.Read(ctx, report.Dock)
		if firmwareErr != nil {
			report.Warnings = append(report.Warnings, "macOS firmware reader unavailable: "+firmwareErr.Error())
		} else {
			report.Dock.Firmware = firmware
			report.Warnings = removeWarning(report.Warnings, "firmware version unavailable from USB descriptors")
		}
	}
	return report, nil
}

func removeWarning(warnings []string, unwanted string) []string {
	filtered := warnings[:0]
	for _, warning := range warnings {
		if warning != unwanted {
			filtered = append(filtered, warning)
		}
	}
	return filtered
}

type LinuxInspector struct {
	Runner CommandRunner
}

func (i LinuxInspector) Inspect(ctx context.Context, command string) (domain.Report, error) {
	devices, firmware, warnings, err := (LinuxCollector{Runner: i.Runner}).CollectWithFirmware(ctx)
	if err != nil {
		return domain.Report{}, err
	}
	report := BuildReport("linux", command, devices)
	if report.Dock != nil {
		report.Dock.Firmware = firmware
	}
	report.Warnings = append(report.Warnings, warnings...)
	return report, nil
}
