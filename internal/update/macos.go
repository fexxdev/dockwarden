package update

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/fexxdev/dockwarden/internal/domain"
)

const (
	wd19Gen2ProductID uint16 = 0xb06e
	wd19Gen1ProductID uint16 = 0xb06f

	salomonDockType byte = 0x04

	firmwareUpdateComplete byte = 0x01
	passiveReboot          byte = 0x02
	passiveReset           byte = 0x01

	ecBlobName             = "ec.bin"
	salomonPackageBlobName = "salomon_package.bin"
	gen1HubBlobName        = "rts5413.bin"
	gen2HubBlobName        = "rts5487.bin"
	mstBlobName            = "vmm5331.bin"

	ecBlobSize          = 0x1ffc0
	hubBlobSize         = 0x10000
	mstBlobSize         = 0x80000
	packageBlobSize     = 24
	ecBlobVersionOffset = 0x1afc0
	gen1VersionOffset   = 0x7f6e
	gen2VersionOffset   = 0x7f52
	mstVersionOffset    = 0x18400
)

type HIDConnection interface {
	HIDReports
	Close()
}

type HIDOpener func(productID uint16) (HIDConnection, error)

type FirmwareExtractor interface {
	Extract(context.Context, string, string) ([]byte, error)
}

type BsdtarExtractor struct {
	Runner CommandRunner
}

func (e BsdtarExtractor) Extract(ctx context.Context, archivePath, name string) ([]byte, error) {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return nil, fmt.Errorf("invalid CAB member name %q", name)
	}
	runner := e.Runner
	if runner == nil {
		runner = systemRunner{}
	}
	output, err := runner.Run(ctx, "bsdtar", "-xOf", archivePath, "DriverPackage/"+name)
	if err != nil {
		return nil, fmt.Errorf("cannot extract %s from Dell CAB: %w", name, err)
	}
	if len(output) == 0 {
		return nil, fmt.Errorf("Dell CAB member %s is empty", name)
	}
	return output, nil
}

type MacUpdater struct {
	HTTP      HTTPDoer
	Open      HIDOpener
	Extractor FirmwareExtractor
	TempDir   string
}

type macUpdatePlan struct {
	ecBlob           []byte
	ecCandidate      string
	updateEC         bool
	packageBlob      []byte
	packageCandidate string
	updatePackage    bool
	hubs             []macHubUpdate
	unsupportedMST   string
}

type macHubUpdate struct {
	Name         string
	ProductID    uint16
	UnlockTarget byte
	Blob         []byte
	Candidate    string
}

