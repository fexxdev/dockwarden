package discovery

import (
	"strings"

	"github.com/fexxdev/dockwarden/internal/domain"
)

func BuildReport(platform, command string, devices []domain.USBDevice) domain.Report {
	report := domain.Report{
		SchemaVersion: 1,
		Platform:      platform,
		Command:       command,
		Checks:        []domain.Check{},
		Warnings:      []string{},
	}
	dock, state := Identify(devices)
	report.State = state
	if dock == nil {
		return report
	}

	rootDepth := 0
	for _, device := range devices {
		if isWD19(device) {
			rootDepth = device.Depth
			break
		}
	}

	selected := selectDockDevices(devices)
	byLocation := indexDevices(devices)
	dock.Devices = make([]domain.USBDevice, 0, len(selected))
	for _, device := range selected {
		device.Kind = classifyDevice(device, rootDepth)
		if device.Kind == "dock_component" &&
			!isDockComponent(device) &&
			hasDockAncestor(device, byLocation, map[string]bool{}) {
			device.Kind = "downstream_usb"
		}
		dock.Devices = append(dock.Devices, device)
	}
	dock.Services = buildServices(dock.Devices)
	report.Dock = dock
	report.Warnings = append(report.Warnings,
		"firmware version unavailable from USB descriptors",
	)
	return report
}

func classifyDevice(device domain.USBDevice, rootDepth int) string {
	if isWD19(device) {
		return "dock"
	}

	text := strings.ToLower(strings.Join([]string{
		device.Name,
		device.Product,
		device.Vendor,
	}, " "))
	switch {
	case strings.Contains(text, "ethernet"),
		strings.Contains(text, "lan"),
		strings.Contains(text, "realtek"):
		return "ethernet"
	case strings.Contains(text, "audio"):
		return "audio"
	case isDockComponent(device):
		return "dock_component"
	case device.Depth > rootDepth:
		return "downstream_usb"
	default:
		return "dock_component"
	}
}

func isDockComponent(device domain.USBDevice) bool {
	text := strings.ToLower(strings.Join([]string{device.Name, device.Product}, " "))
	return strings.Contains(text, "dell dock")
}

func selectDockDevices(devices []domain.USBDevice) []domain.USBDevice {
	hasTopology := false
	byLocation := indexDevices(devices)
	for _, device := range devices {
		if device.ParentLocation != "" {
			hasTopology = true
		}
	}
	if !hasTopology {
		return append([]domain.USBDevice(nil), devices...)
	}

	selected := make([]domain.USBDevice, 0, len(devices))
	for _, device := range devices {
		if dockDeviceRelated(device, byLocation, map[string]bool{}) {
			selected = append(selected, device)
		}
	}
	return selected
}

func indexDevices(devices []domain.USBDevice) map[string]domain.USBDevice {
	byLocation := make(map[string]domain.USBDevice)
	for _, device := range devices {
		if device.Location != "" {
			byLocation[device.Location] = device
		}
	}
	return byLocation
}

func dockDeviceRelated(device domain.USBDevice, byLocation map[string]domain.USBDevice, visiting map[string]bool) bool {
	if isWD19(device) || isDockComponent(device) {
		return true
	}
	if device.ParentLocation == "" {
		return false
	}
	if device.Location != "" {
		if visiting[device.Location] {
			return false
		}
		visiting[device.Location] = true
	}
	parent, ok := byLocation[device.ParentLocation]
	if !ok {
		return false
	}
	return dockDeviceRelated(parent, byLocation, visiting)
}

func hasDockAncestor(device domain.USBDevice, byLocation map[string]domain.USBDevice, visiting map[string]bool) bool {
	if device.ParentLocation == "" {
		return false
	}
	if device.Location != "" {
		if visiting[device.Location] {
			return false
		}
		visiting[device.Location] = true
	}
	parent, ok := byLocation[device.ParentLocation]
	if !ok {
		return false
	}
	if isWD19(parent) || isDockComponent(parent) {
		return true
	}
	return hasDockAncestor(parent, byLocation, visiting)
}

func buildServices(devices []domain.USBDevice) []domain.ServiceObservation {
	hasRoot := false
	hasEthernet := false
	hasAudio := false
	hasDownstream := false
	for _, device := range devices {
		switch device.Kind {
		case "dock":
			hasRoot = true
		case "ethernet":
			hasEthernet = true
		case "audio":
			hasAudio = true
		case "downstream_usb":
			hasDownstream = true
		}
	}

	return []domain.ServiceObservation{
		{
			Name:     "usb",
			State:    serviceState(hasRoot),
			Evidence: serviceEvidence(devices, "dock"),
		},
		{
			Name:     "ethernet",
			State:    serviceState(hasEthernet),
			Evidence: serviceEvidence(devices, "ethernet"),
		},
		{
			Name:     "audio",
			State:    serviceState(hasAudio),
			Evidence: serviceEvidence(devices, "audio"),
		},
		{
			Name:     "downstream_usb",
			State:    serviceState(hasDownstream),
			Evidence: serviceEvidence(devices, "downstream_usb"),
		},
	}
}

func serviceState(present bool) string {
	if present {
		return "pass"
	}
	return "missing"
}

func serviceEvidence(devices []domain.USBDevice, kind string) string {
	var names []string
	for _, device := range devices {
		if device.Kind == kind {
			names = append(names, device.Product)
		}
	}
	return strings.Join(names, ", ")
}
