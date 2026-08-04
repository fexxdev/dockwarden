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

	dock.Devices = make([]domain.USBDevice, 0, len(devices))
	for _, device := range devices {
		device.Kind = classifyDevice(device, rootDepth)
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
	case device.Depth > rootDepth:
		return "downstream_usb"
	default:
		return "dock_component"
	}
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