func (u MacUpdater) Apply(ctx context.Context, dock *domain.Dock, candidate *domain.FirmwareCandidate) domain.UpdateCheck {
	if !isSupportedWD19(dock) {
		return failed(candidate, "firmware backend accepts only the detected Dell Dock WD19 (413c:b06e)")
	}
	if candidate == nil {
		return failed(nil, "no firmware candidate was provided")
	}
	if !isDellDownloadURL(candidate.DownloadURL) {
		return failed(candidate, "candidate download URL is not an HTTPS Dell URL")
	}
	if !isCABDownloadURL(candidate.DownloadURL) {
		return failed(candidate, "macOS native updater accepts only a Dell CAB package")
	}
	if !isSHA256(candidate.SHA256) {
		return failed(candidate, "candidate does not contain a valid SHA-256")
	}
	if u.HTTP == nil {
		return failed(candidate, "firmware HTTP client is not configured")
	}
	if u.Open == nil {
		return failed(candidate, "macOS HID opener is not configured")
	}

	payloadPath, err := (FwupdUpdater{HTTP: u.HTTP, TempDir: u.TempDir}).download(ctx, candidate)
	if err != nil {
		return failed(candidate, err.Error())
	}
	defer os.Remove(payloadPath)

	baseConnection, err := u.Open(wd19Gen2ProductID)
	if err != nil {
		return failed(candidate, "cannot open WD19 control HID: "+err.Error())
	}
	if baseConnection == nil {
		return failed(candidate, "macOS HID opener returned no WD19 control device")
	}
	defer baseConnection.Close()
	base := DellHID{Reports: baseConnection}

	dockData, err := base.ReadDockData()
	if err != nil {
		return failed(candidate, "cannot read WD19 data over HID: "+err.Error())
	}
	if dockData.DockType != salomonDockType {
		return failed(candidate, fmt.Sprintf("detected WD19 dock type %#02x is not supported by the native updater", dockData.DockType))
	}
	components, err := base.ReadDockInfo()
	if err != nil {
		return failed(candidate, "cannot read WD19 component versions over HID: "+err.Error())
	}
	status, err := base.ReadUpdateStatus()
	if err != nil {
		return failed(candidate, "cannot read WD19 firmware update status: "+err.Error())
	}
	if status != firmwareUpdateComplete {
		return failed(candidate, fmt.Sprintf("WD19 reports firmware update status %#02x, refusing a new update", status))
	}

	extractor := u.Extractor
	if extractor == nil {
		extractor = BsdtarExtractor{}
	}
	plan, err := buildMacUpdatePlan(ctx, extractor, payloadPath, dockData, components)
	if err != nil {
		return failed(candidate, err.Error())
	}
	if plan.unsupportedMST != "" {
		return failed(candidate, "MST candidate "+plan.unsupportedMST+" is newer, but the native macOS MST writer is not implemented; no component was flashed")
	}

	updated := make([]string, 0, 4)
	if plan.updateEC {
		if err := base.ModifyLock(1, true); err != nil {
			return failed(candidate, "cannot unlock WD19 EC: "+err.Error())
		}
		if err := flashHID(ctx, base, 0xff000000, 0xff, plan.ecBlob, false); err != nil {
			return failed(candidate, "cannot write WD19 EC firmware: "+err.Error())
		}
		updated = append(updated, "EC "+plan.ecCandidate)
	}

	for _, hub := range plan.hubs {
		hubConnection := baseConnection
		if hub.ProductID != wd19Gen2ProductID {
			hubConnection, err = u.Open(hub.ProductID)
			if err != nil {
				return failed(candidate, "cannot open WD19 hub HID "+fmt.Sprintf("413c:%04x", hub.ProductID)+": "+err.Error())
			}
			if hubConnection == nil {
				return failed(candidate, fmt.Sprintf("macOS HID opener returned no WD19 hub device 413c:%04x", hub.ProductID))
			}
			defer hubConnection.Close()
		}
		hubDevice := DellHID{Reports: hubConnection}
		if err := base.ModifyLock(hub.UnlockTarget, true); err != nil {
			return failed(candidate, "cannot unlock "+hub.Name+": "+err.Error())
		}
		if err := flashHID(ctx, hubDevice, 0, 1, hub.Blob, true); err != nil {
			return failed(candidate, "cannot write "+hub.Name+": "+err.Error())
		}
		updated = append(updated, hub.Name+" "+hub.Candidate)
	}

	if plan.updatePackage {
		if err := base.CommitPackage(plan.packageBlob); err != nil {
			return failed(candidate, "cannot commit WD19 package versions: "+err.Error())
		}
		updated = append(updated, "package "+plan.packageCandidate)
	}
	if len(updated) == 0 {
		return domain.UpdateCheck{
			State:     "up_to_date",
			SourceURL: candidate.SourceURL,
			Reason:    "verified Dell CAB contains no component newer than the detected WD19 firmware",
			Candidate: candidate,
		}
	}

	flow := passiveReboot
	if plan.updateEC {
		flow |= passiveReset
	}
	if err := base.RebootPassive(flow); err != nil {
		return failed(candidate, "components were written but WD19 reboot activation failed: "+err.Error())
	}
	return domain.UpdateCheck{
		State:     "update_applied",
		SourceURL: candidate.SourceURL,
		Reason:    "updated " + strings.Join(updated, ", ") + "; unplug and reconnect the dock USB-C cable",
		Candidate: candidate,
	}
}

