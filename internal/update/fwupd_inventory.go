package update

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/fexxdev/dockwarden/internal/domain"
	"github.com/fexxdev/dockwarden/internal/firmwareversion"
	"github.com/fexxdev/dockwarden/internal/logging"
)

var fwupdRequiredWD19Components = []string{
	domain.FirmwareComponentPackage,
	domain.FirmwareComponentEmbeddedController,
	domain.FirmwareComponentUSBHubGen1,
	domain.FirmwareComponentUSBHubGen2,
	domain.FirmwareComponentMST,
}

// FwupdToolClient contains the read-only managed fwupd runtime settings.
// It is shared by inventory, permission, and preflight checks.
type FwupdToolClient struct {
	Runner    CommandRunner
	ToolPath  string
	ConfigDir string
	TempDir   string
	Logger    logging.Logger
}

// FwupdToolFirmwareReader reads macOS firmware through the upstream Dell
// plugin. It does not open or write a device from Dockwarden.
type FwupdToolFirmwareReader struct {
	Client FwupdToolClient
}

// FwupdToolPreflight performs the read-only check before the explicit writer.
type FwupdToolPreflight struct {
	Client FwupdToolClient
}

// FwupdToolPermissionChecker checks the same managed runtime used by updates.
type FwupdToolPermissionChecker struct {
	Client FwupdToolClient
}

// MacPreflightResult identifies one fwupd target and the candidate decision.
type MacPreflightResult struct {
	DeviceID        string
	UpdateAvailable bool
}

type fwupdToolSession struct {
	runner   CommandRunner
	env      []string
	toolPath string
	stateDir string
	logger   logging.Logger
}

func (c FwupdToolClient) openSession(ctx context.Context) (fwupdToolSession, error) {
	toolPath, err := resolveFwupdToolPath(c.ToolPath, c.ConfigDir)
	if err != nil {
		return fwupdToolSession{}, err
	}
	if err := verifyFwupdTool(toolPath); err != nil {
		return fwupdToolSession{}, err
	}
	runner := c.Runner
	if runner == nil {
		runner = systemRunner{}
	}
	if _, ok := runner.(CommandRunnerWithEnv); !ok {
		return fwupdToolSession{}, fmt.Errorf("fwupdtool runner does not support an isolated environment")
	}
	stateDir, err := os.MkdirTemp(c.TempDir, "dockwarden-fwupd-read-")
	if err != nil {
		return fwupdToolSession{}, fmt.Errorf("cannot create temporary fwupd state: %w", err)
	}
	session := fwupdToolSession{
		runner:   runner,
		env:      fwupdToolEnvironment(stateDir),
		toolPath: toolPath,
		stateDir: stateDir,
		logger:   c.Logger,
	}
	if err := verifyFwupdToolVersion(ctx, session.runner, session.env, session.toolPath, session.logger); err != nil {
		session.close()
		return fwupdToolSession{}, err
	}
	return session, nil
}

func (s fwupdToolSession) close() {
	if s.stateDir != "" {
		_ = os.RemoveAll(s.stateDir)
	}
}

func (c FwupdToolClient) readDevices(ctx context.Context) ([]fwupdToolDevice, error) {
	session, err := c.openSession(ctx)
	if err != nil {
		return nil, err
	}
	defer session.close()
	return readFwupdDevices(ctx, session.runner, session.env, session.toolPath, session.logger)
}

func (r FwupdToolFirmwareReader) Read(ctx context.Context, dock *domain.Dock) ([]domain.FirmwareObservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !isSupportedWD19(dock) {
		return nil, fmt.Errorf("firmware reader accepts only the detected Dell Dock WD19 (413c:b06e)")
	}
	devices, err := r.Client.readDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("fwupdtool cannot enumerate the WD19: %w", err)
	}
	parent, err := selectFwupdWD19DeviceForDock(devices, dock)
	if err != nil {
		return nil, err
	}
	logUpdateEvent(r.Client.Logger, "INFO", "fwupd.target.selected", map[string]string{"device": parent.DeviceID})
	observations, err := fwupdInventoryObservations(devices, parent.DeviceID)
	if err != nil {
		return nil, enrichFwupdInventoryError(err, dock)
	}
	return observations, nil
}

