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
	"github.com/fexxdev/dockwarden/internal/update"
)

const version = "0.2.0-dev"
const wd19MacDriverURL = "https://www.dell.com/support/home/en-us/drivers/driversdetails?driverid=nkjg6"
const wd19LinuxDriverURL = "https://www.dell.com/support/home/en-us/drivers/driversdetails?driverid=4p6vj"

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

	dependencies := app.Dependencies{
		Out: os.Stdout,
		Err: os.Stderr,
	}
	httpClient := &http.Client{Timeout: 15 * time.Second}
	driverURL := wd19MacDriverURL
	switch runtime.GOOS {
	case "darwin":
		dependencies.Inspector = discovery.MacInspector{}
	case "linux":
		dependencies.Inspector = discovery.LinuxInspector{}
		dependencies.Updater = update.FwupdUpdater{HTTP: httpClient}
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
	}

	os.Exit(app.Run(context.Background(), options, dependencies))
}
