package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTextLoggerWritesOneEscapedLine(t *testing.T) {
	var output strings.Builder
	logger := NewWriter(&output)

	if err := logger.Log("info", "update.start", map[string]string{
		"z": "last",
		"a": "line one\nline two",
	}); err != nil {
		t.Fatal(err)
	}

	line := output.String()
	if strings.Count(line, "\n") != 1 {
		t.Fatalf("expected one log line, got %q", line)
	}
	if !strings.Contains(line, " level=INFO event=update.start") {
		t.Fatalf("missing normalized level and event: %q", line)
	}
	if !strings.Contains(line, `a="line one\nline two" z="last"`) {
		t.Fatalf("fields are not sorted and escaped: %q", line)
	}
}

func TestNewFileLoggerAppendsWithPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dockwarden.log")
	logger, err := NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Log("info", "first", nil); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Log("info", "second", nil); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "event=first") || !strings.Contains(text, "event=second") {
		t.Fatalf("logger did not append records: %q", text)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("log permissions = %o, want 600", info.Mode().Perm())
	}
}
