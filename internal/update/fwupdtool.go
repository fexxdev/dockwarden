package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fexxdev/dockwarden/internal/domain"
	"github.com/fexxdev/dockwarden/internal/logging"
)

const (
	// FwupdToolEnvironmentVariable selects the standalone fwupdtool binary on macOS.
	FwupdToolEnvironmentVariable = "DOCKWARDEN_FWUPDTOOL"
	fwupdToolVersion             = "2.2.1"
	fwupdSourceCommit            = "74fcfabec244dc073aeb36669c5118fdfcd5107b"
)

type CommandRunnerWithEnv interface {
	RunWithEnv(context.Context, []string, string, ...string) ([]byte, error)
}

type MacPreflighter interface {
	Check(context.Context, *domain.Dock, *domain.FirmwareCandidate) (MacPreflightResult, error)
}

type FwupdToolUpdater struct {
	HTTP      HTTPDoer
	Runner    CommandRunner
	ToolPath  string
	ConfigDir string
	TempDir   string
	Preflight MacPreflighter
	Logger    logging.Logger
}

type fwupdToolManifest struct {
	FwupdVersion  string            `json:"fwupd_version"`
	SourceCommit  string            `json:"source_commit"`
	BinarySHA256  string            `json:"binary_sha256"`
	RuntimeSHA256 map[string]string `json:"runtime_sha256"`
}

type fwupdToolVersionOutput struct {
	Versions []fwupdToolVersionEntry `json:"Versions"`
}

type fwupdToolVersionEntry struct {
	Type        string `json:"Type"`
	AppstreamID string `json:"AppstreamId"`
	Version     string `json:"Version"`
}

type fwupdToolDevicesOutput struct {
	Devices []fwupdToolDevice `json:"Devices"`
}

type fwupdToolDevice struct {
	Name           string   `json:"Name"`
	Summary        string   `json:"Summary"`
	Plugin         string   `json:"Plugin"`
	InstanceIDs    []string `json:"InstanceIds"`
	Serial         string   `json:"Serial"`
	DeviceID       string   `json:"DeviceId"`
	ParentDeviceID string   `json:"ParentDeviceId"`
	CompositeID    string   `json:"CompositeId"`
	Version        string   `json:"Version"`
	Flags          []string `json:"Flags"`
	Problems       []string `json:"Problems"`
}

type fwupdPostInstallState string

const (
	fwupdPostInstallVerified fwupdPostInstallState = "verified"
	fwupdPostInstallStaged   fwupdPostInstallState = "staged"
)

type fwupdPostInstallResult struct {
	State  fwupdPostInstallState
	Reason string
}

var fwupdDeviceIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

var fwupdRuntimeRoots = []string{
	"bin",
	"etc/fwupd",
	"lib",
	"share/fwupd",
}

var fwupdRequiredRuntimeFiles = []string{
	"bin/fwupdtool",
	"lib/fwupd-2.2.1/libfwupdcli.dylib",
	"lib/fwupd-2.2.1/libfwupdengine.dylib",
	"lib/fwupd-2.2.1/libfwupdplugin.dylib",
	"lib/libfwupd.3.dylib",
	"share/fwupd/quirks.d/builtin.quirk.gz",
}

// CheckReady verifies the local macOS writer before apply mode accesses Dell.
func (u FwupdToolUpdater) CheckReady(ctx context.Context) error {
	logUpdateEvent(u.Logger, "INFO", "fwupd.ready.start", map[string]string{"backend": "fwupdtool"})
	toolPath, err := u.resolveToolPath()
	if err != nil {
		return err
	}
	if err := verifyFwupdTool(toolPath); err != nil {
		return err
	}
	runner := u.Runner
	if runner == nil {
		runner = systemRunner{}
	}
	if _, ok := runner.(CommandRunnerWithEnv); !ok {
		return fmt.Errorf("fwupdtool runner does not support an isolated environment")
	}
	stateDir, err := os.MkdirTemp(u.TempDir, "dockwarden-fwupd-ready-*")
	if err != nil {
		return fmt.Errorf("cannot create temporary fwupd state: %w", err)
	}
	defer os.RemoveAll(stateDir)
	err = verifyFwupdToolVersion(ctx, runner, fwupdToolEnvironment(stateDir), toolPath, u.Logger)
	logUpdateEvent(u.Logger, commandLogLevel(err), "fwupd.ready", map[string]string{
		"backend": "fwupdtool",
		"error":   errorText(err),
	})
	return err
}

