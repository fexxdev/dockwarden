package discovery

import (
	"bufio"
	"regexp"
	"strings"

	"github.com/fexxdev/dockwarden/internal/domain"
)

var lsusbDevicePattern = regexp.MustCompile("ID\\s+([0-9a-fA-F]{4}):([0-9a-fA-F]{4})\\s+(.+)$")

func ParseLsusb(input string) ([]domain.USBDevice, error) {
	var devices []domain.USBDevice

	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		matches := lsusbDevicePattern.FindStringSubmatch(scanner.Text())
		if len(matches) != 4 {
			continue
		}

		vendorID, ok := parseHexUSBID(matches[1])
		if !ok {
			continue
		}
		productID, ok := parseHexUSBID(matches[2])
		if !ok {
			continue
		}
		vendor, product := splitLsusbDescription(strings.TrimSpace(matches[3]))
		devices = append(devices, domain.USBDevice{
			Name:      product,
			Vendor:    vendor,
			Product:   product,
			Class:     "lsusb",
			VendorID:  vendorID,
			ProductID: productID,
			Depth:     0,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return devices, nil
}

func parseHexUSBID(value string) (uint16, bool) {
	number, ok := parseUnsigned("0x" + value)
	return uint16(number), ok && number <= 0xffff
}

func splitLsusbDescription(description string) (string, string) {
	for _, vendor := range []string{
		"Dell Inc.",
		"Realtek Semiconductor Corp.",
		"Realtek",
		"Generalplus Technology Inc.",
	} {
		prefix := vendor + " "
		if strings.HasPrefix(description, prefix) {
			return vendor, strings.TrimSpace(strings.TrimPrefix(description, prefix))
		}
	}

	fields := strings.Fields(description)
	if len(fields) == 0 {
		return "", ""
	}
	return fields[0], strings.TrimSpace(strings.TrimPrefix(description, fields[0]))
}
