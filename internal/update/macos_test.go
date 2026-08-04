package update

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/fexxdev/dockwarden/internal/domain"
)

type fakeHIDConnection struct {
	fakeHIDReportDevice
	closed bool
}

type failFlashHIDConnection struct {
	fakeHIDConnection
}

func (f *failFlashHIDConnection) SetOutputReport(report []byte) error {
	f.outputs = append(f.outputs, append([]byte(nil), report...))
	if report[0] == hidCommandWrite && report[1] == hidExtensionWriteFlash {
		return errors.New("simulated flash failure")
	}
	return nil
}

func (f *fakeHIDConnection) Close() {
	f.closed = true
}

type fakeFirmwareExtractor struct {
	blobs map[string][]byte
	calls []string
}

func TestBsdtarExtractorReadsRootCABMember(t *testing.T) {
	runner := &fakeCommandRunner{output: []byte("member")}
	extractor := BsdtarExtractor{Runner: runner}
	got, err := extractor.Extract(context.Background(), "firmware.cab", "ec.bin")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "member" {
		t.Fatalf("extracted member = %q, want %q", got, "member")
	}
	if len(runner.calls) != 1 || len(runner.calls[0]) != 4 || runner.calls[0][0] != "/usr/bin/bsdtar" || runner.calls[0][1] != "-xOf" || runner.calls[0][2] != "firmware.cab" || runner.calls[0][3] != "ec.bin" {
		t.Fatalf("unexpected bsdtar call: %v", runner.calls)
	}
}

func TestHIDTargetForProductUsesDockTopology(t *testing.T) {
	target, err := hidTargetForProduct(matchingDock(), wd19Gen1ProductID)
	if err != nil {
		t.Fatalf("hidTargetForProduct() error = %v", err)
	}
	if target.ProductID != wd19Gen1ProductID || target.LocationID != 0x00135000 {
		t.Fatalf("unexpected target: %+v", target)
	}

	target, err = hidTargetForProduct(matchingDock(), wd19Gen2ProductID)
	if err != nil {
		t.Fatalf("base hidTargetForProduct() error = %v", err)
	}
	if target.Serial != "2000" || target.LocationID != 0x00150000 {
		t.Fatalf("base target is not bound to the detected dock: %+v", target)
	}
}

func TestHIDTargetForProductRejectsAmbiguousTopology(t *testing.T) {
	dock := matchingDock()
	dock.Devices = append(dock.Devices, domain.USBDevice{
		Product:   "Dell dock",
		Vendor:    "Dell Inc.",
		VendorID:  0x413c,
		ProductID: wd19Gen1ProductID,
		Location:  "00145000",
	})
	if _, err := hidTargetForProduct(dock, wd19Gen1ProductID); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous HID target rejection, got %v", err)
	}
}

func TestMacPreflightReadsSafeWD19WithoutWrites(t *testing.T) {
	connection := &fakeHIDConnection{
		fakeHIDReportDevice: fakeHIDReportDevice{inputs: wd19ReadOnlyInputs()},
	}
	result, err := (MacPreflightReader{
		Open:      func(domain.HIDTarget) (HIDConnection, error) { return connection, nil },
		Extractor: &fakeFirmwareExtractor{blobs: wd19FirmwareBlobs()},
	}).Check(context.Background(), matchingDock(), "firmware.cab")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.ServiceTag != "TST0001" || result.ModuleSerial != 2000 || !result.UpdateAvailable {
		t.Fatalf("unexpected preflight result: %+v", result)
	}
	if !connection.closed {
		t.Fatal("preflight did not close the HID connection")
	}
	assertNoFirmwareWrites(t, connection.outputs)
}

func TestMacPreflightRejectsUnsafeAndPendingWD19WithoutWrites(t *testing.T) {
	for _, test := range []struct {
		name   string
		inputs [][]byte
	}{
		{name: "unsafe board", inputs: wd19ReadOnlyInputsFor(5, 180, "01.01.00.13")},
		{name: "pending update", inputs: func() [][]byte {
			inputs := wd19ReadOnlyInputs()
			inputs[2][1] = firmwareUpdatePending
			return inputs
		}()},
	} {
		t.Run(test.name, func(t *testing.T) {
			connection := &fakeHIDConnection{
				fakeHIDReportDevice: fakeHIDReportDevice{inputs: test.inputs},
			}
			_, err := (MacPreflightReader{
				Open:      func(domain.HIDTarget) (HIDConnection, error) { return connection, nil },
				Extractor: &fakeFirmwareExtractor{blobs: wd19FirmwareBlobs()},
			}).Check(context.Background(), matchingDock(), "firmware.cab")
			if err == nil {
				t.Fatal("unsafe preflight was accepted")
			}
			assertNoFirmwareWrites(t, connection.outputs)
		})
	}
}

