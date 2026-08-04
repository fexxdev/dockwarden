package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/fexxdev/dockwarden/internal/domain"
)

func RenderText(w io.Writer, report domain.Report, verbose bool) error {
	var text strings.Builder
	fmt.Fprintf(&text, "dockwarden %s\n", report.Command)
	fmt.Fprintf(&text, "State: %s\n", report.State)
	if report.Platform != "" {
		fmt.Fprintf(&text, "Platform: %s\n", report.Platform)
	}
	text.WriteString("Functionality: host-observable enumeration only; this does not prove charging, display signal, link speed, or audio quality.\n")

	if report.Dock == nil {
		text.WriteString("Dock: not detected\n")
	} else {
		dock := report.Dock
		fmt.Fprintf(&text, "Dock: %s\n", dock.Model)
		if dock.Manufacturer != "" {
			fmt.Fprintf(&text, "Manufacturer: %s\n", dock.Manufacturer)
		}
		fmt.Fprintf(&text, "USB ID: %04x:%04x\n", dock.VendorID, dock.ProductID)
		if dock.Serial != "" {
			fmt.Fprintf(&text, "Serial: %s\n", dock.Serial)
		}
		if dock.DescriptorVersion != "" {
			fmt.Fprintf(&text, "Descriptor version: %s\n", dock.DescriptorVersion)
		}
		if dock.FirmwareVersion == "" && len(dock.Firmware) == 0 {
			text.WriteString("Firmware version: unavailable\n")
		} else if dock.FirmwareVersion != "" {
			fmt.Fprintf(&text, "Firmware version: %s\n", dock.FirmwareVersion)
		} else {
			text.WriteString("Firmware version: component details below\n")
		}

		if len(dock.Services) > 0 {
			text.WriteString("Services:\n")
			for _, service := range dock.Services {
				fmt.Fprintf(&text, "  %s: %s", service.Name, service.State)
				if service.Evidence != "" {
					fmt.Fprintf(&text, " (%s)", service.Evidence)
				}
				text.WriteByte('\n')
			}
		}

		if verbose || report.Command == "scan" || report.Command == "doctor" {
			if len(dock.Devices) > 0 {
				text.WriteString("Components:\n")
				for _, device := range dock.Devices {
					fmt.Fprintf(&text, "  %s: %s", device.Kind, device.Product)
					if device.Vendor != "" {
						fmt.Fprintf(&text, " [%s]", device.Vendor)
					}
					text.WriteByte('\n')
				}
			}
		}
		if len(dock.Firmware) > 0 {
			text.WriteString("Firmware components:\n")
			for _, firmware := range dock.Firmware {
				fmt.Fprintf(&text, "  %s: %s (%s)\n", firmware.Component, firmware.Version, firmware.Source)
			}
		}
	}

	if len(report.Checks) > 0 {
		text.WriteString("Checks:\n")
		for _, check := range report.Checks {
			fmt.Fprintf(&text, "  %s: %s", check.Name, check.State)
			if check.Details != "" {
				fmt.Fprintf(&text, " (%s)", check.Details)
			}
			text.WriteByte('\n')
		}
	}
	if report.Update != nil {
		fmt.Fprintf(&text, "Updates: %s", report.Update.State)
		if report.Update.Reason != "" {
			fmt.Fprintf(&text, " (%s)", report.Update.Reason)
		}
		text.WriteByte('\n')
	}
	if len(report.Warnings) > 0 {
		text.WriteString("Warnings:\n")
		for _, warning := range report.Warnings {
			fmt.Fprintf(&text, "  - %s\n", warning)
		}
	}

	_, err := io.WriteString(w, text.String())
	return err
}
