// Package firmwareversion compares dotted numeric firmware versions.
package firmwareversion

import (
	"fmt"
	"strconv"
	"strings"
)

// Compare returns 1 when candidate is newer than current, -1 when it is older,
// and 0 when the versions are equal.
func Compare(candidate, current string) (int, error) {
	candidateParts, err := parse(candidate)
	if err != nil {
		return 0, fmt.Errorf("invalid candidate version %q: %w", candidate, err)
	}
	currentParts, err := parse(current)
	if err != nil {
		return 0, fmt.Errorf("invalid current version %q: %w", current, err)
	}
	candidateParts = trimLeadingZeros(candidateParts)
	currentParts = trimLeadingZeros(currentParts)
	for index := 0; index < len(candidateParts) || index < len(currentParts); index++ {
		var candidatePart, currentPart int
		if index < len(candidateParts) {
			candidatePart = candidateParts[index]
		}
		if index < len(currentParts) {
			currentPart = currentParts[index]
		}
		if candidatePart > currentPart {
			return 1, nil
		}
		if candidatePart < currentPart {
			return -1, nil
		}
	}
	return 0, nil
}

func trimLeadingZeros(parts []int) []int {
	for len(parts) > 1 && parts[0] == 0 {
		parts = parts[1:]
	}
	return parts
}

func parse(value string) ([]int, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) == 0 || len(parts) > 8 {
		return nil, fmt.Errorf("version has invalid component count")
	}
	result := make([]int, len(parts))
	for index, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("version has an empty component")
		}
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed < 0 || parsed > 255 {
			return nil, fmt.Errorf("version has invalid component %q", part)
		}
		result[index] = parsed
	}
	return result, nil
}
