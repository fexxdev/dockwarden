package update

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fakeHIDConnection struct {
	fakeHIDReportDevice
	closed bool
}

func (f *fakeHIDConnection) Close() {
	f.closed = true
}

type fakeFirmwareExtractor struct {
	blobs map[string][]byte
	calls []string
}

func (f *fakeFirmwareExtractor) Extract(_ context.Context, _ string, name string) ([]byte, error) {
	f.calls = append(f.calls, name)
	return append([]byte(nil), f.blobs[name]...), nil
}

func TestMacUpdaterUpdatesWD19ThroughHID(t *testing.T) {
	payload := []byte("verified Dell CAB")
	base := &fakeHIDConnection{
		fakeHIDReportDevice: fakeHIDReportDevice{inputs: wd19ReadOnlyInputs()},
	}
	extractor := &fakeFirmwareExtractor{blobs: wd19FirmwareBlobs()}
	client := &fakeHTTPDoer{response: (&httpResponseOK{body: payload}).response()}
	var opened []uint16

	updater := MacUpdater{
		HTTP:      client,
		TempDir:   t.TempDir(),
		Extractor: extractor,
		Open: func(productID uint16) (HIDConnection, error) {
			opened = append(opened, productID)
			return base, nil
		},
	}
	candidate := candidateFor(payload)
	result := updater.Apply(context.Background(), matchingDock(), &candidate)

	if result.State != "update_applied" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !strings.Contains(result.Reason, "EC 01.01.00.14") || !strings.Contains(result.Reason, "package 01.01.00.01") {
		t.Fatalf("update result does not list updated components: %s", result.Reason)
	}
	if len(opened) != 1 || opened[0] != 0xb06e {
		t.Fatalf("unexpected HID interfaces opened: %04x", opened)
	}
	if !base.closed {
		t.Fatal("base HID interface was not closed")
	}
	if len(extractor.calls) != 5 {
		t.Fatalf("extracted %d blobs, want all WD19 components: %v", len(extractor.calls), extractor.calls)
	}

	var flashWrites, packageWrites, passiveWrites int
	for _, report := range base.outputs {
		switch {
		case report[0] == hidCommandWrite && report[1] == hidExtensionWriteFlash:
			flashWrites++
		case report[0] == hidCommandWrite && report[1] == hidExtensionI2CWrite && report[hidDataOffset] == ecCommandSetPackage:
			packageWrites++
		case report[0] == hidCommandWrite && report[1] == hidExtensionI2CWrite && report[hidDataOffset] == ecCommandPassive:
			passiveWrites++
		}
	}
	if flashWrites != 1024 {
		t.Fatalf("EC flash writes = %d, want %d", flashWrites, 0x1ffc0/hidMaxWrite)
	}
	if packageWrites != 1 || passiveWrites != 1 {
		t.Fatalf("package writes = %d, passive writes = %d", packageWrites, passiveWrites)
	}
	last := base.outputs[len(base.outputs)-1]
	if got, want := last[hidDataOffset:hidDataOffset+3], []byte{ecCommandPassive, 1, 3}; !bytes.Equal(got, want) {
		t.Fatalf("passive flow = %x, want %x", got, want)
	}
}

func TestMacUpdaterRejectsNonCABBeforeOpeningHID(t *testing.T) {
	payload := []byte("payload")
	client := &fakeHTTPDoer{}
	opened := false
	updater := MacUpdater{
		HTTP:    client,
		TempDir: t.TempDir(),
		Open: func(productID uint16) (HIDConnection, error) {
			opened = true
			return nil, nil
		},
	}
	candidate := candidateFor(payload)
	candidate.DownloadURL = strings.TrimSuffix(candidate.DownloadURL, ".cab") + ".exe"
	result := updater.Apply(context.Background(), matchingDock(), &candidate)
	if result.State != "update_failed" || !strings.Contains(result.Reason, "CAB") {
		t.Fatalf("unexpected result: %+v", result)
	}
	if client.request != nil || opened {
		t.Fatal("invalid package must fail before download and HID access")
	}
}

