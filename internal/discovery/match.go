package discovery

import (
	"strings"

	"github.com/fexxdev/dockwarden/internal/domain"
)

const (
	dellVendorID  uint16 = 0x413c
	wd19ProductID uint16 = 0xb06e
)

func Identify(devices []domain.USBDevice) (*domain.Dock, string) {
	if len(devices) == 0 {
		return nil, "no_dock"
	}

	for _, device := range devices {
		if isWD19(device) {
			return dockFromDevice(device), "detected"
		}
	}

	for _, device := range devices {
		if isDellDevice(device) {
			return nil, "unknown_dell_device"
		}
	}
	return nil, "no_dock"
}

func isWD19(device domain.USBDevice) bool {
	product := strings.ToLower(strings.TrimSpace(device.Product))
	name := strings.ToLower(strings.TrimSpace(device.Name))
	return (device.VendorID == dellVendorID && device.ProductID == wd19ProductID) ||
		product == "dell dock wd19" ||
		name == "dell dock wd19"
}

func isDellDevice(device domain.USBDevice) bool {
	return device.VendorID == dellVendorID ||
		strings.Contains(strings.ToLower(device.Vendor), "dell")
}

func dockFromDevice(device domain.USBDevice) *domain.Dock {
	manufacturer := device.Vendor
	if manufacturer == "" {
		manufacturer = "Dell Inc."
	}
	return &domain.Dock{
		Manufacturer:      manufacturer,
		Model:             "Dell Dock WD19",
		VendorID:          device.VendorID,
		ProductID:         device.ProductID,
		Serial:            device.Serial,
		DescriptorVersion: device.DescriptorVersion,
		Devices:           []domain.USBDevice{},
		Services:          []domain.ServiceObservation{},
		Firmware:          []domain.FirmwareObservation{},
	}
}
