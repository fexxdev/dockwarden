package cli

import (
	"fmt"
	"strings"
)

type Options struct {
	JSON    bool
	Verbose bool
	Apply   bool
	LogFile string
	Version bool
	Help    bool
	Command string
}

var commands = map[string]struct{}{
	"scan":          {},
	"status":        {},
	"doctor":        {},
	"check-updates": {},
	"update":        {},
}

func Parse(args []string) (Options, error) {
	var options Options

	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--json":
			options.JSON = true
		case "--verbose":
			options.Verbose = true
		case "--apply":
			options.Apply = true
		case "--log-file":
			if index+1 >= len(args) || strings.TrimSpace(args[index+1]) == "" || strings.HasPrefix(args[index+1], "-") {
				return Options{}, fmt.Errorf("--log-file requires a path")
			}
			index++
			options.LogFile = args[index]
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
	if options.Apply && options.Command != "update" {
		return Options{}, fmt.Errorf("--apply is only valid with update")
	}
	return options, nil
}

func Usage() string {
	return "usage: dockwarden [--json] [--verbose] [--log-file PATH] [--apply] <scan|status|doctor|check-updates|update>"
}
