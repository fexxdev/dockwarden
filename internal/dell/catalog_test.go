package dell

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fexxdev/dockwarden/internal/domain"
)

const wd19SourceURL = "https://www.dell.com/support/home/en-us/drivers/driversdetails?driverid=389w0"

type fakeHTTPDoer struct {
	response *http.Response
	err      error
	request  *http.Request
}

func (f *fakeHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	f.request = request
	return f.response, f.err
}

func loadDriverPage(t *testing.T) []byte {
	t.Helper()
	page, err := os.ReadFile(filepath.Join("testdata", "wd19-driver-page.html"))
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestParseDriverPage(t *testing.T) {
	candidate, err := ParseDriverPage(wd19SourceURL, loadDriverPage(t))
	if err != nil {
		t.Fatal(err)
	}
	if candidate.PackageName != "DellDockFirmwarePackage_WD19_WD22_FW_01.00.36.cab" ||
		candidate.Version != "01.00.36" ||
		candidate.ReleaseDate != "15 Apr 2026" ||
		candidate.DownloadURL != "https://dl.dell.com/DellDockFirmwarePackage_WD19_WD22_FW_01.00.36.cab" {
		t.Fatalf("unexpected candidate identity: %+v", candidate)
	}
	if candidate.SHA256 != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected checksum: %+v", candidate)
	}
	if candidate.Format != "CAB" || len(candidate.SupportedOS) != 3 ||
		len(candidate.CompatibleModels) != 2 {
		t.Fatalf("unexpected candidate metadata: %+v", candidate)
	}
}

func TestParseDriverPageRejectsNonCABFirmware(t *testing.T) {
	page := []byte(`<div>Version: 01.00.36</div><div>Release Date: 15 Apr 2026</div><div>File Name: DellDock_WD19_FW.exe</div><div>File Format: Application</div><div>SHA-256: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef</div><div>Operating Systems: Linux</div><div>Compatible Systems: Dell Dock WD19</div><a href="https://dl.dell.com/DellDock_WD19_FW.exe">download</a>`)
	if _, err := ParseDriverPage(wd19SourceURL, page); err == nil || !strings.Contains(err.Error(), "CAB") {
		t.Fatalf("expected non-CAB rejection, got %v", err)
	}
}

func TestParseDriverPageRequiresHash(t *testing.T) {
	page := []byte("<html><div>Version: 01.00.36</div><div>Release Date: 15 Apr 2026</div><div>File Name: DellDock_WD19_FW.exe</div><div>Compatible Systems: Dell Dock WD19</div></html>")
	if _, err := ParseDriverPage(wd19SourceURL, page); err == nil {
		t.Fatal("expected missing hash error")
	}
}

func TestParseDriverPageHandlesComponentVersionsAndLists(t *testing.T) {
	page := []byte("<div>Version : 01.01.11.01</div><div>Release Date : 19 Apr 2026</div><div>File Name : DellDockFirmwarePackage_WD19_WD22_01.01.11.cab</div><div>File Format : CAB</div><div>SHA-256 : 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef</div><h2>Compatible Systems</h2><a>Dell 14 D14260</a><a>Dell Dock WD19</a><h2>Supported Operating Systems</h2><p>Linux</p><a href=\"https://dl.dell.com/DellDockFirmwarePackage_WD19_WD22_01.01.11.cab\">download</a>")
	candidate, err := ParseDriverPage(wd19SourceURL, page)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Version != "01.01.11.01" {
		t.Fatalf("unexpected component versions: %+v", candidate)
	}
	foundWD19 := false
	for _, model := range candidate.CompatibleModels {
		if model == "Dell Dock WD19" {
			foundWD19 = true
		}
	}
	if !foundWD19 {
		t.Fatalf("WD19 compatibility was lost: %+v", candidate)
	}
}

func TestParseDriverPageRejectsUnsafeDownloadURL(t *testing.T) {
	page := []byte(`<div>Version: 01.00.36</div><div>Release Date: 15 Apr 2026</div><div>File Name: DellDock_WD19_FW.exe</div><div>SHA-256: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef</div><div>Compatible Systems: Dell Dock WD19</div><a href="http://example.com/DellDock_WD19_FW.exe">download</a>`)
	if _, err := ParseDriverPage(wd19SourceURL, page); err == nil {
		t.Fatal("expected unsafe download URL error")
	}
}

func TestCatalogCheckAttestsComponentVersionsOnlyAfterHashMatch(t *testing.T) {
	pinned := PinnedWD19LinuxCandidate()
	for _, test := range []struct {
		name      string
		page      []byte
		wantState string
		wantMap   bool
	}{
		{
			name:      "matching hash",
			page:      []byte(strings.Replace(string(loadDriverPage(t)), "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", pinned.SHA256, 1)),
			wantState: "update_available",
			wantMap:   true,
		},
		{
			name:      "different hash",
			page:      loadDriverPage(t),
			wantState: "version_check_unavailable",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			httpClient := &fakeHTTPDoer{response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(test.page)),
			}}
			client := CatalogClient{
				HTTP: httpClient,
				Sources: map[string]string{
					"Dell Dock WD19": wd19SourceURL,
				},
				Fallbacks: map[string]domain.FirmwareCandidate{
					"Dell Dock WD19": pinned,
				},
			}
			result := client.Check(context.Background(), wd19DockWithFirmware(wd19CurrentFirmware()))
			if result.State != test.wantState || result.SourceURL != wd19SourceURL || result.Candidate == nil {
				t.Fatalf("unexpected update result: %+v", result)
			}
			if test.wantMap && result.Candidate.ComponentVersions["package"] != "01.01.01.01" {
				t.Fatalf("matching candidate did not inherit component versions: %+v", result.Candidate)
			}
			if httpClient.request == nil || httpClient.request.Header.Get("User-Agent") == "" {
				t.Fatal("expected a descriptive user agent")
			}
		})
	}
}

