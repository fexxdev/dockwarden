package discovery

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/fexxdev/dockwarden/internal/domain"
)

var ioregNodePattern = regexp.MustCompile("\\+-o\\s+(.+?)\\s+<class\\s+([^,>]+)")

type ioregNode struct {
	name         string
	class        string
	depth        int
	location     string
	product      string
	vendor       string
	serial       string
	vendorID     uint16
	productID    uint16
	bcdDevice    uint64
	hasBCDDevice bool
}

func ParseIORegistry(input string) ([]domain.USBDevice, error) {
	var devices []domain.USBDevice
	var current *ioregNode

	finish := func() {
		if current == nil || !strings.Contains(current.class, "IOUSBHostDevice") {
			return
		}

		product := current.product
		if product == "" {
			product = current.name
		}
		vendor := current.vendor
		if vendor == "" && strings.Contains(strings.ToLower(product), "dell") {
			vendor = "Dell Inc."
		}

		device := domain.USBDevice{
			Name:      current.name,
			Vendor:    vendor,
			Product:   product,
			Class:     current.class,
			Serial:    current.serial,
			Location:  current.location,
			VendorID:  current.vendorID,
			ProductID: current.productID,
			Depth:     current.depth,
		}
		if current.hasBCDDevice {
			device.DescriptorVersion = formatBCD(current.bcdDevice)
		}
		devices = append(devices, device)
	}

	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if node, ok := parseIORegistryNode(line); ok {
			finish()
			current = node
			continue
		}
		if current != nil {
			applyIORegistryProperty(current, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	finish()

	return devices, nil
}

func parseIORegistryNode(line string) (*ioregNode, bool) {
	matches := ioregNodePattern.FindStringSubmatch(line)
	if len(matches) != 3 {
		return nil, false
	}

	name, location := splitIORegistryName(strings.TrimSpace(matches[1]))
	marker := strings.Index(line, "+-o")
	return &ioregNode{
		name:     name,
		class:    strings.TrimSpace(matches[2]),
		depth:    marker / 2,
		location: location,
	}, true
}

func splitIORegistryName(value string) (string, string) {
	at := strings.LastIndex(value, "@")
	if at < 0 || at == len(value)-1 {
		return value, ""
	}
	return strings.TrimSpace(value[:at]), strings.TrimSpace(value[at+1:])
}

func applyIORegistryProperty(node *ioregNode, line string) {
	equal := strings.IndexByte(line, '=')
	if equal < 0 {
		return
	}

	key := strings.TrimSpace(line[:equal])
	key = strings.Trim(key, "| ")
	key = strings.Trim(key, "\"")
	value := parseIORegistryValue(line[equal+1:])

	switch key {
	case "USB Product Name":
		node.product = value
	case "USB Vendor Name":
		node.vendor = value
	case "USB Serial Number":
		node.serial = value
	case "idVendor":
		if number, ok := parseUnsigned(value); ok {
			node.vendorID = uint16(number)
		}
	case "idProduct":
		if number, ok := parseUnsigned(value); ok {
			node.productID = uint16(number)
		}
	case "bcdDevice":
		if number, ok := parseUnsigned(value); ok {
			node.bcdDevice = number
			node.hasBCDDevice = true
		}
	}
}

func parseIORegistryValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
		return strings.Trim(value, "\"")
	}
	return value
}

func parseUnsigned(value string) (uint64, bool) {
	value = strings.TrimSpace(value)
	base := 10
	if strings.HasPrefix(strings.ToLower(value), "0x") {
		value = value[2:]
		base = 16
	}
	number, err := strconv.ParseUint(value, base, 64)
	return number, err == nil
}

func formatBCD(value uint64) string {
	majorByte := byte(value >> 8)
	minorByte := byte(value)
	major := int(majorByte>>4)*10 + int(majorByte&0x0f)
	minor := int(minorByte>>4)*10 + int(minorByte&0x0f)
	if minor == 0 {
		return fmt.Sprintf("%d.0", major)
	}
	return fmt.Sprintf("%d.%02d", major, minor)
}
