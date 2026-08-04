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
	}
}

func candidateFor(payload []byte) domain.FirmwareCandidate {
	hash := sha256.Sum256(payload)
	return domain.FirmwareCandidate{
		SourceURL:   "https://www.dell.com/support/home/en-us/drivers/driversdetails?driverid=4p6vj",
		PackageName: "DellDockFirmwarePackage_WD19_WD22_01.01.04.cab",
		DownloadURL: "https://dl.dell.com/DellDockFirmwarePackage_WD19_WD22_01.01.04.cab",
		Version:     "01.01.00.01, 01.01.04.01",
		SHA256:      hex.EncodeToString(hash[:]),
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
	if result.State != "update_applied" {
		t.Fatalf("unexpected result: %+v", result)
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

func candidatePtr(candidate domain.FirmwareCandidate) *domain.FirmwareCandidate {
	return &candidate
}
