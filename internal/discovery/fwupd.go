package discovery

import (
	"bufio"
	"regexp"
	"strings"

	"github.com/fexxdev/dockwarden/internal/domain"
)

var fwupdCurrentVersionPattern = regexp.MustCompile("(?i)^Current version:\\s*(\\S+)")

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

		content := trimFwupdTreePrefix(line)
		if matches := fwupdCurrentVersionPattern.FindStringSubmatch(content); len(matches) == 2 {
			if component == "" {
				component = "unknown component"
			}
			observations = append(observations, domain.FirmwareObservation{
				Component:  normalizeFwupdComponentName(component),
				Version:    matches[1],
				Source:     "fwupdmgr",
				Confidence: "reported",
			})
			continue
		}

		hasTreeBranch := strings.Contains(line, "├─") || strings.Contains(line, "└─")
		if !hasTreeBranch && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "│")) {
			continue
		}
		component = content
		component = strings.TrimSuffix(component, ":")
		if strings.EqualFold(component, "Devices") {
			component = ""
		}
	}
	return observations
}

func trimFwupdTreePrefix(line string) string {
	return strings.TrimLeftFunc(line, func(value rune) bool {
		switch value {
		case ' ', '\t', '│', '├', '└', '─':
			return true
		default:
			return false
		}
	})
}

func normalizeFwupdComponentName(component string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(component), " "))
	compact := strings.NewReplacer("-", "", "_", "", " ", "").Replace(normalized)
	switch {
	case normalized == "wd19":
		return domain.FirmwareComponentEmbeddedController
	case normalized == "package level of dell dock":
		return domain.FirmwareComponentPackage
	case normalized == "dell dock":
		return domain.FirmwareComponentEmbeddedController
	case strings.Contains(compact, "rts5413") && strings.Contains(normalized, "dell dock"):
		return domain.FirmwareComponentUSBHubGen1
	case strings.Contains(compact, "rts5487") && strings.Contains(normalized, "dell dock"):
		return domain.FirmwareComponentUSBHubGen2
	case strings.Contains(compact, "vmm5331") && strings.Contains(normalized, "dell dock"):
		return domain.FirmwareComponentMST
	case !strings.Contains(normalized, "wd19"):
		return component
	case strings.Contains(normalized, "mst"):
		return domain.FirmwareComponentMST
	case strings.Contains(normalized, "embedded controller") || strings.HasSuffix(normalized, " ec"):
		return domain.FirmwareComponentEmbeddedController
	case strings.Contains(normalized, "usb") && strings.Contains(compact, "gen1"):
		return domain.FirmwareComponentUSBHubGen1
	case strings.Contains(normalized, "usb") && strings.Contains(compact, "gen2"):
		return domain.FirmwareComponentUSBHubGen2
	case strings.Contains(normalized, "dock") || normalized == "dell wd19":
		return domain.FirmwareComponentPackage
	default:
		return component
	}
}
