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
	if candidate.PackageName != "DellDock_WD19_WD22_FW_01.00.36.exe" ||
		candidate.Version != "01.00.36" ||
		candidate.ReleaseDate != "15 Apr 2026" {
		t.Fatalf("unexpected candidate identity: %+v", candidate)
	}
	if candidate.SHA256 != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected checksum: %+v", candidate)
	}
	if candidate.Format != "Application" || len(candidate.SupportedOS) != 3 ||
		len(candidate.CompatibleModels) != 2 {
		t.Fatalf("unexpected candidate metadata: %+v", candidate)
	}
}

func TestParseDriverPageRequiresHash(t *testing.T) {
	page := []byte("<html><div>Version: 01.00.36</div><div>Release Date: 15 Apr 2026</div><div>File Name: DellDock_WD19_FW.exe</div><div>Compatible Systems: Dell Dock WD19</div></html>")
	if _, err := ParseDriverPage(wd19SourceURL, page); err == nil {
		t.Fatal("expected missing hash error")
	}
}

func TestParseDriverPageHandlesComponentVersionsAndLists(t *testing.T) {
	page := []byte("<div>Version : 01.01.03.01, 01.01.14.01</div><div>Release Date : 30 Apr 2026</div><div>File Name : DellDockFirmwarePackage_WD19_WD22_01.01.14.exe</div><div>SHA-256 : 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef</div><h2>Compatible Systems</h2><a>Dell 14 D14260</a><a>Dell Dock WD19</a><h2>Supported Operating Systems</h2><p>Windows 11</p>")
	candidate, err := ParseDriverPage(wd19SourceURL, page)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Version != "01.01.03.01, 01.01.14.01" {
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

func TestCatalogCheckReturnsCandidate(t *testing.T) {
	page := loadDriverPage(t)
	httpClient := &fakeHTTPDoer{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(page)),
		},
	}
	client := CatalogClient{
		HTTP: httpClient,
		Sources: map[string]string{
			"Dell Dock WD19": wd19SourceURL,
		},
	}
	result := client.Check(context.Background(), &domain.Dock{Model: "Dell Dock WD19"})
	if result.State != "update_available" || result.SourceURL != wd19SourceURL ||
		result.Candidate == nil || result.Candidate.Version != "01.00.36" {
		t.Fatalf("unexpected update result: %+v", result)
	}
	if httpClient.request == nil || httpClient.request.Header.Get("User-Agent") == "" {
		t.Fatal("expected a descriptive user agent")
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
