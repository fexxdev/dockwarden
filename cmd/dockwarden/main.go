package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/fexxdev/dockwarden/internal/app"
	"github.com/fexxdev/dockwarden/internal/cli"
	"github.com/fexxdev/dockwarden/internal/dell"
	"github.com/fexxdev/dockwarden/internal/discovery"
	"github.com/fexxdev/dockwarden/internal/domain"
	"github.com/fexxdev/dockwarden/internal/logging"
	"github.com/fexxdev/dockwarden/internal/update"
)

var version = "0.3.0-dev"

const wd19LinuxDriverURL = "https://www.dell.com/support/home/en-us/drivers/driversdetails?driverid=389w0"
const defaultLogFile = "dockwarden-log.txt"

func main() {
	options, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, cli.Usage())
		os.Exit(2)
	}
	if options.Version {
		fmt.Println(version)
		return
	}
	if options.Help {
		fmt.Println(cli.Usage())
		return
	}
	logPath := options.LogFile
	if logPath == "" {
		logPath = defaultLogFile
	}
	logger, err := logging.NewFile(logPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockwarden:", err)
		os.Exit(2)
	}
	defer logger.Close()
	_ = logger.Log("INFO", "logger.ready", map[string]string{"path": logPath})

	dependencies := app.Dependencies{
		Out:    os.Stdout,
		Err:    os.Stderr,
		Logger: logger,
	}
	httpClient := &http.Client{Timeout: 15 * time.Second}
	driverURL := wd19LinuxDriverURL
	switch runtime.GOOS {
	case "darwin":
		fwupdClient := update.FwupdToolClient{
			ToolPath: os.Getenv(update.FwupdToolEnvironmentVariable),
			Logger:   logger,
		}
		dependencies.Inspector = discovery.MacInspector{
			Firmware: update.FwupdToolFirmwareReader{Client: fwupdClient},
		}
		dependencies.PermissionCheckForDock = func(ctx context.Context, dock *domain.Dock) error {
			return (update.FwupdToolPermissionChecker{Client: fwupdClient}).CheckForDock(ctx, dock)
		}
		dependencies.Updater = newDarwinFwupdToolUpdater(httpClient, fwupdClient)
		driverURL = wd19LinuxDriverURL
	case "linux":
		dependencies.Inspector = discovery.LinuxInspector{}
		dependencies.Updater = update.FwupdUpdater{HTTP: httpClient, Logger: logger}
		driverURL = wd19LinuxDriverURL
	default:
		fmt.Fprintf(os.Stderr, "dockwarden: unsupported platform %s\n", runtime.GOOS)
		os.Exit(2)
	}
	dependencies.Updates = dell.CatalogClient{
		HTTP: httpClient,
		Sources: map[string]string{
			"Dell Dock WD19": driverURL,
		},
		Fallbacks: map[string]domain.FirmwareCandidate{
			"Dell Dock WD19": dell.PinnedWD19LinuxCandidate(),
		},
	}

	os.Exit(app.Run(context.Background(), options, dependencies))
}

func newDarwinFwupdToolUpdater(httpClient *http.Client, client update.FwupdToolClient) update.FwupdToolUpdater {
	return update.FwupdToolUpdater{
		HTTP:      httpClient,
		Runner:    client.Runner,
		ToolPath:  client.ToolPath,
		ConfigDir: client.ConfigDir,
		TempDir:   client.TempDir,
		Logger:    client.Logger,
		Preflight: update.FwupdToolPreflight{Client: client},
	}
}
