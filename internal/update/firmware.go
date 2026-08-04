package update

import (
	"context"
	"fmt"

	"github.com/fexxdev/dockwarden/internal/domain"
)

type MacFirmwareReader struct {
	Open HIDOpener
}

func (r MacFirmwareReader) Read(ctx context.Context, dock *domain.Dock) ([]domain.FirmwareObservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !isSupportedWD19(dock) {
		return nil, fmt.Errorf("firmware reader accepts only the detected Dell Dock WD19 (413c:b06e)")
	}
	if r.Open == nil {
		return nil, fmt.Errorf("macOS HID opener is not configured")
	}
	target, err := hidTargetForProduct(dock, wd19Gen2ProductID)
	if err != nil {
		return nil, err
	}
	connection, err := r.Open(target)
	if err != nil {
		return nil, fmt.Errorf("cannot open WD19 firmware HID: %w", err)
	}
	if connection == nil {
		return nil, fmt.Errorf("macOS HID opener returned no WD19 firmware device")
	}
	defer connection.Close()
	device := DellHID{Reports: connection}

	dockData, err := device.ReadDockData()
	if err != nil {
		return nil, fmt.Errorf("cannot read WD19 firmware data: %w", err)
	}
	if dockData.DockType != salomonDockType {
		return nil, fmt.Errorf("detected WD19 dock type %#02x is not supported by the native reader", dockData.DockType)
	}
	components, err := device.ReadDockInfo()
	if err != nil {
		return nil, fmt.Errorf("cannot read WD19 component versions: %w", err)
	}
	status, err := device.ReadUpdateStatus()
	if err != nil {
		return nil, fmt.Errorf("cannot read WD19 firmware update status: %w", err)
	}
	if status != firmwareUpdateComplete {
		return nil, fmt.Errorf("WD19 reports firmware update status %#02x; versions are not verified", status)
	}

	observations := []domain.FirmwareObservation{{
		Component:  "package",
		Version:    dockData.PackageVersion,
		Source:     "macos_hid",
		Confidence: "direct",
	}}
	for _, component := range components {
		observations = append(observations, domain.FirmwareObservation{
			Component:  firmwareComponentName(component),
			Version:    component.Version,
			Source:     "macos_hid",
			Confidence: "direct",
		})
	}
	return observations, nil
}

func firmwareComponentName(component DockComponent) string {
	switch component.DeviceType {
	case dockDeviceTypeEC:
		return "embedded_controller"
	case dockDeviceTypePD:
		return "power_delivery"
	case dockDeviceTypeHub:
		if component.SubType == 1 {
			return "usb_hub_gen1"
		}
		return "usb_hub_gen2"
	case dockDeviceTypeMST:
		return "mst"
	case dockDeviceTypeTBT:
		return "thunderbolt"
	default:
		return fmt.Sprintf("component_%d", component.DeviceType)
	}
}