func buildMacUpdatePlan(ctx context.Context, extractor FirmwareExtractor, payloadPath string, dockData DockData, components []DockComponent) (macUpdatePlan, error) {
	if extractor == nil {
		return macUpdatePlan{}, fmt.Errorf("firmware extractor is not configured")
	}
	ecComponent, ok := findDockComponent(components, dockDeviceTypeEC, 0)
	if !ok {
		return macUpdatePlan{}, fmt.Errorf("WD19 component list has no embedded controller version")
	}
	ecBlob, err := extractExpectedBlob(ctx, extractor, payloadPath, ecBlobName, ecBlobSize)
	if err != nil {
		return macUpdatePlan{}, err
	}
	ecCandidate, err := asciiVersionAt(ecBlob, ecBlobVersionOffset, 11)
	if err != nil {
		return macUpdatePlan{}, fmt.Errorf("invalid %s version: %w", ecBlobName, err)
	}
	updateEC, err := versionIsNewer(ecCandidate, ecComponent.Version)
	if err != nil {
		return macUpdatePlan{}, fmt.Errorf("cannot compare EC versions: %w", err)
	}

	packageBlob, err := extractExpectedBlob(ctx, extractor, payloadPath, salomonPackageBlobName, packageBlobSize)
	if err != nil {
		return macUpdatePlan{}, err
	}
	packageCandidate, err := bcdVersionAt(packageBlob, 0x14)
	if err != nil {
		return macUpdatePlan{}, fmt.Errorf("invalid %s version: %w", salomonPackageBlobName, err)
	}
	updatePackage, err := versionIsNewer(packageCandidate, dockData.PackageVersion)
	if err != nil {
		return macUpdatePlan{}, fmt.Errorf("cannot compare package versions: %w", err)
	}

	plan := macUpdatePlan{
		ecBlob:           ecBlob,
		ecCandidate:      ecCandidate,
		updateEC:         updateEC,
		packageBlob:      packageBlob,
		packageCandidate: packageCandidate,
		updatePackage:    updatePackage,
	}
	if err := addHubPlan(ctx, extractor, payloadPath, components, &plan, 0, gen2HubBlobName, gen2VersionOffset, wd19Gen2ProductID, 7, "USB hub Gen2"); err != nil {
		return macUpdatePlan{}, err
	}
	if err := addHubPlan(ctx, extractor, payloadPath, components, &plan, 1, gen1HubBlobName, gen1VersionOffset, wd19Gen1ProductID, 8, "USB hub Gen1"); err != nil {
		return macUpdatePlan{}, err
	}

	mstComponent, hasMST := findDockComponent(components, dockDeviceTypeMST, 0)
	if !hasMST {
		mstComponent, hasMST = findDockComponent(components, dockDeviceTypeMST, 1)
	}
	if hasMST {
		mstBlob, err := extractExpectedBlob(ctx, extractor, payloadPath, mstBlobName, mstBlobSize)
		if err != nil {
			return macUpdatePlan{}, err
		}
		mstCandidate, err := tripleVersionAt(mstBlob, mstVersionOffset)
		if err != nil {
			return macUpdatePlan{}, fmt.Errorf("invalid %s version: %w", mstBlobName, err)
		}
		newer, err := versionIsNewer(mstCandidate, mstComponent.Version)
		if err != nil {
			return macUpdatePlan{}, fmt.Errorf("cannot compare MST versions: %w", err)
		}
		if newer {
			plan.unsupportedMST = mstCandidate + " (current " + mstComponent.Version + ")"
		}
	}
	return plan, nil
}

func addHubPlan(ctx context.Context, extractor FirmwareExtractor, payloadPath string, components []DockComponent, plan *macUpdatePlan, subType byte, blobName string, blobVersionOffset int, productID uint16, unlockTarget byte, name string) error {
	component, ok := findDockComponent(components, dockDeviceTypeHub, subType)
	if !ok {
		return nil
	}
	blob, err := extractExpectedBlob(ctx, extractor, payloadPath, blobName, hubBlobSize)
	if err != nil {
		return err
	}
	candidate, err := pairVersionAt(blob, blobVersionOffset)
	if err != nil {
		return fmt.Errorf("invalid %s version: %w", blobName, err)
	}
	newer, err := versionIsNewer(candidate, component.Version)
	if err != nil {
		return fmt.Errorf("cannot compare %s versions: %w", name, err)
	}
	if newer {
		plan.hubs = append(plan.hubs, macHubUpdate{
			Name:         name,
			ProductID:    productID,
			UnlockTarget: unlockTarget,
			Blob:         blob,
			Candidate:    candidate,
		})
	}
	return nil
}

func flashHID(ctx context.Context, device DellHID, address uint32, bank byte, blob []byte, verify bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := device.RaiseMcuClock(true); err != nil {
		return fmt.Errorf("raise MCU clock: %w", err)
	}
	clockRaised := true
	defer func() {
		if clockRaised {
			_ = device.RaiseMcuClock(false)
		}
	}()
	if err := device.EraseBank(bank); err != nil {
		return fmt.Errorf("erase bank %#02x: %w", bank, err)
	}
	for offset := 0; offset < len(blob); offset += hidMaxWrite {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := offset + hidMaxWrite
		if end > len(blob) {
			end = len(blob)
		}
		if err := device.WriteFlash(address+uint32(offset), blob[offset:end]); err != nil {
			return fmt.Errorf("write address %#08x: %w", address+uint32(offset), err)
		}
	}
	if verify {
		verified, err := device.VerifyUpdate()
		if err != nil {
			return fmt.Errorf("verify update: %w", err)
		}
		if !verified {
			return fmt.Errorf("device rejected firmware verification")
		}
	}
	if err := device.RaiseMcuClock(false); err != nil {
		return fmt.Errorf("lower MCU clock: %w", err)
	}
	clockRaised = false
	return nil
}

