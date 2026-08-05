package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fexxdev/dockwarden/internal/domain"
)

const testFwupdDeviceID = "0123456789abcdef0123456789abcdef01234567"

func TestManagedFwupdToolPrefix(t *testing.T) {
	prefix := os.Getenv("DOCKWARDEN_TEST_FWUPD_PREFIX")
	if prefix == "" {
		t.Skip("managed fwupdtool prefix was not provided")
	}
	if err := verifyFwupdTool(filepath.Join(prefix, "bin", "fwupdtool")); err != nil {
		t.Fatal(err)
	}
	if err := (FwupdToolUpdater{
		ToolPath: filepath.Join(prefix, "bin", "fwupdtool"),
		TempDir:  t.TempDir(),
	}).CheckReady(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type fakeEnvironmentCommandRunner struct {
	calls    [][]string
	envCalls [][]string
	respond  func(string, []string) ([]byte, error)
}

type fakeUpdateLogger struct {
	events []string
}

func (f *fakeUpdateLogger) Log(_ string, event string, _ map[string]string) error {
	f.events = append(f.events, event)
	return nil
}

func (f *fakeEnvironmentCommandRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.respond == nil {
		return nil, nil
	}
	return f.respond(name, args)
}

func (f *fakeEnvironmentCommandRunner) RunWithEnv(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	f.envCalls = append(f.envCalls, append([]string(nil), env...))
	return f.Run(ctx, name, args...)
}

type fakeMacPreflighter struct {
	calls     int
	candidate *domain.FirmwareCandidate
	result    MacPreflightResult
	err       error
}

func (f *fakeMacPreflighter) Check(_ context.Context, _ *domain.Dock, candidate *domain.FirmwareCandidate) (MacPreflightResult, error) {
	f.calls++
	f.candidate = candidate
	return f.result, f.err
}

func TestFwupdToolUpdaterAttestsTargetsAndBindsInstall(t *testing.T) {
	payload := []byte("verified firmware payload")
	configDir := t.TempDir()
	toolPath := writeManagedFwupdTool(t, configDir)
	logger := &fakeUpdateLogger{}
	preflight := &fakeMacPreflighter{result: MacPreflightResult{
		DeviceID:        testFwupdDeviceID,
		UpdateAvailable: true,
	}}
	runner := &fakeEnvironmentCommandRunner{respond: func(_ string, args []string) ([]byte, error) {
		switch {
		case isVersionCommand(args):
			return []byte(fwupdVersionJSON("2.2.1", "2.2.1")), nil
		case isGetDevicesCommand(args):
			return []byte(fwupdDevicesJSON(fwupdDeviceJSON(testFwupdDeviceID))), nil
		case isInstallCommand(args):
			return []byte("Successfully installed"), nil
		default:
			return nil, errors.New("unexpected fwupdtool command")
		}
	}}
	httpClient := &fakeHTTPDoer{response: &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(payload)),
	}}
	updater := FwupdToolUpdater{
		HTTP:      httpClient,
		Runner:    runner,
		ToolPath:  toolPath,
		ConfigDir: configDir,
		TempDir:   t.TempDir(),
		Preflight: preflight,
		Logger:    logger,
	}

	result := updater.Apply(context.Background(), matchingDock(), candidatePtr(candidateFor(payload)))
	if result.State != "update_staged" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if preflight.calls != 1 || preflight.candidate == nil {
		t.Fatalf("fwupd preflight did not run: %+v", preflight)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("fwupdtool calls = %v, want version and install", runner.calls)
	}
	if !isVersionCommand(runner.calls[0][1:]) {
		t.Fatalf("unexpected readiness call: %v", runner.calls[0])
	}
	install := runner.calls[1]
	if len(install) != 8 || install[0] != toolPath || install[5] != "install" || install[7] != testFwupdDeviceID {
		t.Fatalf("install was not bound to the selected full DeviceId: %v", install)
	}
	if _, err := os.Stat(install[6]); !os.IsNotExist(err) {
		t.Fatalf("expected temporary payload cleanup, stat error: %v", err)
	}
	if len(runner.envCalls) != 2 || !hasEnvPrefix(runner.envCalls[0], "FWUPD_LOCALSTATEDIR=") || !hasEnvPrefix(runner.envCalls[0], "CACHE_DIRECTORY=") {
		t.Fatalf("fwupdtool state isolation was not configured: %v", runner.envCalls)
	}
	if !strings.Contains(result.Reason, "reconnect") {
		t.Fatalf("expected reconnect instruction, got %q", result.Reason)
	}
	for _, event := range []string{"fwupd.ready", "firmware.download.complete", "macos.preflight.complete", "fwupdtool.command.start", "fwupdtool.command.complete", "firmware.apply.complete"} {
		found := false
		for _, got := range logger.events {
			if got == event {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing log event %q in %v", event, logger.events)
		}
	}
}

func TestFwupdToolUpdaterCheckReadyAttestsBeforeCandidateNetwork(t *testing.T) {
	configDir := t.TempDir()
	toolPath := writeManagedFwupdTool(t, configDir)
	runner := &fakeEnvironmentCommandRunner{respond: func(_ string, args []string) ([]byte, error) {
		if !isVersionCommand(args) {
			return nil, errors.New("unexpected fwupdtool command")
		}
		return []byte(fwupdVersionJSON("2.2.1", "2.2.1")), nil
	}}
	updater := FwupdToolUpdater{
		Runner:    runner,
		ToolPath:  toolPath,
		ConfigDir: configDir,
		TempDir:   t.TempDir(),
	}
	if err := updater.CheckReady(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || !isVersionCommand(runner.calls[0][1:]) {
		t.Fatalf("unexpected readiness calls: %v", runner.calls)
	}
	if len(runner.envCalls) != 1 || hasEnvPrefix(runner.envCalls[0], "FWUPD_SELF_TEST=") || !hasEnvPrefix(runner.envCalls[0], "PATH=/usr/bin:/bin") {
		t.Fatalf("readiness environment is not sanitized: %v", runner.envCalls)
	}
}

func TestFwupdToolUpdaterRejectsRunnerWithoutEnvironmentIsolation(t *testing.T) {
	configDir := t.TempDir()
	toolPath := writeManagedFwupdTool(t, configDir)
	httpClient := &fakeHTTPDoer{}
	result := (FwupdToolUpdater{
		HTTP:      httpClient,
		Runner:    &fakeCommandRunner{},
		ToolPath:  toolPath,
		ConfigDir: configDir,
		TempDir:   t.TempDir(),
	}).Apply(context.Background(), matchingDock(), candidatePtr(candidateFor([]byte("payload"))))
	if result.State != "update_failed" || !strings.Contains(result.Reason, "isolated environment") {
		t.Fatalf("unexpected runner result: %+v", result)
	}
	if httpClient.request != nil {
		t.Fatal("unsafe runner reached candidate network")
	}
}

func TestFwupdToolUpdaterRejectsUntrustedToolBeforeNetwork(t *testing.T) {
	payload := []byte("verified firmware payload")
	for _, test := range []struct {
		name      string
		configure func(t *testing.T, configDir string) string
	}{
		{
			name: "managed tool missing",
			configure: func(_ *testing.T, _ string) string {
				return ""
			},
		},
		{
			name: "relative override",
			configure: func(_ *testing.T, _ string) string {
				return "fwupdtool"
			},
		},
		{
			name: "absolute override missing",
			configure: func(_ *testing.T, configDir string) string {
				return filepath.Join(configDir, "missing", "fwupdtool")
			},
		},
		{
			name: "binary changed after manifest",
			configure: func(t *testing.T, configDir string) string {
				toolPath := writeManagedFwupdTool(t, configDir)
				if err := os.WriteFile(toolPath, []byte("changed"), 0o755); err != nil {
					t.Fatal(err)
				}
				return toolPath
			},
		},
		{
			name: "runtime library changed after manifest",
			configure: func(t *testing.T, configDir string) string {
				toolPath := writeManagedFwupdTool(t, configDir)
				libraryPath := filepath.Join(filepath.Dir(filepath.Dir(toolPath)), "lib", "fwupd-2.2.1", "libfwupdplugin.dylib")
				if err := os.WriteFile(libraryPath, []byte("changed plugin"), 0o644); err != nil {
					t.Fatal(err)
				}
				return toolPath
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			configDir := t.TempDir()
			httpClient := &fakeHTTPDoer{}
			runner := &fakeEnvironmentCommandRunner{}
			result := (FwupdToolUpdater{
				HTTP:      httpClient,
				Runner:    runner,
				ToolPath:  test.configure(t, configDir),
				ConfigDir: configDir,
				TempDir:   t.TempDir(),
			}).Apply(context.Background(), matchingDock(), candidatePtr(candidateFor(payload)))
			if result.State != "update_failed" {
				t.Fatalf("unexpected result: %+v", result)
			}
			if httpClient.request != nil || len(runner.calls) != 0 {
				t.Fatalf("untrusted tool accessed network or runner: request=%+v calls=%v", httpClient.request, runner.calls)
			}
		})
	}
}

func TestFwupdToolUpdaterUsesManagedDefaultPath(t *testing.T) {
	toolPath, err := (FwupdToolUpdater{ConfigDir: "/managed/config"}).resolveToolPath()
	if err != nil {
		t.Fatal(err)
	}
	want := "/managed/config/dockwarden/fwupd-2.2.1/bin/fwupdtool"
	if toolPath != want {
		t.Fatalf("managed tool path = %q, want %q", toolPath, want)
	}
}

func TestFwupdToolUpdaterRejectsWrongFwupdVersionsBeforeNetwork(t *testing.T) {
	for _, test := range []struct {
		name    string
		compile string
		runtime string
	}{
		{name: "wrong compile version", compile: "2.2.0", runtime: "2.2.1"},
		{name: "wrong runtime version", compile: "2.2.1", runtime: "2.2.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			configDir := t.TempDir()
			toolPath := writeManagedFwupdTool(t, configDir)
			httpClient := &fakeHTTPDoer{}
			runner := &fakeEnvironmentCommandRunner{respond: func(_ string, args []string) ([]byte, error) {
				if !isVersionCommand(args) {
					return nil, errors.New("unexpected fwupdtool command")
				}
				return []byte(fwupdVersionJSON(test.compile, test.runtime)), nil
			}}
			result := (FwupdToolUpdater{
				HTTP:      httpClient,
				Runner:    runner,
				ToolPath:  toolPath,
				ConfigDir: configDir,
				TempDir:   t.TempDir(),
			}).Apply(context.Background(), matchingDock(), candidatePtr(candidateFor([]byte("payload"))))
			if result.State != "update_failed" || !strings.Contains(result.Reason, "2.2.1") {
				t.Fatalf("unexpected version result: %+v", result)
			}
			if httpClient.request != nil || len(runner.calls) != 1 || !isVersionCommand(runner.calls[0][1:]) {
				t.Fatalf("wrong fwupd version did not stop before network: request=%+v calls=%v", httpClient.request, runner.calls)
			}
		})
	}
}

func TestFwupdToolUpdaterStopsBeforeInstallOnPreflightFailureOrNoUpdate(t *testing.T) {
	payload := []byte("verified firmware payload")
	for _, test := range []struct {
		name      string
		preflight fakeMacPreflighter
		wantState string
	}{
		{
			name:      "newer MST",
			preflight: fakeMacPreflighter{err: errors.New("MST candidate 05.07.09 is newer")},
			wantState: "update_failed",
		},
		{
			name: "no update",
			preflight: fakeMacPreflighter{result: MacPreflightResult{
				DeviceID: testFwupdDeviceID, UpdateAvailable: false,
			}},
			wantState: "up_to_date",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			configDir := t.TempDir()
			toolPath := writeManagedFwupdTool(t, configDir)
			runner := &fakeEnvironmentCommandRunner{respond: func(_ string, args []string) ([]byte, error) {
				switch {
				case isVersionCommand(args):
					return []byte(fwupdVersionJSON("2.2.1", "2.2.1")), nil
				case isGetDevicesCommand(args):
					return []byte(fwupdDevicesJSON(fwupdDeviceJSON(testFwupdDeviceID))), nil
				case isInstallCommand(args):
					return nil, errors.New("install must not run")
				default:
					return nil, errors.New("unexpected fwupdtool command")
				}
			}}
			result := (FwupdToolUpdater{
				HTTP: &fakeHTTPDoer{response: &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(payload)),
				}},
				Runner:    runner,
				ToolPath:  toolPath,
				ConfigDir: configDir,
				TempDir:   t.TempDir(),
				Preflight: &test.preflight,
			}).Apply(context.Background(), matchingDock(), candidatePtr(candidateFor(payload)))
			if result.State != test.wantState {
				t.Fatalf("unexpected preflight result: %+v", result)
			}
			if test.preflight.calls != 1 || containsInstallCall(runner.calls) {
				t.Fatalf("preflight did not stop install: preflight=%+v calls=%v", test.preflight, runner.calls)
			}
		})
	}
}

