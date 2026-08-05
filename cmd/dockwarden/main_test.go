package main

import (
	"net/http"
	"testing"

	"github.com/fexxdev/dockwarden/internal/update"
)

func TestDarwinFwupdToolUpdaterUsesFwupdPreflight(t *testing.T) {
	updater := newDarwinFwupdToolUpdater(&http.Client{}, update.FwupdToolClient{})
	preflight, ok := updater.Preflight.(update.FwupdToolPreflight)
	if !ok {
		t.Fatalf("Darwin fwupdtool updater did not configure fwupd preflight: %+v", updater)
	}
	if preflight.Client.ToolPath != "" {
		t.Fatalf("unexpected preflight tool override: %q", preflight.Client.ToolPath)
	}
}

func TestDefaultVersionIsLinkerInjectable(t *testing.T) {
	versionPointer := &version
	if *versionPointer != "0.3.0-dev" {
		t.Fatalf("unexpected default version: %q", version)
	}
}
