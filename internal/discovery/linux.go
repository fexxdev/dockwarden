package discovery

import (
	"context"
	"fmt"

	"github.com/fexxdev/dockwarden/internal/domain"
)

type LinuxCollector struct {
	Runner CommandRunner
}

func (c LinuxCollector) Collect(ctx context.Context) ([]domain.USBDevice, []string, error) {
	devices, _, warnings, err := c.collect(ctx)
	return devices, warnings, err
}

func (c LinuxCollector) CollectWithFirmware(ctx context.Context) ([]domain.USBDevice, []domain.FirmwareObservation, []string, error) {
	return c.collect(ctx)
}

func (c LinuxCollector) collect(ctx context.Context) ([]domain.USBDevice, []domain.FirmwareObservation, []string, error) {
	runner := c.Runner
	if runner == nil {
		runner = SystemRunner{}
	}

	rawUSB, err := runner.Run(ctx, "lsusb")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("lsusb: %w", err)
	}
	devices, err := ParseLsusb(string(rawUSB))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse lsusb: %w", err)
	}

	rawFwupd, err := runner.Run(ctx, "fwupdmgr", "get-devices")
	if err != nil {
		return devices, nil, []string{"fwupdmgr unavailable: " + err.Error()}, nil
	}
	firmware := ParseFwupdDevices(string(rawFwupd))
	if len(firmware) == 0 {
		return devices, nil, []string{"fwupdmgr returned no usable firmware versions"}, nil
	}
	return devices, firmware, nil, nil
}
