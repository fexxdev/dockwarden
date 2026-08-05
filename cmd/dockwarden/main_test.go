package main

import (
	"net/http"
	"testing"

	"github.com/fexxdev/dockwarden/internal/domain"
	"github.com/fexxdev/dockwarden/internal/update"
)

func TestDarwinFwupdToolUpdaterConfiguresNativePreflight(t *testing.T) {
	updater := newDarwinFwupdToolUpdater(&http.Client{}, func(domain.HIDTarget) (update.HIDConnection, error) {
		return nil, nil
	})
	preflight, ok := updater.Preflight.(update.MacPreflightReader)
	if !ok || preflight.Open == nil {
		t.Fatalf("Darwin fwupdtool updater did not configure native preflight: %+v", updater)
	}
}

func TestDefaultVersionIsLinkerInjectable(t *testing.T) {
	versionPointer := &version
	if *versionPointer != "0.3.0-dev" {
		t.Fatalf("unexpected default version: %q", version)
	}
}