func (u FwupdToolUpdater) Apply(ctx context.Context, dock *domain.Dock, candidate *domain.FirmwareCandidate) (result domain.UpdateCheck) {
	logUpdateEvent(u.Logger, "INFO", "firmware.apply.start", map[string]string{
		"backend": "fwupdtool",
		"model":   dockModel(dock),
	})
	defer func() {
		level := "INFO"
		if result.State == "update_failed" {
			level = "ERROR"
		}
		logUpdateEvent(u.Logger, level, "firmware.apply.complete", map[string]string{
			"backend": "fwupdtool",
			"state":   result.State,
			"reason":  result.Reason,
		})
	}()
	if !isSupportedWD19(dock) {
		return failed(candidate, "firmware backend accepts only the detected Dell Dock WD19 (413c:b06e)")
	}
	if candidate == nil {
		return failed(nil, "no firmware candidate was provided")
	}
	if !isDellDownloadURL(candidate.DownloadURL) || !isCABDownloadURL(candidate.DownloadURL) {
		return failed(candidate, "macOS fwupdtool backend accepts only an HTTPS Dell CAB package")
	}
	if !strings.EqualFold(candidate.Format, "CAB") || !strings.HasSuffix(strings.ToLower(candidate.PackageName), ".cab") {
		return failed(candidate, "macOS fwupdtool backend requires CAB metadata")
	}
	if !isSHA256(candidate.SHA256) {
		return failed(candidate, "candidate does not contain a valid SHA-256")
	}
	if !candidateSupports(candidate, "wd19", "linux") {
		return failed(candidate, "candidate does not explicitly support Dell Dock WD19")
	}
	if u.HTTP == nil {
		return failed(candidate, "firmware HTTP client is not configured")
	}

	toolPath, err := u.resolveToolPath()
	if err != nil {
		return failed(candidate, err.Error())
	}
	if err := verifyFwupdTool(toolPath); err != nil {
		return failed(candidate, err.Error())
	}
	runner := u.Runner
	if runner == nil {
		runner = systemRunner{}
	}
	if _, ok := runner.(CommandRunnerWithEnv); !ok {
		return failed(candidate, "fwupdtool runner does not support an isolated environment")
	}
	stateDir, err := os.MkdirTemp(u.TempDir, "dockwarden-fwupd-state-*")
	if err != nil {
		return failed(candidate, fmt.Sprintf("cannot create temporary fwupd state: %v", err))
	}
	defer os.RemoveAll(stateDir)
	env := fwupdToolEnvironment(stateDir)
	if err := verifyFwupdToolVersion(ctx, runner, env, toolPath, u.Logger); err != nil {
		logUpdateEvent(u.Logger, "ERROR", "fwupd.ready", map[string]string{
			"backend": "fwupdtool",
			"error":   err.Error(),
		})
		return failed(candidate, err.Error())
	}
	logUpdateEvent(u.Logger, "INFO", "fwupd.ready", map[string]string{"backend": "fwupdtool"})

	preflight := u.Preflight
	if preflight == nil {
		return failed(candidate, "macOS fwupd preflight is not configured")
	}
	return u.applyWithTarget(ctx, dock, candidate, runner, env, toolPath, preflight)
}

