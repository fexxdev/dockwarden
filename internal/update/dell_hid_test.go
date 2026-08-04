package update

import (
	"encoding/binary"
	"errors"
	"testing"
)

type fakeHIDReportDevice struct {
	outputs [][]byte
	inputs  [][]byte
}

func (f *fakeHIDReportDevice) SetOutputReport(report []byte) error {
	f.outputs = append(f.outputs, append([]byte(nil), report...))
	return nil
}

func (f *fakeHIDReportDevice) GetInputReport(report []byte) error {
	if len(f.inputs) == 0 {
		return errors.New("no fake HID input report")
	}
	input := f.inputs[0]
	f.inputs = f.inputs[1:]
	copy(report, input)
	return nil
}

func TestDellHIDReadI2CBuildsFwupdPacket(t *testing.T) {
	fake := &fakeHIDReportDevice{inputs: [][]byte{{0x04}}}
	device := DellHID{Reports: fake}

	data, err := device.ReadI2C(0x05, 1, I2CSettings{Target: 0xec, RegisterLength: 1, Speed: I2CSpeed250K})
	if err != nil {
		t.Fatalf("ReadI2C() error = %v", err)
	}
	if got, want := data, []byte{0x04}; string(got) != string(want) {
		t.Fatalf("ReadI2C() = %x, want %x", got, want)
	}
	if len(fake.outputs) != 1 {
		t.Fatalf("SetOutputReport() calls = %d, want 1", len(fake.outputs))
	}
	packet := fake.outputs[0]
	if len(packet) != hidReportSize {
		t.Fatalf("packet length = %d, want %d", len(packet), hidReportSize)
	}
	if packet[0] != hidCommandWrite || packet[1] != hidExtensionI2CRead {
		t.Fatalf("packet command = %02x %02x, want %02x %02x", packet[0], packet[1], hidCommandWrite, hidExtensionI2CRead)
	}
	if got := binary.LittleEndian.Uint32(packet[2:6]); got != 0x05 {
		t.Fatalf("packet register = %#x, want %#x", got, uint32(0x05))
	}
	if got := binary.LittleEndian.Uint16(packet[6:8]); got != 1 {
		t.Fatalf("packet length = %d, want 1", got)
	}
	if got, want := packet[8:11], []byte{0xec, 1, 0x80}; string(got) != string(want) {
		t.Fatalf("packet parameters = %x, want %x", got, want)
	}
}

func TestDellHIDParsesDockDataAndInfo(t *testing.T) {
	dockData := make([]byte, dockDataSize)
	dockData[0] = 0x02
	dockData[1] = 0x04
	binary.LittleEndian.PutUint16(dockData[2:4], 130)
	binary.LittleEndian.PutUint16(dockData[4:6], 3)
	binary.LittleEndian.PutUint16(dockData[6:8], 6)
	binary.LittleEndian.PutUint32(dockData[12:16], 0x01000101)
	copy(dockData[32:39], "ABC1234")
	copy(dockData[39:103], "Dell Dock WD19")

	info := make([]byte, 0xb7)
	info[0] = 2
	info[1] = 3
	info[2] = 4
	info[3] = 0
	info[4] = 3
	info[5] = 1
	info[6] = 0
	copy(info[7:11], []byte{0x01, 0x23, 0x00, 0x00})
	info[12] = 1
	info[13] = 3
	info[14] = 0
	info[15] = 0
	info[16] = 0
	copy(info[17:21], []byte{0x01, 0x62, 0x00, 0x00})

	fake := &fakeHIDReportDevice{
		inputs: [][]byte{
			append([]byte{byte(len(dockData))}, dockData...),
			append([]byte{byte(len(info))}, info...),
		},
	}
	device := DellHID{Reports: fake}

	data, err := device.ReadDockData()
	if err != nil {
		t.Fatalf("ReadDockData() error = %v", err)
	}
	if data.DockType != 0x04 || data.BoardID != 6 || data.PowerSupplyWattage != 130 {
		t.Fatalf("unexpected dock data: %+v", data)
	}
	if data.PackageVersion != "01.01.00.01" || data.MarketingName != "Dell Dock WD19" || data.ServiceTag != "ABC1234" {
		t.Fatalf("unexpected dock identity: %+v", data)
	}

	components, err := device.ReadDockInfo()
	if err != nil {
		t.Fatalf("ReadDockInfo() error = %v", err)
	}
	if len(components) != 2 || components[0].DeviceType != dockDeviceTypeHub || components[1].Version != "01.62.00.00" {
		t.Fatalf("unexpected dock info: %+v", components)
	}
}

func TestDellHIDWriteFlashUses128ByteChunks(t *testing.T) {
	fake := &fakeHIDReportDevice{}
	device := DellHID{Reports: fake}
	data := make([]byte, hidMaxWrite)
	for index := range data {
		data[index] = byte(index)
	}

	if err := device.WriteFlash(0xff000000, data); err != nil {
		t.Fatalf("WriteFlash() error = %v", err)
	}
	if len(fake.outputs) != 1 {
		t.Fatalf("SetOutputReport() calls = %d, want 1", len(fake.outputs))
	}
	packet := fake.outputs[0]
	if packet[0] != hidCommandWrite || packet[1] != hidExtensionWriteFlash {
		t.Fatalf("packet command = %02x %02x", packet[0], packet[1])
	}
	if got := binary.LittleEndian.Uint32(packet[2:6]); got != 0xff000000 {
		t.Fatalf("packet address = %#x, want %#x", got, uint32(0xff000000))
	}
	if got := binary.LittleEndian.Uint16(packet[6:8]); got != hidMaxWrite {
		t.Fatalf("packet length = %d, want %d", got, hidMaxWrite)
	}
	if got, want := packet[hidDataOffset:hidDataOffset+hidMaxWrite], data; string(got) != string(want) {
		t.Fatalf("packet data mismatch")
	}
}
