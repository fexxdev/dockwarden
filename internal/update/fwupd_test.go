package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/fexxdev/dockwarden/internal/domain"
)

type fakeHTTPDoer struct {
	response *http.Response
	err      error
	request  *http.Request
}

func (f *fakeHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	f.request = request
	return f.response, f.err
}

type fakeCommandRunner struct {
	calls  [][]string
	output []byte
	err    error
}

func (f *fakeCommandRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)
	return f.output, f.err
}

func matchingDock() *domain.Dock {
	return &domain.Dock{
		Model:     "Dell Dock WD19",
		VendorID:  0x413c,
		ProductID: 0xb06e,
		Serial:    "2000",
		Devices: []domain.USBDevice{
			{Product: "Dell Dock WD19", Vendor: "Dell Inc.", VendorID: 0x413c, ProductID: 0xb06e, Serial: "2000", Location: "00150000"},
			{Product: "Dell dock", Vendor: "Dell Inc.", VendorID: 0x413c, ProductID: 0xb06f, Location: "00135000"},
		},
	}
}

func candidateFor(payload []byte) domain.FirmwareCandidate {
	hash := sha256.Sum256(payload)
	return domain.FirmwareCandidate{
		SourceURL:        "https://www.dell.com/support/home/en-us/drivers/driversdetails?driverid=389w0",
		PackageName:      "DellDockFirmwarePackage_WD19_WD22_01.01.04.cab",
		DownloadURL:      "https://dl.dell.com/DellDockFirmwarePackage_WD19_WD22_01.01.04.cab",
		Version:          "01.01.00.01, 01.01.04.01",
		Format:           "CAB",
		SupportedOS:      []string{"Linux"},
		CompatibleModels: []string{"Dell Dock WD19"},
		SHA256:           hex.EncodeToString(hash[:]),
	}
}

func TestFwupdUpdaterRejectsNonCABBeforeDownload(t *testing.T) {
	payload := []byte("verified payload")
	httpClient := &fakeHTTPDoer{}
	runner := &fakeCommandRunner{}
	candidate := candidateFor(payload)
	candidate.Format = "Application"
	candidate.DownloadURL = strings.TrimSuffix(candidate.DownloadURL, ".cab") + ".exe"
	result := (FwupdUpdater{HTTP: httpClient, Runner: runner, TempDir: t.TempDir()}).Apply(context.Background(), matchingDock(), &candidate)
	if result.State != "update_failed" || !strings.Contains(result.Reason, "CAB") {
		t.Fatalf("unexpected format failure: %+v", result)
	}
	if httpClient.request != nil || len(runner.calls) != 0 {
		t.Fatal("non-CAB candidate must fail before download and fwupdmgr")
	}
}

func TestFwupdUpdaterVerifiesAndInstallsDellPayload(t *testing.T) {
	payload := []byte("verified firmware payload")
	httpClient := &fakeHTTPDoer{response: &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(payload)),
	}}
	runner := &fakeCommandRunner{output: []byte("Successfully installed")}
	updater := FwupdUpdater{
		HTTP:    httpClient,
		Runner:  runner,
		TempDir: t.TempDir(),
	}

	result := updater.Apply(context.Background(), matchingDock(), candidatePtr(candidateFor(payload)))
	if result.State != "update_staged" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !strings.Contains(result.Reason, "reconnect") {
		t.Fatalf("expected reconnect instruction, got: %s", result.Reason)
	}
	if len(runner.calls) != 1 || len(runner.calls[0]) != 4 || runner.calls[0][0] != "fwupdmgr" || runner.calls[0][1] != "local-install" || runner.calls[0][3] != "--assume-yes" {
		t.Fatalf("unexpected fwupdmgr call: %v", runner.calls)
	}
	if _, err := os.Stat(runner.calls[0][2]); !os.IsNotExist(err) {
		t.Fatalf("expected temporary payload cleanup, stat error: %v", err)
	}
	if httpClient.request == nil || httpClient.request.URL.String() != candidateFor(payload).DownloadURL {
		t.Fatalf("unexpected download request: %+v", httpClient.request)
	}
}

func TestFwupdUpdaterUsesBrowserHeadersForDellDownload(t *testing.T) {
	payload := []byte("browser-compatible payload")
	httpClient := &fakeHTTPDoer{response: &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(payload)),
	}}
	runner := &fakeCommandRunner{}
	candidate := candidateFor(payload)
	result := (FwupdUpdater{HTTP: httpClient, Runner: runner, TempDir: t.TempDir()}).Apply(context.Background(), matchingDock(), &candidate)
	if result.State != "update_staged" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if httpClient.request == nil {
		t.Fatal("expected a Dell download request")
	}
	if httpClient.request.Header.Get("User-Agent") == "" {
		t.Fatal("Dell download must send a browser user agent")
	}
	if httpClient.request.Header.Get("Referer") != "https://www.dell.com/" {
		t.Fatalf("unexpected Dell download referer: %q", httpClient.request.Header.Get("Referer"))
	}
}