func (u FwupdToolUpdater) applyWithTarget(ctx context.Context, dock *domain.Dock, candidate *domain.FirmwareCandidate, runner CommandRunner, env []string, toolPath string, preflight MacPreflighter) domain.UpdateCheck {
	result, err := preflight.Check(ctx, dock, candidate)
	if err != nil {
		return failed(candidate, err.Error())
	}
	logUpdateEvent(u.Logger, "INFO", "macos.preflight.complete", map[string]string{
		"device": result.DeviceID,
		"update": fmt.Sprintf("%t", result.UpdateAvailable),
	})
	if !result.UpdateAvailable {
		return domain.UpdateCheck{
			State:     "up_to_date",
			SourceURL: candidate.SourceURL,
			Reason:    "verified Dell CAB contains no component newer than the detected WD19 firmware",
			Candidate: candidate,
		}
	}
	if !fwupdDeviceIDPattern.MatchString(result.DeviceID) {
		return failed(candidate, "fwupd preflight returned an invalid WD19 DeviceId")
	}
	payloadPath, err := (FwupdUpdater{HTTP: u.HTTP, TempDir: u.TempDir, Logger: u.Logger}).download(ctx, candidate)
	if err != nil {
		return failed(candidate, err.Error())
	}
	defer os.Remove(payloadPath)

	args := []string{
		"--plugins",
		"dell_dock",
		"--assume-yes",
		"--no-reboot-check",
		"install",
		payloadPath,
		result.DeviceID,
	}
	output, err := runLoggedFwupdCommand(ctx, runner, env, toolPath, u.Logger, args...)
	logUpdateEvent(u.Logger, commandLogLevel(err), "fwupdtool.target.result", map[string]string{
		"device": result.DeviceID,
		"error":  errorText(err),
	})
	postInstall, postInstallErr := fwupdPostInstallVerification(ctx, runner, env, toolPath, result.DeviceID, candidate, u.Logger)
	if err != nil {
		if postInstallErr == nil && postInstall.State == fwupdPostInstallVerified {
			reason := "fwupdtool returned an error after the candidate versions were verified on the dock: " + err.Error()
			if text := strings.TrimSpace(string(output)); text != "" {
				reason += ": " + summarize(text)
			}
			return domain.UpdateCheck{
				State:     "update_verified",
				SourceURL: candidate.SourceURL,
				Reason:    reason,
				Candidate: candidate,
			}
		}
		if postInstallErr == nil && postInstall.State == fwupdPostInstallStaged {
			reason := "fwupdtool returned an error, but the dock reports an update pending; unplug and reconnect the dock USB-C cable, then run status: " + err.Error()
			if text := strings.TrimSpace(string(output)); text != "" {
				reason += ": " + summarize(text)
			}
			return domain.UpdateCheck{
				State:     "update_staged",
				SourceURL: candidate.SourceURL,
				Reason:    reason,
				Candidate: candidate,
			}
		}
		reason := "fwupdtool: " + err.Error()
		if text := strings.TrimSpace(string(output)); text != "" {
			reason += ": " + summarize(text)
		}
		if postInstallErr != nil {
			reason += "; post-install verification unavailable: " + postInstallErr.Error()
		} else if postInstall.Reason != "" {
			reason += "; post-install verification failed: " + postInstall.Reason
		}
		return failed(candidate, reason)
	}
	if postInstallErr == nil && postInstall.State == fwupdPostInstallVerified {
		reason := "fwupdtool completed and all candidate component versions were verified on the selected WD19"
		if text := strings.TrimSpace(string(output)); text != "" {
			reason += ": " + summarize(text)
		}
		return domain.UpdateCheck{
			State:     "update_verified",
			SourceURL: candidate.SourceURL,
			Reason:    reason,
			Candidate: candidate,
		}
	}
	if postInstallErr == nil && len(candidate.ComponentVersions) > 0 && postInstall.State != fwupdPostInstallStaged {
		reason := "fwupdtool completed, but post-install verification did not match the candidate: " + postInstall.Reason
		return failed(candidate, reason)
	}
	reason := "fwupdtool accepted the verified Dell package for the selected WD19; unplug and reconnect the dock USB-C cable, then run status"
	if text := strings.TrimSpace(string(output)); text != "" {
		reason = summarize(text) + "; unplug and reconnect the dock USB-C cable, then run status"
	}
	if postInstallErr != nil {
		reason += "; post-install verification unavailable: " + postInstallErr.Error()
	} else if postInstall.Reason != "" {
		reason += "; post-install verification: " + postInstall.Reason
	}
	return domain.UpdateCheck{
		State:     "update_staged",
		SourceURL: candidate.SourceURL,
		Reason:    reason,
		Candidate: candidate,
	}
}

func (u FwupdToolUpdater) resolveToolPath() (string, error) {
	return resolveFwupdToolPath(u.ToolPath, u.ConfigDir)
}