func TestFwupdToolUpdaterTreatsExitTwoAsUpdateFailure(t *testing.T) {
	payload := []byte("verified firmware payload")
	configDir := t.TempDir()
	toolPath := writeManagedFwupdTool(t, configDir)
	runner := &fakeEnvironmentCommandRunner{respond: func(_ string, args []string) ([]byte, error) {
		switch {
		case isVersionCommand(args):
			return []byte(fwupdVersionJSON("2.2.1", "2.2.1")), nil
		case isGetDevicesCommand(args):
			return []byte(fwupdDevicesJSON(fwupdDeviceJSON(testFwupdDeviceID))), nil
		case isInstallCommand(args):
			return []byte("nothing to do"), errors.New("exit status 2")
		default:
			return nil, errors.New("unexpected fwupdtool command")
		}
	}}
	result := (FwupdToolUpdater{
		HTTP: &fakeHTTPDoer{response: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(payload)),
		}},
		Runner:    runner,
		ToolPath:  toolPath,
		ConfigDir: configDir,
		TempDir:   t.TempDir(),
		Preflight: &fakeMacPreflighter{result: MacPreflightResult{DeviceID: testFwupdDeviceID, UpdateAvailable: true}},
	}).Apply(context.Background(), matchingDock(), candidatePtr(candidateFor(payload)))
	if result.State != "update_failed" || !strings.Contains(result.Reason, "exit status 2") {
		t.Fatalf("exit status 2 was accepted: %+v", result)
	}
}

