package dell

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/fexxdev/dockwarden/internal/domain"
)

const maxDriverPageBytes = 4 * 1024 * 1024

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type CatalogClient struct {
	HTTP    HTTPDoer
	Sources map[string]string
}

func ParseDriverPage(sourceURL string, page []byte) (domain.FirmwareCandidate, error) {
	if !isDellSupportURL(sourceURL) {
		return domain.FirmwareCandidate{}, fmt.Errorf("source is not an HTTPS dell.com page")
	}

	text := normalizeHTML(page)
	packageName := firstField(text, "File Name", "Dateiname", "Nome file")
	version := firstField(text, "Version")
	releaseDate := firstField(text, "Release Date", "Veröffentlichungsdatum", "Data di rilascio")
	checksum := firstField(text, "SHA-256", "SHA256", "Checksum")
	compatibleText := metadataField(text,
		[]string{"Compatible Systems", "Supported Systems", "Kompatible Systeme", "Sistemi compatibili"},
		[]string{"Supported Operating Systems", "Operating Systems", "Unterstützte Betriebssysteme", "Sistemi operativi supportati", "Installation instructions", "Installationsanweisungen", "Istruzioni di installazione", "Applies to", "Gilt für", "Si applica a"},
	)
	supportedOSText := metadataField(text,
		[]string{"Operating Systems", "Supported Operating Systems", "Supported OS", "Unterstützte Betriebssysteme", "Sistemi operativi supportati"},
		[]string{"Compatible Systems", "Supported Systems", "Kompatible Systeme", "Sistemi compatibili", "Installation instructions", "Installationsanweisungen", "Istruzioni di installazione", "Applies to", "Gilt für", "Si applica a"},
	)
	format := firstField(text, "File Format", "Dateiformat", "Formato file")

	if packageName == "" || !isDockFirmwarePackage(packageName) {
		return domain.FirmwareCandidate{}, fmt.Errorf("page has no compatible Dell dock firmware package")
	}
	downloadURL, err := downloadURLForPackage(page, packageName)
	if err != nil {
		return domain.FirmwareCandidate{}, err
	}
	if !isSimpleVersion(version) {
		return domain.FirmwareCandidate{}, fmt.Errorf("page has no usable firmware version")
	}
	if releaseDate == "" {
		return domain.FirmwareCandidate{}, fmt.Errorf("page has no release date")
	}
	if !isSHA256(checksum) {
		return domain.FirmwareCandidate{}, fmt.Errorf("page has no valid SHA-256")
	}
	compatibleModels := splitMetadata(compatibleText)
	if len(compatibleModels) == 0 {
		return domain.FirmwareCandidate{}, fmt.Errorf("page has no compatible systems")
	}
	if format == "" {
		format = "unknown"
	}

	return domain.FirmwareCandidate{
		SourceURL:        sourceURL,
		PackageName:      packageName,
		DownloadURL:      downloadURL,
		Version:          version,
		ReleaseDate:      releaseDate,
		SHA256:           strings.ToLower(checksum),
		Format:           format,
		SupportedOS:      splitMetadata(supportedOSText),
		CompatibleModels: compatibleModels,
	}, nil
}

func (c CatalogClient) Check(ctx context.Context, dock *domain.Dock) domain.UpdateCheck {
	if dock == nil {
		return domain.UpdateCheck{
			State:  "not_checked",
			Reason: "dock not detected",
		}
	}
	sourceURL := c.Sources[dock.Model]
	if sourceURL == "" {
		return unavailable("no official Dell source is configured for " + dock.Model)
	}
	if !isDellSupportURL(sourceURL) {
		return unavailable("configured source is not an HTTPS dell.com page")
	}
	if c.HTTP == nil {
		return unavailable("HTTP client is not configured")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return unavailable("cannot create Dell metadata request: " + err.Error())
	}
	request.Header.Set("User-Agent", "dockwarden/0.1 (read-only firmware metadata)")
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")

	response, err := c.HTTP.Do(request)
	if err != nil {
		return unavailable(err.Error())
	}
	if response == nil {
		return unavailable("Dell metadata request returned no response")
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return unavailable(fmt.Sprintf("Dell metadata returned HTTP %d", response.StatusCode))
	}
	if response.Body == nil {
		return unavailable("Dell metadata response has no body")
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxDriverPageBytes+1))
	if err != nil {
		return unavailable("cannot read Dell metadata: " + err.Error())
	}
	if len(body) > maxDriverPageBytes {
		return unavailable("Dell metadata page is too large")
	}

	candidate, err := ParseDriverPage(sourceURL, body)
	if err != nil {
		return unavailable(err.Error())
	}
	if !candidateMatchesModel(candidate, dock.Model) {
		return unavailable("Dell metadata does not list the detected dock model")
	}

	return domain.UpdateCheck{
		State:     "update_available",
		SourceURL: sourceURL,
		Reason:    "compatible Dell package found; current dock firmware was not read",
		Candidate: &candidate,
	}
}

