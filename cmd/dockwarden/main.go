package main

import (
	"fmt"
	"os"

	"github.com/fexxdev/dockwarden/internal/cli"
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

	fmt.Fprintf(os.Stderr, "%s: command wiring is not ready\n", options.Command)
	os.Exit(2)
}