func TestFwupdToolUpdaterVerifiesVersionsAfterInstallError(t *testing.T) {
	payload := []byte("verified firmware payload")
	configDir := t.TempDir()
	toolPath := writeManagedFwupdTool(t, configDir)
	candidate := candidateFor(payload)
	candidate.ComponentVersions = map[string]string{
		domain.FirmwareComponentPackage:            "01.01.01.01",
		domain.FirmwareComponentEmbeddedController: "01.01.00.15",
		domain.FirmwareComponentUSBHubGen1:         "01.23",
		domain.FirmwareComponentUSBHubGen2:         "01.62",
		domain.FirmwareComponentMST:                "05.07.08",
	}
	getDevicesCalls := 0
	runner := &fakeEnvironmentCommandRunner{respond: func(_ string, args []string) ([]byte, error) {
		switch {
		case isVersionCommand(args):
			return []byte(fwupdVersionJSON("2.2.1", "2.2.1")), nil
		case isGetDevicesCommand(args):
			getDevicesCalls++
			return []byte(fwupdDevicesJSON(fwupdVerifiedDevicesJSON(testFwupdDeviceID))), nil
		case isInstallCommand(args):
			return []byte("Writing…: 70.5%\nfinal HID timeout"), errors.New("exit status 1")
		default:
			return nil, errors.New("unexpected fwupdtool command")
		}
	}}
	result := (FwupdToolUpdater{
		HTTP: &fakeHTTPDoer{response: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(payload)),
		}},
		Runner:    runner,
		ToolPath:  toolPath,
		ConfigDir: configDir,
		TempDir:   t.TempDir(),
		Preflight: &fakeMacPreflighter{result: MacPreflightResult{DeviceID: testFwupdDeviceID, UpdateAvailable: true}},
	}).Apply(context.Background(), matchingDock(), &candidate)
	if result.State != "update_verified" {
		t.Fatalf("post-install versions were not verified: %+v", result)
	}
	if !strings.Contains(result.Reason, "verified") || !strings.Contains(result.Reason, "exit status 1") {
		t.Fatalf("verification reason lost the fwupd error: %q", result.Reason)
	}
	if getDevicesCalls != 1 {
		t.Fatalf("get-devices calls = %d, want post-install verification", getDevicesCalls)
	}
}