func (p FwupdToolPreflight) Check(ctx context.Context, dock *domain.Dock, candidate *domain.FirmwareCandidate) (MacPreflightResult, error) {
	if err := ctx.Err(); err != nil {
		return MacPreflightResult{}, err
	}
	if !isSupportedWD19(dock) {
		return MacPreflightResult{}, fmt.Errorf("firmware backend accepts only the detected Dell Dock WD19 (413c:b06e)")
	}
	if candidate == nil {
		return MacPreflightResult{}, fmt.Errorf("no firmware candidate was provided")
	}
	devices, err := p.Client.readDevices(ctx)
	if err != nil {
		return MacPreflightResult{}, fmt.Errorf("fwupdtool cannot enumerate the WD19: %w", err)
	}
	parent, err := selectFwupdWD19DeviceForDock(devices, dock)
	if err != nil {
		return MacPreflightResult{}, err
	}
	logUpdateEvent(p.Client.Logger, "INFO", "fwupd.target.selected", map[string]string{"device": parent.DeviceID})
	observations, err := fwupdInventoryObservations(devices, parent.DeviceID)
	if err != nil {
		return MacPreflightResult{}, enrichFwupdInventoryError(err, dock)
	}
	updateAvailable, err := compareFwupdCandidate(observations, candidate)
	if err != nil {
		return MacPreflightResult{}, err
	}
	return MacPreflightResult{
		DeviceID:        parent.DeviceID,
		UpdateAvailable: updateAvailable,
	}, nil
}

func (p FwupdToolPermissionChecker) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := p.Client.readDevices(ctx); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no detected devices") {
			return nil
		}
		if fwupdHIDPermissionError(err) {
			return fmt.Errorf("fwupdtool HID permission probe failed: %w", err)
		}
		return fmt.Errorf("fwupdtool inventory probe failed: %w", err)
	}
	return nil
}

func fwupdHIDPermissionError(err error) bool {
	if err == nil {
		return false
	}
	details := strings.ToLower(err.Error())
	for _, marker := range []string{
		"input monitoring",
		"permission",
		"access denied",
		"not permitted",
		"not authorized",
	} {
		if strings.Contains(details, marker) {
			return true
		}
	}
	return false
}

func selectFwupdWD19DeviceForDock(devices []fwupdToolDevice, dock *domain.Dock) (fwupdToolDevice, error) {
	if dock == nil {
		return fwupdToolDevice{}, fmt.Errorf("dock is not detected")
	}
	wantSerial := strings.TrimSpace(dock.Serial)
	if wantSerial == "" {
		return fwupdToolDevice{}, fmt.Errorf("detected WD19 has no USB serial")
	}
	matches := make([]fwupdToolDevice, 0, 1)
	for _, device := range devices {
		if device.Plugin != "dell_dock" || !hasWD19EmbeddedInstanceID(device.InstanceIDs) {
			continue
		}
		if !fwupdWD19DeviceMatchesUSBSerial(device, devices, wantSerial) {
			continue
		}
		if !fwupdDeviceIDPattern.MatchString(device.DeviceID) {
			return fwupdToolDevice{}, fmt.Errorf("matching WD19 device has an invalid DeviceId")
		}
		matches = append(matches, device)
	}
	if len(matches) == 0 {
		return fwupdToolDevice{}, fmt.Errorf("no matching WD19 device was reported by fwupdtool")
	}
	if len(matches) != 1 {
		return fwupdToolDevice{}, fmt.Errorf("multiple matching WD19 devices were reported by fwupdtool")
	}
	return matches[0], nil
}

func fwupdWD19DeviceMatchesUSBSerial(parent fwupdToolDevice, devices []fwupdToolDevice, usbSerial string) bool {
	if fwupdSerialMatchesUSBSerial(parent.Serial, usbSerial) {
		return true
	}
	for _, device := range devices {
		if device.Plugin != "dell_dock" || device.DeviceID == parent.DeviceID {
			continue
		}
		if !fwupdDeviceBelongsTo(device, parent.DeviceID) {
			continue
		}
		if fwupdSerialMatchesUSBSerial(device.Serial, usbSerial) {
			return true
		}
	}
	return false
}

func fwupdSerialMatchesUSBSerial(fwupdSerial, usbSerial string) bool {
	fwupdSerial = strings.TrimSpace(fwupdSerial)
	usbSerial = strings.TrimSpace(usbSerial)
	if fwupdSerial == "" || usbSerial == "" {
		return false
	}
	if fwupdSerial == usbSerial {
		return true
	}
	return strings.HasPrefix(fwupdSerial, usbSerial+"/")
}