func resolveFwupdToolPath(toolOverride, configuredDir string) (string, error) {
	toolPath := strings.TrimSpace(toolOverride)
	if toolPath != "" {
		if !filepath.IsAbs(toolPath) {
			return "", fmt.Errorf("%s must be an absolute path", FwupdToolEnvironmentVariable)
		}
		return filepath.Clean(toolPath), nil
	}
	configDir := strings.TrimSpace(configuredDir)
	if configDir == "" {
		var err error
		configDir, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve managed fwupdtool path: %w", err)
		}
	}
	if !filepath.IsAbs(configDir) {
		return "", fmt.Errorf("managed fwupdtool configuration directory is not absolute")
	}
	return filepath.Join(configDir, "dockwarden", "fwupd-"+fwupdToolVersion, "bin", "fwupdtool"), nil
}

func verifyFwupdTool(toolPath string) error {
	info, err := os.Lstat(toolPath)
	if err != nil {
		return fmt.Errorf("cannot access managed fwupdtool %s: %w", toolPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("managed fwupdtool %s is not a regular file", toolPath)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("managed fwupdtool %s is not executable", toolPath)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("managed fwupdtool %s is writable by group or others", toolPath)
	}
	manifestPath := filepath.Join(filepath.Dir(filepath.Dir(toolPath)), "manifest.json")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil {
		return fmt.Errorf("cannot access fwupdtool manifest: %w", err)
	}
	if !manifestInfo.Mode().IsRegular() || manifestInfo.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("fwupdtool manifest is not a protected regular file")
	}
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("cannot read fwupdtool manifest: %w", err)
	}
	var manifest fwupdToolManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("cannot parse fwupdtool manifest: %w", err)
	}
	if manifest.FwupdVersion != fwupdToolVersion || manifest.SourceCommit != fwupdSourceCommit {
		return fmt.Errorf("fwupdtool manifest does not match the managed fwupd 2.2.1 build")
	}
	if !isSHA256(manifest.BinarySHA256) {
		return fmt.Errorf("fwupdtool manifest has no valid binary SHA-256")
	}
	actualHash, err := fileSHA256(toolPath)
	if err != nil {
		return fmt.Errorf("cannot hash managed fwupdtool: %w", err)
	}
	if !strings.EqualFold(actualHash, manifest.BinarySHA256) {
		return fmt.Errorf("managed fwupdtool binary SHA-256 does not match its manifest")
	}
	prefix := filepath.Dir(filepath.Dir(toolPath))
	runtimeHashes, err := fwupdRuntimeHashes(prefix)
	if err != nil {
		return fmt.Errorf("cannot attest fwupdtool runtime: %w", err)
	}
	for _, relativePath := range fwupdRequiredRuntimeFiles {
		if _, ok := manifest.RuntimeSHA256[relativePath]; !ok {
			return fmt.Errorf("fwupdtool manifest has no hash for required runtime file %s", relativePath)
		}
	}
	if len(runtimeHashes) != len(manifest.RuntimeSHA256) {
		return fmt.Errorf("fwupdtool runtime file set does not match its manifest")
	}
	for relativePath, actual := range runtimeHashes {
		expected, ok := manifest.RuntimeSHA256[relativePath]
		if !ok || !isSHA256(expected) || !strings.EqualFold(actual, expected) {
			return fmt.Errorf("fwupdtool runtime file %s does not match its manifest", relativePath)
		}
	}
	if !strings.EqualFold(runtimeHashes["bin/fwupdtool"], manifest.BinarySHA256) {
		return fmt.Errorf("fwupdtool binary hashes are inconsistent in its manifest")
	}
	return nil
}

