package update

import (
	"encoding/binary"
	"fmt"
)

const (
	hidReportSize          = 192
	hidMaxRead             = 192
	hidMaxWrite            = 128
	hidDataOffset          = 64
	dockDataSize           = 103
	dockInfoSize           = 0xb7
	hidCommandRead         = 0xc0
	hidCommandWrite        = 0x40
	hidExtensionStatus     = 0x09
	hidExtensionClock      = 0x06
	hidExtensionI2CWrite   = 0xc6
	hidExtensionWriteFlash = 0xc8
	hidExtensionI2CRead    = 0xd6
	hidExtensionVerify     = 0xd9
	hidExtensionErase      = 0xe8
	ecAddress              = 0xec
	ecCommandSetPackage    = 0x01
	ecCommandDockInfo      = 0x02
	ecCommandDockData      = 0x03
	ecCommandDockType      = 0x05
	ecCommandModifyLock    = 0x0a
	ecCommandReset         = 0x0b
	ecCommandPassive       = 0x0d
	ecCommandUpdateStatus  = 0x0f

	I2CSpeed250K byte = 0
	I2CSpeed400K byte = 1
	I2CSpeed800K byte = 2

	dockDeviceTypeEC  byte = 0
	dockDeviceTypePD  byte = 1
	dockDeviceTypeHub byte = 3
	dockDeviceTypeMST byte = 4
	dockDeviceTypeTBT byte = 5
)

type HIDReports interface {
	SetOutputReport([]byte) error
	GetInputReport([]byte) error
}

type I2CSettings struct {
	Target         byte
	RegisterLength byte
	Speed          byte
}

type DellHID struct {
	Reports HIDReports
}

type DockData struct {
	DockConfiguration  byte
	DockType           byte
	PowerSupplyWattage uint16
	ModuleType         uint16
	BoardID            uint16
	Port0Status        uint16
	Port1Status        uint16
	PackageVersion     string
	ModuleSerial       uint64
	OriginalSerial     uint64
	ServiceTag         string
	MarketingName      string
}

type DockComponent struct {
	Location   byte
	DeviceType byte
	SubType    byte
	Argument   byte
	Instance   byte
	Version    string
}

func (d DellHID) ReadI2C(command uint32, size int, settings I2CSettings) ([]byte, error) {
	if d.Reports == nil {
		return nil, fmt.Errorf("HID report device is not configured")
	}
	if size < 0 || size > hidMaxRead {
		return nil, fmt.Errorf("I2C read size %d is outside 0..%d", size, hidMaxRead)
	}
	if settings.RegisterLength >= 4 {
		return nil, fmt.Errorf("I2C register length %d is invalid", settings.RegisterLength)
	}
	packet := newHIDPacket(hidCommandWrite, hidExtensionI2CRead)
	binary.LittleEndian.PutUint32(packet[2:6], command)
	binary.LittleEndian.PutUint16(packet[6:8], uint16(size))
	packet[8] = settings.Target
	packet[9] = settings.RegisterLength
	packet[10] = settings.Speed | 0x80
	if err := d.Reports.SetOutputReport(packet); err != nil {
		return nil, fmt.Errorf("set HID output report: %w", err)
	}
	result := make([]byte, hidReportSize)
	if err := d.Reports.GetInputReport(result); err != nil {
		return nil, fmt.Errorf("get HID input report: %w", err)
	}
	return append([]byte(nil), result[:size]...), nil
}

func (d DellHID) WriteI2C(data []byte, settings I2CSettings) error {
	if len(data) == 0 || len(data) > hidMaxWrite {
		return fmt.Errorf("I2C write size %d is outside 1..%d", len(data), hidMaxWrite)
	}
	packet := newHIDPacket(hidCommandWrite, hidExtensionI2CWrite)
	binary.LittleEndian.PutUint16(packet[6:8], uint16(len(data)))
	packet[8] = settings.Target
	packet[10] = settings.Speed | 0x80
	copy(packet[hidDataOffset:], data)
	return d.Reports.SetOutputReport(packet)
}

func (d DellHID) ReadHubVersion() (string, error) {
	packet := newHIDPacket(hidCommandRead, hidExtensionStatus)
	binary.LittleEndian.PutUint16(packet[6:8], 12)
	if err := d.Reports.SetOutputReport(packet); err != nil {
		return "", fmt.Errorf("set hub version report: %w", err)
	}
	result := make([]byte, hidReportSize)
	if err := d.Reports.GetInputReport(result); err != nil {
		return "", fmt.Errorf("get hub version report: %w", err)
	}
	return fmt.Sprintf("%02x.%02x", result[10], result[11]), nil
}

func (d DellHID) ReadDockType() (byte, error) {
	data, err := d.readEC(ecCommandDockType, 1)
	if err != nil {
		return 0, err
	}
	return data[0], nil
}

