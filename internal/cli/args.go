package cli

import (
	"fmt"
	"strings"
)

type Options struct {
	JSON    bool
	Verbose bool
	Version bool
	Help    bool
	Command string
}

var commands = map[string]struct{}{
	"scan":          {},
	"status":        {},
	"doctor":        {},
	"check-updates": {},
}

func Parse(args []string) (Options, error) {
	var options Options

	for _, arg := range args {
		switch arg {
		case "--json":
			options.JSON = true
		case "--verbose":
			options.Verbose = true
		case "--version", "-V":
			options.Version = true
		case "--help", "-h":
			options.Help = true
		default:
			if strings.HasPrefix(arg, "-") {
				return Options{}, fmt.Errorf("unknown option %q", arg)
			}
			if options.Command != "" {
				return Options{}, fmt.Errorf("unexpected argument %q", arg)
			}
			if _, ok := commands[arg]; !ok {
				return Options{}, fmt.Errorf("unknown command %q", arg)
			}
			options.Command = arg
		}
	}

	if options.Version || options.Help {
		return options, nil
	}
	if options.Command == "" {
		return Options{}, fmt.Errorf("a command is required")
	}
	return options, nil
}

func Usage() string {
	return "usage: dockwarden [--json] [--verbose] <scan|status|doctor|check-updates>"
}
