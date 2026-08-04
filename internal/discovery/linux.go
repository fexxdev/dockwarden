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
	runner := c.Runner
	if runner == nil {
		runner = SystemRunner{}
	}

	rawUSB, err := runner.Run(ctx, "lsusb")
	if err != nil {
		return nil, nil, fmt.Errorf("lsusb: %w", err)
	}
	devices, err := ParseLsusb(string(rawUSB))
	if err != nil {
		return nil, nil, fmt.Errorf("parse lsusb: %w", err)
	}

	rawFwupd, err := runner.Run(ctx, "fwupdmgr", "get-devices")
	if err != nil {
		return devices, []string{"fwupdmgr unavailable: " + err.Error()}, nil
	}
	if len(ParseFwupdDevices(string(rawFwupd))) == 0 {
		return devices, []string{"fwupdmgr returned no usable firmware versions"}, nil
	}
	return devices, nil, nil
}
