package update

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fexxdev/dockwarden/internal/domain"
)

// FwupdToolEnvironmentVariable selects the standalone fwupdtool binary on macOS.
const FwupdToolEnvironmentVariable = "DOCKWARDEN_FWUPDTOOL"

type CommandRunnerWithEnv interface {
	RunWithEnv(context.Context, []string, string, ...string) ([]byte, error)
}

type FwupdToolUpdater struct {
	HTTP     HTTPDoer
	Runner   CommandRunner
	ToolPath string
	TempDir  string
}

func (u FwupdToolUpdater) Apply(ctx context.Context, dock *domain.Dock, candidate *domain.FirmwareCandidate) domain.UpdateCheck {
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

	payloadPath, err := (FwupdUpdater{HTTP: u.HTTP, TempDir: u.TempDir}).download(ctx, candidate)
	if err != nil {
		return failed(candidate, err.Error())
	}
	defer os.Remove(payloadPath)

	stateDir, err := os.MkdirTemp(u.TempDir, "dockwarden-fwupd-state-*")
	if err != nil {
		return failed(candidate, fmt.Sprintf("cannot create temporary fwupd state: %v", err))
	}
	defer os.RemoveAll(stateDir)

	toolPath := strings.TrimSpace(u.ToolPath)
	if toolPath == "" {
		toolPath = "fwupdtool"
	}
	runner := u.Runner
	if runner == nil {
		runner = systemRunner{}
	}
	env := []string{
		"FWUPD_LOCALSTATEDIR=" + stateDir,
		"CACHE_DIRECTORY=" + filepath.Join(stateDir, "cache"),
	}
	args := []string{
		"--plugins",
		"dell_dock",
		"--assume-yes",
		"--no-reboot-check",
		"install",
		payloadPath,
	}
	output, err := runCommandWithEnv(ctx, runner, env, toolPath, args...)
	if err != nil {
		reason := "fwupdtool: " + err.Error()
		if text := strings.TrimSpace(string(output)); text != "" {
			reason += ": " + summarize(text)
		}
		return failed(candidate, reason)
	}

	reason := "fwupdtool accepted the verified Dell package; unplug and reconnect the dock USB-C cable, then run status"
	if text := strings.TrimSpace(string(output)); text != "" {
		reason = summarize(text) + "; unplug and reconnect the dock USB-C cable, then run status"
	}
	return domain.UpdateCheck{
		State:     "update_staged",
		SourceURL: candidate.SourceURL,
		Reason:    reason,
		Candidate: candidate,
	}
}

func runCommandWithEnv(ctx context.Context, runner CommandRunner, env []string, name string, args ...string) ([]byte, error) {
	if runnerWithEnv, ok := runner.(CommandRunnerWithEnv); ok {
		return runnerWithEnv.RunWithEnv(ctx, env, name, args...)
	}
	return runner.Run(ctx, name, args...)
}