func TestMacPreflightReportsNoUpdateWithoutWrites(t *testing.T) {
	blobs := wd19FirmwareBlobs()
	copy(blobs["ec.bin"][ecBlobVersionOffset:], "01.01.00.13")
	copy(blobs["salomon_package.bin"][0x14:], []byte{0x01, 0x00, 0x47, 0x01})
	connection := &fakeHIDConnection{
		fakeHIDReportDevice: fakeHIDReportDevice{inputs: wd19ReadOnlyInputs()},
	}
	result, err := (MacPreflightReader{
		Open:      func(domain.HIDTarget) (HIDConnection, error) { return connection, nil },
		Extractor: &fakeFirmwareExtractor{blobs: blobs},
	}).Check(context.Background(), matchingDock(), "firmware.cab")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.UpdateAvailable {
		t.Fatalf("preflight reported an update for equal versions: %+v", result)
	}
	assertNoFirmwareWrites(t, connection.outputs)
}

func TestMacUpdaterRejectsUnsafePreflight(t *testing.T) {
	tests := []struct {
		name       string
		boardID    uint16
		power      uint16
		ecVersion  string
		reasonPart string
	}{
		{name: "old board", boardID: 5, power: 180, ecVersion: "01.01.00.13", reasonPart: "board"},
		{name: "missing power", boardID: 6, power: 0, ecVersion: "01.01.00.13", reasonPart: "power"},
		{name: "old EC", boardID: 6, power: 180, ecVersion: "01.01.00.00", reasonPart: "baseline"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := &fakeHIDConnection{
				fakeHIDReportDevice: fakeHIDReportDevice{inputs: wd19ReadOnlyInputsFor(test.boardID, test.power, test.ecVersion)},
			}
			payload := []byte("verified Dell CAB")
			updater := MacUpdater{
				HTTP:      &fakeHTTPDoer{response: (&httpResponseOK{body: payload}).response()},
				TempDir:   t.TempDir(),
				Extractor: &fakeFirmwareExtractor{blobs: wd19FirmwareBlobs()},
				Open: func(_ domain.HIDTarget) (HIDConnection, error) {
					return base, nil
				},
			}
			candidate := candidateFor(payload)
			result := updater.Apply(context.Background(), matchingDock(), &candidate)
			if result.State != "update_failed" || !strings.Contains(strings.ToLower(result.Reason), test.reasonPart) {
				t.Fatalf("unexpected preflight result: %+v", result)
			}
			for _, report := range base.outputs {
				if report[1] == hidExtensionWriteFlash || report[1] == hidExtensionI2CWrite && report[hidDataOffset] == ecCommandSetPackage {
					t.Fatalf("preflight failure caused a write: %02x", report[1])
				}
			}
		})
	}
}

