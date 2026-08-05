package update

import (
	"context"
	"strings"

	"github.com/fexxdev/dockwarden/internal/logging"
)

func logUpdateEvent(logger logging.Logger, level, event string, fields map[string]string) {
	if logger != nil {
		_ = logger.Log(level, event, fields)
	}
}

func runLoggedFwupdCommand(ctx context.Context, runner CommandRunner, env []string, toolPath string, logger logging.Logger, args ...string) ([]byte, error) {
	logUpdateEvent(logger, "INFO", "fwupdtool.command.start", map[string]string{
		"command": toolPath,
		"args":    strings.Join(args, " "),
	})
	output, err := runCommandWithEnv(ctx, runner, env, toolPath, args...)
	logUpdateEvent(logger, commandLogLevel(err), "fwupdtool.command.complete", map[string]string{
		"command": toolPath,
		"args":    strings.Join(args, " "),
		"output":  summarize(string(output)),
		"error":   errorText(err),
	})
	return output, err
}