func TestEvaluateFwupdPostInstallKeepsPendingStateStaged(t *testing.T) {
	var response fwupdToolDevicesOutput
	if err := json.Unmarshal([]byte(fwupdDevicesJSON(fwupdVerifiedDevicesJSON(testFwupdDeviceID))), &response); err != nil {
		t.Fatal(err)
	}
	response.Devices[0].Problems = []string{"update-pending"}
	result := evaluateFwupdPostInstall(response.Devices, testFwupdDeviceID, testCandidateVersions())
	if result.State != fwupdPostInstallStaged {
		t.Fatalf("pending device was not kept staged: %+v", result)
	}
}

func TestEvaluateFwupdPostInstallRejectsMismatchedVersion(t *testing.T) {
	var response fwupdToolDevicesOutput
	if err := json.Unmarshal([]byte(fwupdDevicesJSON(fwupdVerifiedDevicesJSON(testFwupdDeviceID))), &response); err != nil {
		t.Fatal(err)
	}
	response.Devices[1].Version = "01.01.00.13-old"
	result := evaluateFwupdPostInstall(response.Devices, testFwupdDeviceID, testCandidateVersions())
	if result.State != "" || !strings.Contains(result.Reason, "embedded_controller") {
		t.Fatalf("mismatched version was accepted: %+v", result)
	}
}

