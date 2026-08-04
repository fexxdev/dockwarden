package discovery

import (
	"context"
	"fmt"

	"github.com/fexxdev/dockwarden/internal/domain"
)

type MacInspector struct {
	Runner CommandRunner
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
	return BuildReport("darwin", command, devices), nil
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
