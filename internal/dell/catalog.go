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
	"github.com/fexxdev/dockwarden/internal/firmwareversion"
)

const maxDriverPageBytes = 4 * 1024 * 1024

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type CatalogClient struct {
	HTTP      HTTPDoer
	Sources   map[string]string
	Fallbacks map[string]domain.FirmwareCandidate
}

func PinnedWD19LinuxCandidate() domain.FirmwareCandidate {
	return domain.FirmwareCandidate{
		SourceURL:        "https://www.dell.com/support/home/en-us/drivers/driversdetails?driverid=389w0",
		PackageName:      "DellDockFirmwarePackage_WD19_WD22_HD22_WD25_SD25_01.01.11.cab",
		DownloadURL:      "https://dl.dell.com/FOLDER14009221M/1/DellDockFirmwarePackage_WD19_WD22_HD22_WD25_SD25_01.01.11.cab",
		Version:          "01.01.11.01, 01.01.11.01",
		ReleaseDate:      "19 Apr 2026",
		SHA256:           "f476fda34db1299da1c251bf04144d892a897a81fad0a40ee0c9771471f41614",
		Format:           "CAB",
		SupportedOS:      []string{"Linux"},
		CompatibleModels: []string{"Dell Dock WD19", "Dell Dock WD22", "Dell HD22", "Dell WD25", "Dell SD25"},
		ComponentVersions: map[string]string{
			domain.FirmwareComponentPackage:            "01.01.01.01",
			domain.FirmwareComponentEmbeddedController: "01.01.00.15",
			domain.FirmwareComponentUSBHubGen1:         "01.23",
			domain.FirmwareComponentUSBHubGen2:         "01.62",
			domain.FirmwareComponentMST:                "05.07.08",
		},
	}
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
	if !strings.EqualFold(format, "CAB") || !isCABDownloadURL(downloadURL) {
		return domain.FirmwareCandidate{}, fmt.Errorf("Dell dock firmware candidate is not a CAB package")
	}
	if !containsMetadataValue(supportedOSText, "linux") {
		return domain.FirmwareCandidate{}, fmt.Errorf("Dell dock firmware candidate does not support Linux")
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
	request.Header.Set("User-Agent", dellBrowserUserAgent)
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	request.Header.Set("Referer", "https://www.dell.com/")

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
	if response.StatusCode == http.StatusForbidden {
		if candidate, ok := c.fallbackCandidate(dock.Model, sourceURL); ok {
			return c.checkComponentVersions(dock, sourceURL, candidate, "Dell metadata page returned HTTP 403; using pinned official Dell package")
		}
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
	if err := c.inheritPinnedComponentVersions(dock.Model, &candidate); err != nil {
		return versionCheckUnavailable(sourceURL, &candidate, err.Error())
	}
	return c.checkComponentVersions(dock, sourceURL, &candidate, "compatible Dell package found")
}

func (c CatalogClient) fallbackCandidate(model, sourceURL string) (*domain.FirmwareCandidate, bool) {
	candidate, ok := c.Fallbacks[model]
	if !ok {
		return nil, false
	}
	if candidate.SourceURL == "" {
		candidate.SourceURL = sourceURL
	}
	if !isDellSupportURL(candidate.SourceURL) || !isDellSupportURL(candidate.DownloadURL) ||
		!isDockFirmwarePackage(candidate.PackageName) || !isSHA256(candidate.SHA256) ||
		!strings.EqualFold(candidate.Format, "CAB") || !isCABDownloadURL(candidate.DownloadURL) ||
		!containsValue(candidate.SupportedOS, "linux") ||
		!candidateMatchesModel(candidate, model) {
		return nil, false
	}
	return &candidate, true
}

func unavailable(reason string) domain.UpdateCheck {
	return domain.UpdateCheck{
		State:  "vendor_metadata_unavailable",
		Reason: reason,
	}
}

func (c CatalogClient) inheritPinnedComponentVersions(model string, candidate *domain.FirmwareCandidate) error {
	if candidate == nil {
		return fmt.Errorf("Dell metadata has no firmware candidate")
	}
	pinned, ok := c.fallbackCandidate(model, candidate.SourceURL)
	if !ok {
		return fmt.Errorf("no verified pinned component versions are configured for %s", model)
	}
	if !strings.EqualFold(candidate.SHA256, pinned.SHA256) {
		return fmt.Errorf("Dell candidate SHA-256 does not match the pinned WD19 component versions")
	}
	candidate.ComponentVersions = cloneComponentVersions(pinned.ComponentVersions)
	return nil
}

func (c CatalogClient) checkComponentVersions(dock *domain.Dock, sourceURL string, candidate *domain.FirmwareCandidate, reason string) domain.UpdateCheck {
	if dock == nil || candidate == nil {
		return versionCheckUnavailable(sourceURL, candidate, "firmware candidate or detected dock is missing")
	}
	currentVersions, err := wd19CurrentComponentVersions(dock.Firmware)
	if err != nil {
		return versionCheckUnavailable(sourceURL, candidate, err.Error())
	}
	for component := range candidate.ComponentVersions {
		if !isWD19Component(component) {
			return versionCheckUnavailable(sourceURL, candidate, "candidate has unsupported component "+component)
		}
	}
	updateAvailable := false
	for _, component := range wd19ComponentNames {
		candidateVersion, ok := candidate.ComponentVersions[component]
		if !ok || strings.TrimSpace(candidateVersion) == "" {
			return versionCheckUnavailable(sourceURL, candidate, "candidate has no "+component+" version")
		}
		comparison, compareErr := firmwareversion.Compare(candidateVersion, currentVersions[component])
		if compareErr != nil {
			return versionCheckUnavailable(sourceURL, candidate, "cannot compare "+component+" versions: "+compareErr.Error())
		}
		if comparison > 0 {
			updateAvailable = true
		}
	}
	if updateAvailable {
		return domain.UpdateCheck{
			State:     "update_available",
			SourceURL: sourceURL,
			Reason:    reason,
			Candidate: candidate,
		}
	}
	return domain.UpdateCheck{
		State:     "up_to_date",
		SourceURL: sourceURL,
		Reason:    "verified Dell CAB contains no component newer than the detected WD19 firmware",
		Candidate: candidate,
	}
}

func versionCheckUnavailable(sourceURL string, candidate *domain.FirmwareCandidate, reason string) domain.UpdateCheck {
	return domain.UpdateCheck{
		State:     "version_check_unavailable",
		SourceURL: sourceURL,
		Reason:    reason,
		Candidate: candidate,
	}
}

var wd19ComponentNames = []string{
	domain.FirmwareComponentPackage,
	domain.FirmwareComponentEmbeddedController,
	domain.FirmwareComponentUSBHubGen1,
	domain.FirmwareComponentUSBHubGen2,
	domain.FirmwareComponentMST,
}

func wd19CurrentComponentVersions(observations []domain.FirmwareObservation) (map[string]string, error) {
	current := make(map[string]string, len(wd19ComponentNames))
	for _, observation := range observations {
		component := strings.TrimSpace(observation.Component)
		if !isWD19Component(component) {
			continue
		}
		version := strings.TrimSpace(observation.Version)
		if version == "" {
			return nil, fmt.Errorf("detected %s version is missing", component)
		}
		if previous, exists := current[component]; exists && previous != version {
			return nil, fmt.Errorf("detected %s has conflicting versions %s and %s", component, previous, version)
		}
		current[component] = version
	}
	for _, component := range wd19ComponentNames {
		if current[component] == "" {
			return nil, fmt.Errorf("detected dock has no %s version", component)
		}
	}
	return current, nil
}

func isWD19Component(component string) bool {
	for _, known := range wd19ComponentNames {
		if component == known {
			return true
		}
	}
	return false
}

func cloneComponentVersions(versions map[string]string) map[string]string {
	clone := make(map[string]string, len(versions))
	for component, version := range versions {
		clone[component] = version
	}
	return clone
}

func isDellSupportURL(sourceURL string) bool {
	parsed, err := url.Parse(sourceURL)
	if err != nil || strings.ToLower(parsed.Scheme) != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "dell.com" || strings.HasSuffix(host, ".dell.com")
}

const dellBrowserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/17.0 Safari/605.1.15"

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

func isCABDownloadURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && strings.HasSuffix(strings.ToLower(parsed.Path), ".cab")
}

func containsMetadataValue(value, wanted string) bool {
	return containsValue(splitMetadata(value), wanted)
}

func containsValue(values []string, wanted string) bool {
	wanted = strings.ToLower(strings.TrimSpace(wanted))
	for _, value := range values {
		if strings.Contains(strings.ToLower(strings.TrimSpace(value)), wanted) {
			return true
		}
	}
	return false
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