func testCandidateVersions() map[string]string {
	return map[string]string{
		domain.FirmwareComponentPackage:            "01.01.01.01",
		domain.FirmwareComponentEmbeddedController: "01.01.00.15",
		domain.FirmwareComponentUSBHubGen1:         "01.23",
		domain.FirmwareComponentUSBHubGen2:         "01.62",
		domain.FirmwareComponentMST:                "05.07.08",
	}
}

func writeManagedFwupdTool(t *testing.T, configDir string) string {
	t.Helper()
	prefix := filepath.Join(configDir, "dockwarden", "fwupd-2.2.1")
	toolPath := filepath.Join(prefix, "bin", "fwupdtool")
	if err := os.MkdirAll(filepath.Dir(toolPath), 0o755); err != nil {
		t.Fatal(err)
	}
	runtimeFiles := map[string][]byte{
		"bin/fwupdtool":                         []byte("fwupdtool test binary"),
		"lib/fwupd-2.2.1/libfwupdcli.dylib":     []byte("cli"),
		"lib/fwupd-2.2.1/libfwupdengine.dylib":  []byte("engine"),
		"lib/fwupd-2.2.1/libfwupdplugin.dylib":  []byte("plugin"),
		"lib/libfwupd.3.dylib":                  []byte("client"),
		"share/fwupd/quirks.d/builtin.quirk.gz": []byte("quirks"),
		"etc/fwupd/fwupd.conf":                  []byte("config"),
	}
	runtimeHashes := make(map[string]string, len(runtimeFiles))
	for relativePath, content := range runtimeFiles {
		path := filepath.Join(prefix, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if relativePath == "bin/fwupdtool" {
			mode = 0o755
		}
		if err := os.WriteFile(path, content, mode); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(content)
		runtimeHashes[relativePath] = hex.EncodeToString(hash[:])
	}
	manifest := fwupdToolManifest{
		FwupdVersion:  "2.2.1",
		SourceCommit:  fwupdSourceCommit,
		BinarySHA256:  runtimeHashes["bin/fwupdtool"],
		RuntimeSHA256: runtimeHashes,
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prefix, "manifest.json"), manifestData, 0o644); err != nil {
		t.Fatal(err)
	}
	return toolPath
}