func extractExpectedBlob(ctx context.Context, extractor FirmwareExtractor, payloadPath, name string, expectedSize int) ([]byte, error) {
	blob, err := extractor.Extract(ctx, payloadPath, name)
	if err != nil {
		return nil, fmt.Errorf("cannot extract %s: %w", name, err)
	}
	if len(blob) != expectedSize {
		return nil, fmt.Errorf("Dell CAB member %s has size %d, want %d", name, len(blob), expectedSize)
	}
	return blob, nil
}

func findDockComponent(components []DockComponent, deviceType, subType byte) (DockComponent, bool) {
	for _, component := range components {
		if component.DeviceType == deviceType && component.SubType == subType {
			return component, true
		}
	}
	return DockComponent{}, false
}

func asciiVersionAt(blob []byte, offset, length int) (string, error) {
	if offset < 0 || length <= 0 || offset+length > len(blob) {
		return "", fmt.Errorf("version offset %#x is outside blob", offset)
	}
	version := strings.Trim(string(blob[offset:offset+length]), "\x00 \t\r\n")
	if _, err := parseFirmwareVersion(version); err != nil {
		return "", err
	}
	return version, nil
}

func bcdVersionAt(blob []byte, offset int) (string, error) {
	if offset < 0 || offset+4 > len(blob) {
		return "", fmt.Errorf("version offset %#x is outside blob", offset)
	}
	version := bcdQuad(uint32(blob[offset]) | uint32(blob[offset+1])<<8 | uint32(blob[offset+2])<<16 | uint32(blob[offset+3])<<24)
	if _, err := parseFirmwareVersion(version); err != nil {
		return "", err
	}
	return version, nil
}

func pairVersionAt(blob []byte, offset int) (string, error) {
	if offset < 0 || offset+2 > len(blob) {
		return "", fmt.Errorf("version offset %#x is outside blob", offset)
	}
	version := fmt.Sprintf("%02x.%02x", blob[offset], blob[offset+1])
	if _, err := parseFirmwareVersion(version); err != nil {
		return "", err
	}
	return version, nil
}

func tripleVersionAt(blob []byte, offset int) (string, error) {
	if offset < 0 || offset+3 > len(blob) {
		return "", fmt.Errorf("version offset %#x is outside blob", offset)
	}
	version := fmt.Sprintf("%02x.%02x.%02x", blob[offset], blob[offset+1], blob[offset+2])
	if _, err := parseFirmwareVersion(version); err != nil {
		return "", err
	}
	return version, nil
}

func versionIsNewer(candidate, current string) (bool, error) {
	candidateParts, err := parseFirmwareVersion(candidate)
	if err != nil {
		return false, fmt.Errorf("invalid candidate version %q: %w", candidate, err)
	}
	currentParts, err := parseFirmwareVersion(current)
	if err != nil {
		return false, fmt.Errorf("invalid current version %q: %w", current, err)
	}
	candidateParts = trimLeadingVersionZeros(candidateParts)
	currentParts = trimLeadingVersionZeros(currentParts)
	for index := 0; index < len(candidateParts) || index < len(currentParts); index++ {
		var candidatePart, currentPart int
		if index < len(candidateParts) {
			candidatePart = candidateParts[index]
		}
		if index < len(currentParts) {
			currentPart = currentParts[index]
		}
		if candidatePart != currentPart {
			return candidatePart > currentPart, nil
		}
	}
	return false, nil
}

func trimLeadingVersionZeros(parts []int) []int {
	for len(parts) > 1 && parts[0] == 0 {
		parts = parts[1:]
	}
	return parts
}

func parseFirmwareVersion(value string) ([]int, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) == 0 || len(parts) > 8 {
		return nil, fmt.Errorf("version %q has invalid component count", value)
	}
	result := make([]int, len(parts))
	for index, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("version %q has an empty component", value)
		}
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed < 0 || parsed > 255 {
			return nil, fmt.Errorf("version %q has invalid component %q", value, part)
		}
		result[index] = parsed
	}
	return result, nil
}

func isCABDownloadURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && strings.HasSuffix(strings.ToLower(parsed.Path), ".cab")
}
