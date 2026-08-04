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

func TestParseAcceptsUpdateApply(t *testing.T) {
	got, err := Parse([]string{"update", "--apply"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != "update" || !got.Apply {
		t.Fatalf("unexpected options: %+v", got)
	}
}

func TestParseRejectsApplyForReadOnlyCommand(t *testing.T) {
	if _, err := Parse([]string{"scan", "--apply"}); err == nil {
		t.Fatal("expected --apply to be rejected for scan")
	}
}
