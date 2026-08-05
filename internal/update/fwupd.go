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
	"unicode/utf8"

	"github.com/fexxdev/dockwarden/internal/domain"
	"github.com/fexxdev/dockwarden/internal/logging"
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
	Logger  logging.Logger
}

func (u FwupdUpdater) Apply(ctx context.Context, dock *domain.Dock, candidate *domain.FirmwareCandidate) (result domain.UpdateCheck) {
	logUpdateEvent(u.Logger, "INFO", "firmware.apply.start", map[string]string{
		"backend": "fwupdmgr",
		"model":   dockModel(dock),
	})
	defer func() {
		level := "INFO"
		if result.State == "update_failed" {
			level = "ERROR"
		}
		logUpdateEvent(u.Logger, level, "firmware.apply.complete", map[string]string{
			"backend": "fwupdmgr",
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
	if !isDellDownloadURL(candidate.DownloadURL) {
		return failed(candidate, "candidate download URL is not an HTTPS Dell URL")
	}
	if !isSHA256(candidate.SHA256) {
		return failed(candidate, "candidate does not contain a valid SHA-256")
	}
	if !strings.EqualFold(candidate.Format, "CAB") || !isCABDownloadURL(candidate.DownloadURL) || !strings.HasSuffix(strings.ToLower(candidate.PackageName), ".cab") {
		return failed(candidate, "Linux firmware backend accepts only a Dell CAB package")
	}
	if !candidateSupports(candidate, "wd19", "linux") {
		return failed(candidate, "candidate does not explicitly support Dell Dock WD19 on Linux")
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
	args := []string{"local-install", payloadPath, "--assume-yes"}
	logUpdateEvent(u.Logger, "INFO", "fwupdmgr.command.start", map[string]string{
		"command": "fwupdmgr",
		"args":    strings.Join(args, " "),
	})
	output, err := runner.Run(ctx, "fwupdmgr", args...)
	logUpdateEvent(u.Logger, commandLogLevel(err), "fwupdmgr.command.complete", map[string]string{
		"command": "fwupdmgr",
		"output":  summarize(string(output)),
		"error":   errorText(err),
	})
	if err != nil {
		reason := "fwupdmgr: " + err.Error()
		if text := strings.TrimSpace(string(output)); text != "" {
			reason += ": " + summarize(text)
		}
		return failed(candidate, reason)
	}

	reason := "fwupdmgr accepted the verified Dell package; unplug and reconnect the dock USB-C cable, then run status"
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

func (u FwupdUpdater) download(ctx context.Context, candidate *domain.FirmwareCandidate) (string, error) {
	logUpdateEvent(u.Logger, "INFO", "firmware.download.start", map[string]string{
		"package": candidatePackage(candidate),
		"url":     candidateURL(candidate),
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.DownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("cannot create Dell firmware request: %w", err)
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/17.0 Safari/605.1.15")
	request.Header.Set("Referer", "https://www.dell.com/")
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
		logUpdateEvent(u.Logger, "ERROR", "firmware.download.failed", map[string]string{
			"package": candidatePackage(candidate),
			"reason":  "SHA-256 mismatch",
			"sha256":  actualHash,
		})
		return "", fmt.Errorf("Dell firmware SHA-256 mismatch: got %s", actualHash)
	}
	keep = true
	logUpdateEvent(u.Logger, "INFO", "firmware.download.complete", map[string]string{
		"package": candidatePackage(candidate),
		"sha256":  actualHash,
	})
	return payloadPath, nil
}

func dockModel(dock *domain.Dock) string {
	if dock == nil {
		return ""
	}
	return dock.Model
}

func candidatePackage(candidate *domain.FirmwareCandidate) string {
	if candidate == nil {
		return ""
	}
	return candidate.PackageName
}

func candidateURL(candidate *domain.FirmwareCandidate) string {
	if candidate == nil {
		return ""
	}
	return candidate.DownloadURL
}

func commandLogLevel(err error) string {
	if err != nil {
		return "ERROR"
	}
	return "INFO"
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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

func candidateSupports(candidate *domain.FirmwareCandidate, model, operatingSystem string) bool {
	if candidate == nil {
		return false
	}
	modelMatch := false
	for _, value := range candidate.CompatibleModels {
		if strings.Contains(strings.ToLower(value), strings.ToLower(model)) {
			modelMatch = true
			break
		}
	}
	osMatch := false
	for _, value := range candidate.SupportedOS {
		if strings.Contains(strings.ToLower(value), strings.ToLower(operatingSystem)) {
			osMatch = true
			break
		}
	}
	return modelMatch && osMatch
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
	const marker = "\n...[output truncated; head and tail kept]...\n"
	available := maxSummaryBytes - len(marker)
	headLength := available / 2
	tailLength := available - headLength
	for headLength > 0 && !utf8.RuneStart(text[headLength]) {
		headLength--
	}
	tailStart := len(text) - tailLength
	for tailStart < len(text) && !utf8.RuneStart(text[tailStart]) {
		tailStart++
	}
	return text[:headLength] + marker + text[tailStart:]
}

type systemRunner struct{}

func (systemRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func (systemRunner) RunWithEnv(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = append([]string(nil), env...)
	return command.CombinedOutput()
}