func (d DellHID) ReadDockData() (DockData, error) {
	data, err := d.readEC(ecCommandDockData, dockDataSize)
	if err != nil {
		return DockData{}, err
	}
	return DockData{
		DockConfiguration:  data[0],
		DockType:           data[1],
		PowerSupplyWattage: binary.LittleEndian.Uint16(data[2:4]),
		ModuleType:         binary.LittleEndian.Uint16(data[4:6]),
		BoardID:            binary.LittleEndian.Uint16(data[6:8]),
		Port0Status:        binary.LittleEndian.Uint16(data[8:10]),
		Port1Status:        binary.LittleEndian.Uint16(data[10:12]),
		PackageVersion:     bcdQuad(binary.LittleEndian.Uint32(data[12:16])),
		ModuleSerial:       binary.LittleEndian.Uint64(data[16:24]),
		OriginalSerial:     binary.LittleEndian.Uint64(data[24:32]),
		ServiceTag:         cString(data[32:39]),
		MarketingName:      cString(data[39:103]),
	}, nil
}

func (d DellHID) ReadDockInfo() ([]DockComponent, error) {
	data, err := d.readEC(ecCommandDockInfo, dockInfoSize)
	if err != nil {
		return nil, err
	}
	if len(data) < 3 {
		return nil, fmt.Errorf("dock info response is too short")
	}
	total := int(data[0])
	if 3+total*9 > len(data) {
		return nil, fmt.Errorf("dock info lists %d components beyond response size", total)
	}
	components := make([]DockComponent, 0, total)
	for index := 0; index < total; index++ {
		entry := data[3+index*9 : 12+index*9]
		components = append(components, DockComponent{
			Location:   entry[0],
			DeviceType: entry[1],
			SubType:    entry[2],
			Argument:   entry[3],
			Instance:   entry[4],
			Version:    bcdQuad(binary.LittleEndian.Uint32(entry[5:9])),
		})
	}
	return components, nil
}

func (d DellHID) ReadUpdateStatus() (byte, error) {
	data, err := d.readEC(ecCommandUpdateStatus, 1)
	if err != nil {
		return 0, err
	}
	return data[0], nil
}

func (d DellHID) ModifyLock(target byte, unlocked bool) error {
	command := uint32(ecCommandModifyLock) | uint32(2)<<8 | uint32(target)<<16
	if unlocked {
		command |= uint32(1) << 24
	}
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, command)
	return d.WriteI2C(data, ecSettings())
}

func (d DellHID) ResetDock() error {
	return d.WriteI2C([]byte{ecCommandReset, 0}, ecSettings())
}

func (d DellHID) RebootPassive(flow byte) error {
	command := uint32(ecCommandPassive) | uint32(1)<<8 | uint32(flow)<<16
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, command)
	return d.WriteI2C(data[:3], ecSettings())
}

func (d DellHID) RaiseMcuClock(enabled bool) error {
	packet := newHIDPacket(hidCommandWrite, hidExtensionClock)
	if enabled {
		packet[2] = 1
	}
	return d.Reports.SetOutputReport(packet)
}

func (d DellHID) EraseBank(index byte) error {
	packet := newHIDPacket(hidCommandWrite, hidExtensionErase)
	packet[3] = index
	return d.Reports.SetOutputReport(packet)
}

func (d DellHID) WriteFlash(address uint32, data []byte) error {
	if len(data) == 0 || len(data) > hidMaxWrite {
		return fmt.Errorf("flash write size %d is outside 1..%d", len(data), hidMaxWrite)
	}
	packet := newHIDPacket(hidCommandWrite, hidExtensionWriteFlash)
	binary.LittleEndian.PutUint32(packet[2:6], address)
	binary.LittleEndian.PutUint16(packet[6:8], uint16(len(data)))
	copy(packet[hidDataOffset:], data)
	return d.Reports.SetOutputReport(packet)
}

func (d DellHID) VerifyUpdate() (bool, error) {
	packet := newHIDPacket(hidCommandWrite, hidExtensionVerify)
	packet[2] = 1
	binary.LittleEndian.PutUint16(packet[6:8], 1)
	if err := d.Reports.SetOutputReport(packet); err != nil {
		return false, err
	}
	result := make([]byte, hidReportSize)
	if err := d.Reports.GetInputReport(result); err != nil {
		return false, err
	}
	return result[0] != 0, nil
}

func (d DellHID) CommitPackage(data []byte) error {
	if len(data) != 24 {
		return fmt.Errorf("dock package size %d, want 24", len(data))
	}
	payload := make([]byte, len(data)+2)
	payload[0] = ecCommandSetPackage
	payload[1] = byte(len(data))
	copy(payload[2:], data)
	return d.WriteI2C(payload, ecSettings())
}

func (d DellHID) readEC(command byte, size int) ([]byte, error) {
	data, err := d.ReadI2C(uint32(command), size+1, ecSettings())
	if err != nil {
		return nil, fmt.Errorf("read EC command %#x: %w", command, err)
	}
	if len(data) != size+1 || int(data[0]) != size {
		return nil, fmt.Errorf("EC command %#x returned length %d, want %d", command, data[0], size)
	}
	return append([]byte(nil), data[1:]...), nil
}

func ecSettings() I2CSettings {
	return I2CSettings{Target: ecAddress, RegisterLength: 1, Speed: I2CSpeed250K}
}

func newHIDPacket(command, extension byte) []byte {
	packet := make([]byte, hidReportSize)
	packet[0] = command
	packet[1] = extension
	return packet
}

func bcdQuad(value uint32) string {
	return fmt.Sprintf("%02x.%02x.%02x.%02x", byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
}

func cString(value []byte) string {
	for index, character := range value {
		if character == 0 {
			value = value[:index]
			break
		}
	}
	return string(value)
}