func unavailable(reason string) domain.UpdateCheck {
	return domain.UpdateCheck{
		State:  "vendor_metadata_unavailable",
		Reason: reason,
	}
}

func isDellSupportURL(sourceURL string) bool {
	parsed, err := url.Parse(sourceURL)
	if err != nil || strings.ToLower(parsed.Scheme) != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "dell.com" || strings.HasSuffix(host, ".dell.com")
}

var htmlTagPattern = regexp.MustCompile("<[^>]*>")
var hrefPattern = regexp.MustCompile(`(?is)<a\b[^>]*\bhref\s*=\s*["']([^"']+)["'][^>]*>`)

func normalizeHTML(page []byte) string {
	text := htmlTagPattern.ReplaceAllString(string(page), "\n")
	text = html.UnescapeString(text)
	lines := strings.Split(text, "\n")
	var normalized []string
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			normalized = append(normalized, line)
		}
	}
	return strings.Join(normalized, "\n")
}

func firstField(text string, labels ...string) string {
	for _, label := range labels {
		pattern := regexp.MustCompile("(?i)^\\s*" + regexp.QuoteMeta(label) + "\\s*:\\s*(.*)$")
		lines := strings.Split(text, "\n")
		for index, line := range lines {
			matches := pattern.FindStringSubmatch(line)
			if len(matches) != 2 {
				continue
			}
			value := strings.TrimSpace(matches[1])
			if value != "" {
				return value
			}
			for _, next := range lines[index+1:] {
				next = strings.TrimSpace(next)
				if next != "" {
					return next
				}
			}
		}
	}
	return ""
}

func splitMetadata(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '\n' || r == '\r'
	})
	var result []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func isDockFirmwarePackage(packageName string) bool {
	text := strings.ToLower(packageName)
	return strings.Contains(text, "dell") &&
		strings.Contains(text, "dock") &&
		(strings.Contains(text, "wd19") || strings.Contains(text, "wd22"))
}

var versionPattern = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._-]*(\\s*,\\s*[A-Za-z0-9][A-Za-z0-9._-]*)*$")
var sha256Pattern = regexp.MustCompile("^[0-9a-fA-F]{64}$")

func isSimpleVersion(value string) bool {
	return versionPattern.MatchString(strings.TrimSpace(value))
}

func isSHA256(value string) bool {
	return sha256Pattern.MatchString(strings.TrimSpace(value))
}

func downloadURLForPackage(page []byte, packageName string) (string, error) {
	for _, matches := range hrefPattern.FindAllStringSubmatch(string(page), -1) {
		if len(matches) != 2 {
			continue
		}
		href := strings.TrimSpace(html.UnescapeString(matches[1]))
		if !strings.Contains(strings.ToLower(href), strings.ToLower(packageName)) {
			continue
		}
		if !isDellSupportURL(href) {
			return "", fmt.Errorf("page has an unsafe Dell firmware download URL")
		}
		return href, nil
	}
	return "", fmt.Errorf("page has no download URL for %s", packageName)
}

func metadataField(text string, labels, stops []string) string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		value, ok := matchFieldLine(line, labels)
		if !ok {
			continue
		}

		values := make([]string, 0, 4)
		if value != "" {
			values = append(values, value)
		}
		for _, next := range lines[index+1:] {
			next = strings.TrimSpace(next)
			if next == "" {
				continue
			}
			if _, stop := matchFieldLine(next, stops); stop {
				break
			}
			values = append(values, next)
		}
		return strings.Join(values, "\n")
	}
	return ""
}

func matchFieldLine(line string, labels []string) (string, bool) {
	for _, label := range labels {
		pattern := regexp.MustCompile("(?i)^\\s*" + regexp.QuoteMeta(label) + "\\s*(?::\\s*(.*))?$")
		matches := pattern.FindStringSubmatch(line)
		if len(matches) == 2 {
			return strings.TrimSpace(matches[1]), true
		}
	}
	return "", false
}

func candidateMatchesModel(candidate domain.FirmwareCandidate, model string) bool {
	model = strings.ToLower(model)
	for _, compatible := range candidate.CompatibleModels {
		compatible = strings.ToLower(compatible)
		if strings.Contains(model, "wd19") && strings.Contains(compatible, "wd19") {
			return true
		}
		if strings.Contains(model, "wd22") && strings.Contains(compatible, "wd22") {
			return true
		}
	}
	return false
}
