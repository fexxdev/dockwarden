package discovery

import (
	"bufio"
	"regexp"
	"strings"

	"github.com/fexxdev/dockwarden/internal/domain"
)

var fwupdCurrentVersionPattern = regexp.MustCompile("(?i)^\\s*Current version:\\s*(\\S+)")

func ParseFwupdDevices(input string) []domain.FirmwareObservation {
	var observations []domain.FirmwareObservation
	component := ""

	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if matches := fwupdCurrentVersionPattern.FindStringSubmatch(line); len(matches) == 2 {
			if component == "" {
				component = "unknown component"
			}
			observations = append(observations, domain.FirmwareObservation{
				Component:  component,
				Version:    matches[1],
				Source:     "fwupdmgr",
				Confidence: "reported",
			})
			continue
		}

		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		component = strings.TrimSpace(strings.TrimLeft(trimmed, "└─"))
		component = strings.TrimSuffix(component, ":")
		if strings.EqualFold(component, "Devices") {
			component = ""
		}
	}
	return observations
}
