package cli

import "testing"

func TestParseAcceptsJSONBeforeCommand(t *testing.T) {
	got, err := Parse([]string{"--json", "--verbose", "scan"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.JSON || !got.Verbose || got.Command != "scan" {
		t.Fatalf("unexpected options: %+v", got)
	}
}

func TestParseRejectsUnknownCommand(t *testing.T) {
	if _, err := Parse([]string{"explode"}); err == nil {
		t.Fatal("expected unknown command error")
	}
}

func TestParseRecognizesVersion(t *testing.T) {
	got, err := Parse([]string{"--version"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Version {
		t.Fatalf("expected version option: %+v", got)
	}
}