func fwupdInventoryObservations(devices []fwupdToolDevice, parentID string) ([]domain.FirmwareObservation, error) {
	versions := make(map[string]string, len(fwupdRequiredWD19Components))
	for _, device := range devices {
		if device.Plugin != "dell_dock" || !fwupdDeviceBelongsTo(device, parentID) {
			continue
		}
		if fwupdDeviceHasPendingUpdate(device) {
			return nil, fmt.Errorf("selected WD19 reports a firmware update pending; versions are not verified")
		}
		component := fwupdDeviceComponent(device, parentID)
		version := strings.TrimSpace(device.Version)
		if component == "" || version == "" {
			continue
		}
		if previous, ok := versions[component]; ok && previous != version {
			return nil, fmt.Errorf("selected WD19 has conflicting %s versions %s and %s", component, previous, version)
		}
		versions[component] = version
	}

	observations := make([]domain.FirmwareObservation, 0, len(fwupdRequiredWD19Components))
	for _, component := range fwupdRequiredWD19Components {
		version := strings.TrimSpace(versions[component])
		if version == "" {
			return nil, fmt.Errorf("selected WD19 has no %s version in fwupdtool output", component)
		}
		observations = append(observations, domain.FirmwareObservation{
			Component:  component,
			Version:    version,
			Source:     "fwupdtool",
			Confidence: "direct",
		})
	}
	return observations, nil
}

func compareFwupdCandidate(observations []domain.FirmwareObservation, candidate *domain.FirmwareCandidate) (bool, error) {
	if candidate == nil {
		return false, fmt.Errorf("no firmware candidate was provided")
	}
	current := make(map[string]string, len(observations))
	for _, observation := range observations {
		current[observation.Component] = strings.TrimSpace(observation.Version)
	}
	updateAvailable := false
	for _, component := range fwupdRequiredWD19Components {
		want := strings.TrimSpace(candidate.ComponentVersions[component])
		if want == "" {
			return false, fmt.Errorf("candidate has no %s version", component)
		}
		got := strings.TrimSpace(current[component])
		if got == "" {
			return false, fmt.Errorf("detected WD19 has no %s version", component)
		}
		comparison, err := firmwareversion.Compare(want, got)
		if err != nil {
			return false, fmt.Errorf("cannot compare %s versions: %w", component, err)
		}
		if comparison > 0 {
			updateAvailable = true
		}
	}
	return updateAvailable, nil
}

func enrichFwupdInventoryError(err error, dock *domain.Dock) error {
	if err == nil || dock == nil || !strings.Contains(err.Error(), "usb_hub_gen1") {
		return err
	}
	device, ok := physicalUSBHubGen1Device(dock)
	if !ok {
		return err
	}
	return fmt.Errorf(
		"%w; physical USB hub Gen1 413c:b06f at location %s reports descriptor version %s; descriptor evidence is read-only and cannot authorize firmware writes",
		err,
		device.Location,
		strings.TrimSpace(device.DescriptorVersion),
	)
}

func physicalUSBHubGen1Device(dock *domain.Dock) (domain.USBDevice, bool) {
	if dock == nil || strings.TrimSpace(dock.Serial) == "" {
		return domain.USBDevice{}, false
	}
	byLocation := make(map[string]domain.USBDevice, len(dock.Devices))
	for _, device := range dock.Devices {
		if device.Location != "" {
			byLocation[device.Location] = device
		}
	}
	var targetRoot string
	for _, device := range dock.Devices {
		if device.VendorID != 0x413c || device.ProductID != 0xb06e ||
			strings.TrimSpace(device.Serial) != strings.TrimSpace(dock.Serial) ||
			!strings.Contains(strings.ToLower(device.Product+" "+device.Name), "wd19") {
			continue
		}
		targetRoot = physicalUSBRoot(device, byLocation)
		break
	}
	if targetRoot == "" {
		return domain.USBDevice{}, false
	}

	var candidate domain.USBDevice
	matches := 0
	for _, device := range dock.Devices {
		if device.VendorID != 0x413c || device.ProductID != 0xb06f ||
			strings.TrimSpace(device.DescriptorVersion) == "" ||
			physicalUSBRoot(device, byLocation) != targetRoot {
			continue
		}
		candidate = device
		matches++
	}
	if matches != 1 {
		return domain.USBDevice{}, false
	}
	return candidate, true
}

func physicalUSBRoot(device domain.USBDevice, byLocation map[string]domain.USBDevice) string {
	current := device
	root := ""
	visited := make(map[string]bool)
	for {
		if current.Location != "" {
			if visited[current.Location] {
				return ""
			}
			visited[current.Location] = true
			root = current.Location
		}
		if current.ParentLocation == "" || current.ParentLocation == "00000000" {
			return root
		}
		next, ok := byLocation[current.ParentLocation]
		if !ok {
			return ""
		}
		current = next
	}
}