func TestMacUpdaterRelocksTargetsAfterFlashFailure(t *testing.T) {
	payload := []byte("verified Dell CAB")
	base := &failFlashHIDConnection{
		fakeHIDConnection: fakeHIDConnection{
			fakeHIDReportDevice: fakeHIDReportDevice{inputs: wd19ReadOnlyInputs()},
		},
	}
	updater := MacUpdater{
		HTTP:      &fakeHTTPDoer{response: (&httpResponseOK{body: payload}).response()},
		TempDir:   t.TempDir(),
		Extractor: &fakeFirmwareExtractor{blobs: wd19FirmwareBlobs()},
		Open: func(_ domain.HIDTarget) (HIDConnection, error) {
			return base, nil
		},
	}
	candidate := candidateFor(payload)
	result := updater.Apply(context.Background(), matchingDock(), &candidate)
	if result.State != "update_failed" {
		t.Fatalf("unexpected result: %+v", result)
	}
	var lockStates []byte
	for _, report := range base.outputs {
		if report[0] == hidCommandWrite && report[1] == hidExtensionI2CWrite && report[hidDataOffset] == ecCommandModifyLock && report[hidDataOffset+2] == 1 {
			lockStates = append(lockStates, report[hidDataOffset+3])
		}
	}
	if len(lockStates) < 2 || lockStates[0] != 1 || lockStates[len(lockStates)-1] != 0 {
		t.Fatalf("EC lock was not cleaned up: %v", lockStates)
	}
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
	var opened []domain.HIDTarget

	updater := MacUpdater{
		HTTP:      client,
		TempDir:   t.TempDir(),
		Extractor: extractor,
		Open: func(target domain.HIDTarget) (HIDConnection, error) {
			opened = append(opened, target)
			return base, nil
		},
	}
	candidate := candidateFor(payload)
	result := updater.Apply(context.Background(), matchingDock(), &candidate)

	if result.State != "update_staged" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !strings.Contains(result.Reason, "EC 01.01.00.14") || !strings.Contains(result.Reason, "package 01.01.00.01") {
		t.Fatalf("update result does not list updated components: %s", result.Reason)
	}
	if len(opened) != 1 || opened[0].ProductID != 0xb06e || opened[0].LocationID != 0x00150000 {
		t.Fatalf("unexpected HID interfaces opened: %+v", opened)
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
	passiveFound := false
	for _, report := range base.outputs {
		if report[0] == hidCommandWrite && report[1] == hidExtensionI2CWrite && report[hidDataOffset] == ecCommandPassive {
			got, want := report[hidDataOffset:hidDataOffset+3], []byte{ecCommandPassive, 1, 3}
			if !bytes.Equal(got, want) {
				t.Fatalf("passive flow = %x, want %x", got, want)
			}
			passiveFound = true
		}
	}
	if !passiveFound {
		t.Fatal("passive activation command was not sent")
	}
}

func TestMacUpdaterRejectsNonCABBeforeOpeningHID(t *testing.T) {
	payload := []byte("payload")
	client := &fakeHTTPDoer{}
	opened := false
	updater := MacUpdater{
		HTTP:    client,
		TempDir: t.TempDir(),
		Open: func(_ domain.HIDTarget) (HIDConnection, error) {
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
		Open: func(_ domain.HIDTarget) (HIDConnection, error) {
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
	var opened []domain.HIDTarget
	updater := MacUpdater{
		HTTP:      client,
		TempDir:   t.TempDir(),
		Extractor: extractor,
		Open: func(target domain.HIDTarget) (HIDConnection, error) {
			opened = append(opened, target)
			if target.ProductID == wd19Gen1ProductID {
				return gen1, nil
			}
			return base, nil
		},
	}
	candidate := candidateFor(payload)
	result := updater.Apply(context.Background(), matchingDock(), &candidate)
	if result.State != "update_staged" || !strings.Contains(result.Reason, "USB hub Gen1 02.00") {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(opened) != 2 || opened[0].ProductID != wd19Gen2ProductID || opened[1].ProductID != wd19Gen1ProductID || opened[1].LocationID != 0x00135000 {
		t.Fatalf("unexpected HID interfaces opened: %+v", opened)
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
	binary.LittleEndian.PutUint64(dockData[16:24], 2000)
	copy(dockData[32:39], "TST0001")
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
		{0, 0},
	}
}

func assertNoFirmwareWrites(t *testing.T, reports [][]byte) {
	t.Helper()
	for _, report := range reports {
		if len(report) > 1 && (report[1] == hidExtensionI2CWrite ||
			report[1] == hidExtensionWriteFlash ||
			report[1] == hidExtensionErase ||
			report[1] == hidExtensionClock ||
			report[1] == hidExtensionVerify) {
			t.Fatalf("read-only preflight wrote HID report: %x", report)
		}
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

func wd19ReadOnlyInputsFor(boardID, power uint16, ecVersion string) [][]byte {
	inputs := wd19ReadOnlyInputs()
	data := inputs[0][1:]
	binary.LittleEndian.PutUint16(data[2:4], power)
	binary.LittleEndian.PutUint16(data[6:8], boardID)
	info := inputs[1][1:]
	copy(info[8:12], []byte{0, 0, 0, 0})
	parts := strings.Split(ecVersion, ".")
	if len(parts) == 4 {
		for index, part := range parts {
			value, err := strconv.ParseUint(part, 16, 8)
			if err != nil {
				return inputs
			}
			info[8+index] = byte(value)
		}
	}
	return inputs
}