func TestFwupdUpdaterRejectsUnsupportedCandidateBeforeDownload(t *testing.T) {
	cases := []struct {
		name string
		edit func(*domain.FirmwareCandidate)
	}{
		{
			name: "non-linux",
			edit: func(candidate *domain.FirmwareCandidate) {
				candidate.SupportedOS = []string{"Windows"}
			},
		},
		{
			name: "wrong-model",
			edit: func(candidate *domain.FirmwareCandidate) {
				candidate.CompatibleModels = []string{"Dell Dock WD22"}
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			httpClient := &fakeHTTPDoer{}
			runner := &fakeCommandRunner{}
			candidate := candidateFor([]byte("payload"))
			testCase.edit(&candidate)
			result := (FwupdUpdater{HTTP: httpClient, Runner: runner, TempDir: t.TempDir()}).Apply(context.Background(), matchingDock(), &candidate)
			if result.State != "update_failed" || !strings.Contains(result.Reason, "WD19") {
				t.Fatalf("unexpected candidate failure: %+v", result)
			}
			if httpClient.request != nil || len(runner.calls) != 0 {
				t.Fatal("unsupported candidate must fail before download and fwupdmgr")
			}
		})
	}
}

func TestFwupdUpdaterStopsBeforeInstallOnHashMismatch(t *testing.T) {
	httpClient := &fakeHTTPDoer{response: &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("tampered payload")),
	}}
	runner := &fakeCommandRunner{}
	candidate := candidateFor([]byte("expected payload"))
	result := (FwupdUpdater{HTTP: httpClient, Runner: runner, TempDir: t.TempDir()}).Apply(context.Background(), matchingDock(), &candidate)
	if result.State != "update_failed" || !strings.Contains(result.Reason, "SHA-256") {
		t.Fatalf("unexpected hash failure: %+v", result)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("fwupdmgr must not run after hash mismatch: %v", runner.calls)
	}
}

func TestFwupdUpdaterRejectsNonWD19BeforeDownload(t *testing.T) {
	httpClient := &fakeHTTPDoer{}
	runner := &fakeCommandRunner{}
	candidate := candidateFor([]byte("payload"))
	result := (FwupdUpdater{HTTP: httpClient, Runner: runner, TempDir: t.TempDir()}).Apply(context.Background(), &domain.Dock{Model: "Dell Dock WD22", VendorID: 0x413c, ProductID: 0xb06e}, &candidate)
	if result.State != "update_failed" || !strings.Contains(result.Reason, "WD19") {
		t.Fatalf("unexpected identity failure: %+v", result)
	}
	if httpClient.request != nil || len(runner.calls) != 0 {
		t.Fatal("non-WD19 input must not access the payload or runner")
	}
}

func TestFwupdUpdaterReportsFwupdFailure(t *testing.T) {
	payload := []byte("verified payload")
	httpClient := &fakeHTTPDoer{response: &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(payload)),
	}}
	runner := &fakeCommandRunner{output: []byte("permission denied"), err: errors.New("exit status 1")}
	candidate := candidateFor(payload)
	result := (FwupdUpdater{HTTP: httpClient, Runner: runner, TempDir: t.TempDir()}).Apply(context.Background(), matchingDock(), &candidate)
	if result.State != "update_failed" || !strings.Contains(result.Reason, "permission denied") {
		t.Fatalf("unexpected fwupd failure: %+v", result)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected fwupdmgr call: %v", runner.calls)
	}
}

func TestSystemRunnerRunWithEnvDoesNotInheritProcessEnvironment(t *testing.T) {
	t.Setenv("FWUPD_SELF_TEST", "unsafe")
	t.Setenv("DYLD_LIBRARY_PATH", "/tmp/unsafe")
	output, err := (systemRunner{}).RunWithEnv(context.Background(), []string{
		"PATH=/usr/bin:/bin",
		"LANG=C",
	}, "/usr/bin/env")
	if err != nil {
		t.Fatal(err)
	}
	text := string(output)
	if strings.Contains(text, "FWUPD_SELF_TEST=") || strings.Contains(text, "DYLD_LIBRARY_PATH=") {
		t.Fatalf("unsafe environment was inherited: %s", text)
	}
	if !strings.Contains(text, "PATH=/usr/bin:/bin") || !strings.Contains(text, "LANG=C") {
		t.Fatalf("explicit environment is missing: %s", text)
	}
}

func candidatePtr(candidate domain.FirmwareCandidate) *domain.FirmwareCandidate {
	return &candidate
}
