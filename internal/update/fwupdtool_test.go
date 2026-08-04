package update

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type fakeEnvironmentCommandRunner struct {
	calls  [][]string
	env    []string
	output []byte
	err    error
}

func (f *fakeEnvironmentCommandRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.output, f.err
}

func (f *fakeEnvironmentCommandRunner) RunWithEnv(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
	f.env = append([]string(nil), env...)
	return f.Run(context.Background(), name, args...)
}

func TestFwupdToolUpdaterVerifiesAndInvokesUpstreamTool(t *testing.T) {
	payload := []byte("verified firmware payload")
	httpClient := &fakeHTTPDoer{response: &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(payload)),
	}}
	runner := &fakeEnvironmentCommandRunner{output: []byte("Successfully installed")}
	updater := FwupdToolUpdater{
		HTTP:     httpClient,
		Runner:   runner,
		ToolPath: "/tmp/fwupdtool",
		TempDir:  t.TempDir(),
	}

	result := updater.Apply(context.Background(), matchingDock(), candidatePtr(candidateFor(payload)))
	if result.State != "update_staged" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one fwupdtool call, got %v", runner.calls)
	}
	call := runner.calls[0]
	if len(call) != 7 || call[0] != "/tmp/fwupdtool" || call[1] != "--plugins" || call[2] != "dell_dock" || call[3] != "--assume-yes" || call[4] != "--no-reboot-check" || call[5] != "install" {
		t.Fatalf("unexpected fwupdtool arguments: %v", call)
	}
	if _, err := os.Stat(call[6]); !os.IsNotExist(err) {
		t.Fatalf("expected temporary payload cleanup, stat error: %v", err)
	}
	if !hasEnvPrefix(runner.env, "FWUPD_LOCALSTATEDIR=") || !hasEnvPrefix(runner.env, "CACHE_DIRECTORY=") {
		t.Fatalf("fwupdtool state isolation was not configured: %v", runner.env)
	}
	if !strings.Contains(result.Reason, "reconnect") {
		t.Fatalf("expected reconnect instruction, got %q", result.Reason)
	}
}

func TestFwupdToolUpdaterRejectsInvalidCandidateBeforeDownload(t *testing.T) {
	httpClient := &fakeHTTPDoer{}
	runner := &fakeEnvironmentCommandRunner{}
	candidate := candidateFor([]byte("payload"))
	candidate.DownloadURL = strings.TrimSuffix(candidate.DownloadURL, ".cab") + ".exe"

	result := (FwupdToolUpdater{
		HTTP:     httpClient,
		Runner:   runner,
		ToolPath: "/tmp/fwupdtool",
		TempDir:  t.TempDir(),
	}).Apply(context.Background(), matchingDock(), &candidate)
	if result.State != "update_failed" || !strings.Contains(result.Reason, "CAB") {
		t.Fatalf("unexpected result: %+v", result)
	}
	if httpClient.request != nil || len(runner.calls) != 0 {
		t.Fatal("invalid candidate must stop before download and fwupdtool")
	}
}

func TestFwupdToolUpdaterStopsBeforeToolOnHashMismatch(t *testing.T) {
	httpClient := &fakeHTTPDoer{response: &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("tampered payload")),
	}}
	runner := &fakeEnvironmentCommandRunner{}
	candidate := candidateFor([]byte("expected payload"))

	result := (FwupdToolUpdater{
		HTTP:     httpClient,
		Runner:   runner,
		ToolPath: "/tmp/fwupdtool",
		TempDir:  t.TempDir(),
	}).Apply(context.Background(), matchingDock(), &candidate)
	if result.State != "update_failed" || !strings.Contains(result.Reason, "SHA-256") {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("fwupdtool must not run after hash mismatch: %v", runner.calls)
	}
}

func TestFwupdToolUpdaterReportsToolFailure(t *testing.T) {
	payload := []byte("verified payload")
	runner := &fakeEnvironmentCommandRunner{
		output: []byte("permission denied"),
		err:    os.ErrPermission,
	}
	result := (FwupdToolUpdater{
		HTTP: &fakeHTTPDoer{response: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(payload)),
		}},
		Runner:   runner,
		ToolPath: "/tmp/fwupdtool",
		TempDir:  t.TempDir(),
	}).Apply(context.Background(), matchingDock(), candidatePtr(candidateFor(payload)))
	if result.State != "update_failed" || !strings.Contains(result.Reason, "permission denied") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func hasEnvPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}