func TestCatalogCheckHandlesNetworkFailure(t *testing.T) {
	client := CatalogClient{
		HTTP: &fakeHTTPDoer{err: errors.New("timeout")},
		Sources: map[string]string{
			"Dell Dock WD19": wd19SourceURL,
		},
	}
	result := client.Check(context.Background(), &domain.Dock{Model: "Dell Dock WD19"})
	if result.State != "vendor_metadata_unavailable" ||
		!strings.Contains(result.Reason, "timeout") {
		t.Fatalf("unexpected failure result: %+v", result)
	}
}

func TestCatalogCheckUsesPinnedCandidateOnDellForbidden(t *testing.T) {
	pinned := PinnedWD19LinuxCandidate()
	client := CatalogClient{
		HTTP: &fakeHTTPDoer{response: &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(strings.NewReader("blocked")),
		}},
		Sources: map[string]string{
			"Dell Dock WD19": wd19SourceURL,
		},
		Fallbacks: map[string]domain.FirmwareCandidate{
			"Dell Dock WD19": pinned,
		},
	}
	result := client.Check(context.Background(), wd19DockWithFirmware(wd19CurrentFirmware()))
	if result.State != "update_available" || result.Candidate == nil || result.Candidate.DownloadURL != pinned.DownloadURL {
		t.Fatalf("unexpected pinned result: %+v", result)
	}
	if !strings.Contains(result.Reason, "pinned") {
		t.Fatalf("fallback reason is not explicit: %s", result.Reason)
	}
}

func TestCatalogCheckDecidesFromComponentVersions(t *testing.T) {
	for _, test := range []struct {
		name      string
		firmware  []domain.FirmwareObservation
		edit      func(*domain.FirmwareCandidate)
		wantState string
	}{
		{
			name:      "newer component",
			firmware:  wd19CurrentFirmware(),
			wantState: "update_available",
		},
		{
			name:     "equal components",
			firmware: wd19CurrentFirmware(),
			edit: func(candidate *domain.FirmwareCandidate) {
				candidate.ComponentVersions["package"] = "01.00.47.01"
				candidate.ComponentVersions["embedded_controller"] = "01.01.00.13"
			},
			wantState: "up_to_date",
		},
		{
			name:     "older component",
			firmware: wd19CurrentFirmware(),
			edit: func(candidate *domain.FirmwareCandidate) {
				candidate.ComponentVersions["package"] = "01.00.00.01"
				candidate.ComponentVersions["embedded_controller"] = "01.01.00.13"
			},
			wantState: "up_to_date",
		},
		{
			name: "missing component",
			firmware: []domain.FirmwareObservation{
				{Component: "package", Version: "01.00.47.01"},
				{Component: "embedded_controller", Version: "01.01.00.13"},
				{Component: "usb_hub_gen1", Version: "01.23"},
				{Component: "usb_hub_gen2", Version: "01.62"},
			},
			wantState: "version_check_unavailable",
		},
		{
			name: "conflicting component",
			firmware: append(wd19CurrentFirmware(), domain.FirmwareObservation{
				Component: "package", Version: "01.00.47.02",
			}),
			wantState: "version_check_unavailable",
		},
		{
			name:     "unsupported candidate component",
			firmware: wd19CurrentFirmware(),
			edit: func(candidate *domain.FirmwareCandidate) {
				candidate.ComponentVersions["thunderbolt"] = "01.00"
			},
			wantState: "version_check_unavailable",
		},
		{
			name:     "invalid later component after newer package",
			firmware: wd19CurrentFirmware(),
			edit: func(candidate *domain.FirmwareCandidate) {
				candidate.ComponentVersions["mst"] = "invalid"
			},
			wantState: "version_check_unavailable",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := PinnedWD19LinuxCandidate()
			candidate.ComponentVersions = cloneTestComponentVersions(candidate.ComponentVersions)
			if test.edit != nil {
				test.edit(&candidate)
			}
			client := CatalogClient{
				HTTP: &fakeHTTPDoer{response: &http.Response{
					StatusCode: http.StatusForbidden,
					Body:       io.NopCloser(strings.NewReader("blocked")),
				}},
				Sources:   map[string]string{"Dell Dock WD19": wd19SourceURL},
				Fallbacks: map[string]domain.FirmwareCandidate{"Dell Dock WD19": candidate},
			}
			result := client.Check(context.Background(), wd19DockWithFirmware(test.firmware))
			if result.State != test.wantState {
				t.Fatalf("state = %q, want %q: %+v", result.State, test.wantState, result)
			}
		})
	}
}

func wd19DockWithFirmware(firmware []domain.FirmwareObservation) *domain.Dock {
	return &domain.Dock{
		Model:    "Dell Dock WD19",
		Firmware: firmware,
	}
}

func wd19CurrentFirmware() []domain.FirmwareObservation {
	return []domain.FirmwareObservation{
		{Component: "package", Version: "01.00.47.01"},
		{Component: "embedded_controller", Version: "01.01.00.13"},
		{Component: "usb_hub_gen1", Version: "01.23"},
		{Component: "usb_hub_gen2", Version: "01.62"},
		{Component: "mst", Version: "05.07.08"},
	}
}

func cloneTestComponentVersions(versions map[string]string) map[string]string {
	clone := make(map[string]string, len(versions))
	for component, version := range versions {
		clone[component] = version
	}
	return clone
}