func fwupdVersionJSON(compile, runtime string) string {
	return "{\"Versions\":[" +
		"{\"Type\":\"compile\",\"AppstreamId\":\"org.freedesktop.fwupd\",\"Version\":\"" + compile + "\"}," +
		"{\"Type\":\"runtime\",\"AppstreamId\":\"org.freedesktop.fwupd\",\"Version\":\"" + runtime + "\"}" +
		"]}"
}

func fwupdDevicesJSON(devices string) string {
	return "{\"Devices\":[" + devices + "]}"
}

func fwupdDeviceJSON(deviceID string) string {
	return "{\"Plugin\":\"dell_dock\",\"InstanceIds\":[\"USB\\\\VID_413C&PID_B06E&hub&embedded\"],\"Serial\":\"TST0001/00002000\",\"DeviceId\":\"" + deviceID + "\"}"
}

func fwupdVerifiedDevicesJSON(parentID string) string {
	return strings.Join([]string{
		"{\"Name\":\"Package level of Dell dock\",\"ParentDeviceId\":\"" + parentID + "\",\"CompositeId\":\"" + parentID + "\",\"Plugin\":\"dell_dock\",\"InstanceIds\":[\"USB\\\\VID_413C&PID_B06E&hub&status\"],\"Version\":\"01.01.01.01\"}",
		"{\"Name\":\"WD19\",\"Plugin\":\"dell_dock\",\"Serial\":\"2000/00002000\",\"InstanceIds\":[\"USB\\\\VID_413C&PID_B06E&hub&embedded\"],\"DeviceId\":\"" + parentID + "\",\"Version\":\"01.01.00.15\"}",
		"{\"Name\":\"RTS5413 in Dell dock\",\"Summary\":\"USB 3.1 Generation 1 Hub\",\"ParentDeviceId\":\"" + parentID + "\",\"CompositeId\":\"" + parentID + "\",\"Plugin\":\"dell_dock\",\"InstanceIds\":[\"USB\\\\VID_413C&PID_B06F\"],\"Version\":\"01.23\"}",
		"{\"Name\":\"RTS5487 in Dell dock\",\"Summary\":\"USB 3.1 Generation 2 Hub\",\"ParentDeviceId\":\"" + parentID + "\",\"CompositeId\":\"" + parentID + "\",\"Plugin\":\"dell_dock\",\"InstanceIds\":[\"USB\\\\VID_413C&PID_B06E\"],\"Version\":\"01.62\"}",
		"{\"Name\":\"VMM5331 in Dell dock\",\"Summary\":\"Multi Stream Transport controller\",\"ParentDeviceId\":\"" + parentID + "\",\"CompositeId\":\"" + parentID + "\",\"Plugin\":\"dell_dock\",\"InstanceIds\":[\"MST-panamera-vmm5331-259\"],\"Version\":\"05.07.08\"}",
	}, ",")
}

func TestSummarizePreservesCommandTail(t *testing.T) {
	input := strings.Repeat("head-", 900) + "FINAL fwupd error: HID timeout"
	got := summarize(input)
	if !strings.Contains(got, "head-head-") {
		t.Fatalf("summary lost the command head: %q", got)
	}
	if !strings.Contains(got, "FINAL fwupd error: HID timeout") {
		t.Fatalf("summary lost the command tail: %q", got)
	}
	if !strings.Contains(got, "[output truncated") {
		t.Fatalf("summary does not identify truncation: %q", got)
	}
	if len(got) > 4096 {
		t.Fatalf("summary length = %d, want <= 4096", len(got))
	}
}

func isVersionCommand(args []string) bool {
	return len(args) == 2 && args[0] == "--version" && args[1] == "--json"
}

func isGetDevicesCommand(args []string) bool {
	return len(args) == 4 && args[0] == "--plugins" && args[1] == "dell_dock" && args[2] == "--json" && args[3] == "get-devices"
}

func isInstallCommand(args []string) bool {
	return len(args) >= 3 && args[len(args)-3] == "install"
}

func containsInstallCall(calls [][]string) bool {
	for _, call := range calls {
		if isInstallCommand(call[1:]) {
			return true
		}
	}
	return false
}

func hasEnvPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}