func fwupdRuntimeHashes(prefix string) (map[string]string, error) {
	hashes := make(map[string]string)
	resolvedPrefix, err := filepath.EvalSymlinks(prefix)
	if err != nil {
		return nil, err
	}
	for _, relativeRoot := range fwupdRuntimeRoots {
		root := filepath.Join(prefix, filepath.FromSlash(relativeRoot))
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				info, err := entry.Info()
				if err != nil {
					return err
				}
				if info.Mode().Perm()&0o022 != 0 {
					return fmt.Errorf("runtime directory %s is writable by group or others", path)
				}
				return nil
			}
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("runtime path %s is not a regular file", path)
			}
			if info.Mode().Perm()&0o022 != 0 {
				return fmt.Errorf("runtime file %s is writable by group or others", path)
			}
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
			withinPrefix, err := filepath.Rel(resolvedPrefix, resolved)
			if err != nil || withinPrefix == ".." || strings.HasPrefix(withinPrefix, ".."+string(filepath.Separator)) {
				return fmt.Errorf("runtime path %s resolves outside the managed prefix", path)
			}
			relativePath, err := filepath.Rel(prefix, path)
			if err != nil {
				return err
			}
			hash, err := fileSHA256(path)
			if err != nil {
				return err
			}
			hashes[filepath.ToSlash(relativePath)] = hash
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return hashes, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func fwupdToolEnvironment(stateDir string) []string {
	return []string{
		"PATH=/usr/bin:/bin",
		"HOME=" + stateDir,
		"TMPDIR=" + stateDir,
		"LANG=C",
		"LC_ALL=C",
		"XDG_CACHE_HOME=" + filepath.Join(stateDir, "xdg-cache"),
		"XDG_CONFIG_HOME=" + filepath.Join(stateDir, "xdg-config"),
		"XDG_DATA_HOME=" + filepath.Join(stateDir, "xdg-data"),
		"FWUPD_LOCALSTATEDIR=" + stateDir,
		"CACHE_DIRECTORY=" + filepath.Join(stateDir, "cache"),
	}
}

func verifyFwupdToolVersion(ctx context.Context, runner CommandRunner, env []string, toolPath string, logger logging.Logger) error {
	output, err := runLoggedFwupdCommand(ctx, runner, env, toolPath, logger, "--version", "--json")
	if err != nil {
		return commandError("fwupdtool --version --json", output, err)
	}
	var version fwupdToolVersionOutput
	if err := json.Unmarshal(output, &version); err != nil {
		return fmt.Errorf("cannot parse fwupdtool --version JSON: %w", err)
	}
	compileVersion := ""
	runtimeVersion := ""
	for _, item := range version.Versions {
		if item.AppstreamID != "org.freedesktop.fwupd" {
			continue
		}
		switch item.Type {
		case "compile":
			compileVersion = item.Version
		case "runtime":
			runtimeVersion = item.Version
		}
	}
	if compileVersion != fwupdToolVersion || runtimeVersion != fwupdToolVersion {
		return fmt.Errorf("fwupdtool compile and runtime versions must both be %s", fwupdToolVersion)
	}
	return nil
}

func readFwupdDevices(ctx context.Context, runner CommandRunner, env []string, toolPath string, logger logging.Logger) ([]fwupdToolDevice, error) {
	args := []string{"--plugins", "dell_dock", "--json", "get-devices"}
	output, err := runLoggedFwupdCommand(ctx, runner, env, toolPath, logger, args...)
	if err != nil {
		return nil, commandError("fwupdtool get-devices", output, err)
	}
	var response fwupdToolDevicesOutput
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("cannot parse fwupdtool get-devices JSON: %w", err)
	}
	return response.Devices, nil
}

func fwupdPostInstallVerification(ctx context.Context, runner CommandRunner, env []string, toolPath, deviceID string, candidate *domain.FirmwareCandidate, logger logging.Logger) (fwupdPostInstallResult, error) {
	if candidate == nil || len(candidate.ComponentVersions) == 0 {
		return fwupdPostInstallResult{}, nil
	}
	logUpdateEvent(logger, "INFO", "fwupdtool.postinstall.start", map[string]string{
		"device": deviceID,
	})
	devices, err := readFwupdDevices(ctx, runner, env, toolPath, logger)
	if err != nil {
		logUpdateEvent(logger, "ERROR", "fwupdtool.postinstall.complete", map[string]string{
			"device": deviceID,
			"error":  err.Error(),
		})
		return fwupdPostInstallResult{}, err
	}
	result := evaluateFwupdPostInstall(devices, deviceID, candidate.ComponentVersions)
	logUpdateEvent(logger, commandLogLevel(nil), "fwupdtool.postinstall.complete", map[string]string{
		"device": deviceID,
		"state":  string(result.State),
		"reason": result.Reason,
	})
	return result, nil
}