func TestMacUpdaterDoesNotPartiallyFlashUnsupportedMST(t *testing.T) {
	payload := []byte("verified Dell CAB")
	base := &fakeHIDConnection{
		fakeHIDReportDevice: fakeHIDReportDevice{inputs: wd19ReadOnlyInputs()},
	}
	blobs := wd19FirmwareBlobs()
	blobs["vmm5331.bin"] = append([]byte(nil), blobs["vmm5331.bin"]...)
	blobs["vmm5331.bin"][0x18400] = 0x06
	extractor := &fakeFirmwareExtractor{blobs: blobs}
	client := &fakeHTTPDoer{response: (&httpResponseOK{body: payload}).response()}
	updater := MacUpdater{
		HTTP:      client,
		TempDir:   t.TempDir(),
		Extractor: extractor,
		Open: func(productID uint16) (HIDConnection, error) {
			return base, nil
		},
	}
	candidate := candidateFor(payload)
	result := updater.Apply(context.Background(), matchingDock(), &candidate)
	if result.State != "update_failed" || !strings.Contains(result.Reason, "MST") {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, report := range base.outputs {
		if report[1] == hidExtensionWriteFlash || report[1] == hidExtensionI2CWrite {
			t.Fatalf("unsupported MST candidate caused a write: %02x", report[1])
		}
	}
}

func TestMacUpdaterFlashesGen1HubThroughSecondHID(t *testing.T) {
	payload := []byte("verified Dell CAB")
	base := &fakeHIDConnection{
		fakeHIDReportDevice: fakeHIDReportDevice{inputs: wd19ReadOnlyInputs()},
	}
	gen1 := &fakeHIDConnection{
		fakeHIDReportDevice: fakeHIDReportDevice{inputs: [][]byte{{1}}},
	}
	blobs := wd19FirmwareBlobs()
	copy(blobs["ec.bin"][ecBlobVersionOffset:], "01.01.00.13")
	copy(blobs["salomon_package.bin"][0x14:], []byte{0x01, 0x00, 0x47, 0x01})
	blobs["rts5413.bin"][gen1VersionOffset] = 0x02
	blobs["rts5413.bin"][gen1VersionOffset+1] = 0x00
	extractor := &fakeFirmwareExtractor{blobs: blobs}
	client := &fakeHTTPDoer{response: (&httpResponseOK{body: payload}).response()}
	var opened []uint16
	updater := MacUpdater{
		HTTP:      client,
		TempDir:   t.TempDir(),
		Extractor: extractor,
		Open: func(productID uint16) (HIDConnection, error) {
			opened = append(opened, productID)
			if productID == wd19Gen1ProductID {
				return gen1, nil
			}
			return base, nil
		},
	}
	candidate := candidateFor(payload)
	result := updater.Apply(context.Background(), matchingDock(), &candidate)
	if result.State != "update_applied" || !strings.Contains(result.Reason, "USB hub Gen1 02.00") {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(opened) != 2 || opened[0] != wd19Gen2ProductID || opened[1] != wd19Gen1ProductID {
		t.Fatalf("unexpected HID interfaces opened: %04x", opened)
	}
	if !base.closed || !gen1.closed {
		t.Fatal("HID interfaces were not closed")
	}
	flashWrites := 0
	for _, report := range gen1.outputs {
		if report[0] == hidCommandWrite && report[1] == hidExtensionWriteFlash {
			flashWrites++
		}
	}
	if flashWrites != hubBlobSize/hidMaxWrite {
		t.Fatalf("Gen1 flash writes = %d, want %d", flashWrites, hubBlobSize/hidMaxWrite)
	}
}

type httpResponseOK struct {
	body []byte
}

func (r *httpResponseOK) response() *http.Response {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(r.body)),
	}
}

func wd19ReadOnlyInputs() [][]byte {
	dockData := make([]byte, dockDataSize)
	dockData[1] = 0x04
	binary.LittleEndian.PutUint16(dockData[2:4], 180)
	binary.LittleEndian.PutUint16(dockData[4:6], 4)
	binary.LittleEndian.PutUint16(dockData[6:8], 6)
	binary.LittleEndian.PutUint32(dockData[12:16], 0x01470001)
	copy(dockData[32:39], "5YVWRV2")
	copy(dockData[39:103], "WD19")

	info := make([]byte, dockInfoSize)
	info[0] = 7
	info[1] = 1
	info[2] = 7
	setDockInfoEntry(info, 0, 0, dockDeviceTypeEC, 0, []byte{0x01, 0x01, 0x00, 0x13})
	setDockInfoEntry(info, 1, 0, dockDeviceTypePD, 0, []byte{0x00, 0x00, 0x00, 0x29})
	setDockInfoEntry(info, 2, 1, dockDeviceTypePD, 0, []byte{0x00, 0x00, 0x00, 0x08})
	setDockInfoEntry(info, 3, 0, dockDeviceTypePD, 0, []byte{0x00, 0x00, 0x00, 0x29})
	setDockInfoEntry(info, 4, 0, dockDeviceTypeHub, 0, []byte{0x00, 0x00, 0x01, 0x62})
	setDockInfoEntry(info, 5, 0, dockDeviceTypeHub, 1, []byte{0x00, 0x00, 0x01, 0x23})
	setDockInfoEntry(info, 6, 0, dockDeviceTypeMST, 1, []byte{0x00, 0x05, 0x07, 0x08})

	return [][]byte{
		append([]byte{byte(len(dockData))}, dockData...),
		append([]byte{byte(len(info))}, info...),
		{1, 1},
	}
}

func setDockInfoEntry(info []byte, index int, location, deviceType, subType byte, version []byte) {
	entry := info[3+index*9 : 12+index*9]
	entry[0] = location
	entry[1] = deviceType
	entry[2] = subType
	copy(entry[5:9], version)
}

func wd19FirmwareBlobs() map[string][]byte {
	ec := make([]byte, 0x1ffc0)
	copy(ec[0x1afc0:], "01.01.00.14")
	packageBlob := make([]byte, 24)
	copy(packageBlob[0x14:], []byte{0x01, 0x01, 0x00, 0x01})

	gen1 := make([]byte, 0x10000)
	gen1[0x7f6e] = 0x01
	gen1[0x7f6f] = 0x23
	gen2 := make([]byte, 0x10000)
	gen2[0x7f52] = 0x01
	gen2[0x7f53] = 0x62
	mst := make([]byte, 0x80000)
	mst[0x18400] = 0x05
	mst[0x18401] = 0x07
	mst[0x18402] = 0x08

	return map[string][]byte{
		"ec.bin":              ec,
		"salomon_package.bin": packageBlob,
		"rts5413.bin":         gen1,
		"rts5487.bin":         gen2,
		"vmm5331.bin":         mst,
	}
}
