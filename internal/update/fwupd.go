package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/fexxdev/dockwarden/internal/domain"
)

const maxFirmwarePayloadBytes = 64 * 1024 * 1024

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type FwupdUpdater struct {
	HTTP    HTTPDoer
	Runner  CommandRunner
	TempDir string
}

func (u FwupdUpdater) Apply(ctx context.Context, dock *domain.Dock, candidate *domain.FirmwareCandidate) domain.UpdateCheck {
	if !isSupportedWD19(dock) {
		return failed(candidate, "firmware backend accepts only the detected Dell Dock WD19 (413c:b06e)")
	}
	if candidate == nil {
		return failed(nil, "no firmware candidate was provided")
	}
	if !isDellDownloadURL(candidate.DownloadURL) {
		return failed(candidate, "candidate download URL is not an HTTPS Dell URL")
	}
	if !isSHA256(candidate.SHA256) {
		return failed(candidate, "candidate does not contain a valid SHA-256")
	}
	if u.HTTP == nil {
		return failed(candidate, "firmware HTTP client is not configured")
	}

	payloadPath, err := u.download(ctx, candidate)
	if err != nil {
		return failed(candidate, err.Error())
	}
	defer os.Remove(payloadPath)

	runner := u.Runner
	if runner == nil {
		runner = systemRunner{}
	}
	output, err := runner.Run(ctx, "fwupdmgr", "local-install", payloadPath, "--assume-yes")
	if err != nil {
		reason := "fwupdmgr: " + err.Error()
		if text := strings.TrimSpace(string(output)); text != "" {
			reason += ": " + summarize(text)
		}
		return failed(candidate, reason)
	}

	reason := "fwupdmgr accepted the verified Dell package"
	if text := strings.TrimSpace(string(output)); text != "" {
		reason = summarize(text)
	}
	return domain.UpdateCheck{
		State:     "update_applied",
		SourceURL: candidate.SourceURL,
		Reason:    reason,
		Candidate: candidate,
	}
}

func (u FwupdUpdater) download(ctx context.Context, candidate *domain.FirmwareCandidate) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.DownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("cannot create Dell firmware request: %w", err)
	}
	request.Header.Set("User-Agent", "dockwarden/0.2 (verified firmware updater)")
	response, err := u.HTTP.Do(request)
	if err != nil {
		return "", fmt.Errorf("cannot download Dell firmware: %w", err)
	}
	if response == nil {
		return "", fmt.Errorf("Dell firmware request returned no response")
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("Dell firmware download returned HTTP %d", response.StatusCode)
	}
	if response.Body == nil {
		return "", fmt.Errorf("Dell firmware response has no body")
	}
	if response.Request != nil && !isDellDownloadURL(response.Request.URL.String()) {
		return "", fmt.Errorf("Dell firmware download redirected to a non-Dell URL")
	}

	payload, err := os.CreateTemp(u.TempDir, "dockwarden-wd19-*.cab")
	if err != nil {
		return "", fmt.Errorf("cannot create temporary firmware file: %w", err)
	}
	payloadPath := payload.Name()
	keep := false
	defer func() {
		payload.Close()
		if !keep {
			os.Remove(payloadPath)
		}
	}()

	hasher := sha256.New()
	count, err := io.Copy(io.MultiWriter(payload, hasher), io.LimitReader(response.Body, maxFirmwarePayloadBytes+1))
	if err != nil {
		return "", fmt.Errorf("cannot save Dell firmware: %w", err)
	}
	if count > maxFirmwarePayloadBytes {
		return "", fmt.Errorf("Dell firmware payload exceeds %d bytes", maxFirmwarePayloadBytes)
	}
	if err := payload.Close(); err != nil {
		return "", fmt.Errorf("cannot close temporary firmware file: %w", err)
	}
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualHash, candidate.SHA256) {
		return "", fmt.Errorf("Dell firmware SHA-256 mismatch: got %s", actualHash)
	}
	keep = true
	return payloadPath, nil
}

func isSupportedWD19(dock *domain.Dock) bool {
	return dock != nil && dock.Model == "Dell Dock WD19" && dock.VendorID == 0x413c && dock.ProductID == 0xb06e
}

func isDellDownloadURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User != nil || strings.ToLower(parsed.Scheme) != "https" || parsed.Path == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "dell.com" || strings.HasSuffix(host, ".dell.com")
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func failed(candidate *domain.FirmwareCandidate, reason string) domain.UpdateCheck {
	result := domain.UpdateCheck{
		State:     "update_failed",
		Reason:    reason,
		Candidate: candidate,
	}
	if candidate != nil {
		result.SourceURL = candidate.SourceURL
	}
	return result
}

func summarize(text string) string {
	const maxSummaryBytes = 4096
	if len(text) <= maxSummaryBytes {
		return text
	}
	return text[:maxSummaryBytes] + "..."
}

type systemRunner struct{}

func (systemRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