func evaluateFwupdPostInstall(devices []fwupdToolDevice, parentID string, candidateVersions map[string]string) fwupdPostInstallResult {
	targetDevices := make([]fwupdToolDevice, 0, len(devices))
	foundParent := false
	for _, device := range devices {
		if device.Plugin == "dell_dock" && fwupdDeviceBelongsTo(device, parentID) {
			targetDevices = append(targetDevices, device)
			if device.DeviceID == parentID {
				foundParent = true
			}
		}
	}
	if !foundParent {
		return fwupdPostInstallResult{Reason: "selected WD19 parent is no longer reported"}
	}
	for _, device := range targetDevices {
		if fwupdDeviceHasPendingUpdate(device) {
			return fwupdPostInstallResult{
				State:  fwupdPostInstallStaged,
				Reason: "fwupd reports an update-pending state",
			}
		}
	}
	for component, want := range candidateVersions {
		matches := make([]fwupdToolDevice, 0, 1)
		for _, device := range targetDevices {
			if fwupdDeviceComponent(device, parentID) == component {
				matches = append(matches, device)
			}
		}
		if len(matches) != 1 {
			return fwupdPostInstallResult{Reason: fmt.Sprintf("component %s was reported %d times", component, len(matches))}
		}
		got := strings.TrimSpace(matches[0].Version)
		if got != strings.TrimSpace(want) {
			return fwupdPostInstallResult{Reason: fmt.Sprintf("component %s is %s, want %s", component, got, want)}
		}
	}
	return fwupdPostInstallResult{
		State:  fwupdPostInstallVerified,
		Reason: "all candidate component versions match",
	}
}

func fwupdDeviceBelongsTo(device fwupdToolDevice, parentID string) bool {
	return device.DeviceID == parentID || device.ParentDeviceID == parentID || device.CompositeID == parentID
}

func fwupdDeviceHasPendingUpdate(device fwupdToolDevice) bool {
	for _, value := range append(append([]string{}, device.Flags...), device.Problems...) {
		if strings.Contains(strings.ToLower(strings.ReplaceAll(value, "_", "-")), "update-pending") {
			return true
		}
	}
	return false
}

func fwupdDeviceComponent(device fwupdToolDevice, parentID string) string {
	text := strings.ToLower(strings.Join([]string{device.Name, device.Summary, strings.Join(device.InstanceIDs, " ")}, " "))
	if device.DeviceID == parentID && hasWD19EmbeddedInstanceID(device.InstanceIDs) {
		return domain.FirmwareComponentEmbeddedController
	}
	switch {
	case strings.Contains(text, "package level") || strings.Contains(text, "&status"):
		return domain.FirmwareComponentPackage
	case strings.Contains(text, "rts5413") || strings.Contains(text, "generation 1") || strings.Contains(text, "pid_b06f"):
		return domain.FirmwareComponentUSBHubGen1
	case strings.Contains(text, "rts5487") || strings.Contains(text, "generation 2") || strings.Contains(text, "pid_b06e"):
		return domain.FirmwareComponentUSBHubGen2
	case strings.Contains(text, "vmm5331") || strings.Contains(text, "multi stream"):
		return domain.FirmwareComponentMST
	default:
		return ""
	}
}

func hasWD19EmbeddedInstanceID(instanceIDs []string) bool {
	for _, instanceID := range instanceIDs {
		if instanceID == "USB\\VID_413C&PID_B06E&hub&embedded" {
			return true
		}
	}
	return false
}

func commandError(command string, output []byte, err error) error {
	if text := strings.TrimSpace(string(output)); text != "" {
		return fmt.Errorf("%s: %w: %s", command, err, summarize(text))
	}
	return fmt.Errorf("%s: %w", command, err)
}

func runCommandWithEnv(ctx context.Context, runner CommandRunner, env []string, name string, args ...string) ([]byte, error) {
	if runnerWithEnv, ok := runner.(CommandRunnerWithEnv); ok {
		return runnerWithEnv.RunWithEnv(ctx, env, name, args...)
	}
	return runner.Run(ctx, name, args...)
}
