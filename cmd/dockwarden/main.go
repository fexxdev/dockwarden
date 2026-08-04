package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/fexxdev/dockwarden/internal/app"
	"github.com/fexxdev/dockwarden/internal/cli"
	"github.com/fexxdev/dockwarden/internal/discovery"
)

const version = "0.1.0-dev"

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
	switch runtime.GOOS {
	case "darwin":
		dependencies.Inspector = discovery.MacInspector{}
	case "linux":
		dependencies.Inspector = discovery.LinuxInspector{}
	default:
		fmt.Fprintf(os.Stderr, "dockwarden: unsupported platform %s\n", runtime.GOOS)
		os.Exit(2)
	}

	os.Exit(app.Run(context.Background(), options, dependencies))
}
