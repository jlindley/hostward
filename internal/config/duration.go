package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func ParseDuration(input string) (time.Duration, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, fmt.Errorf("duration cannot be empty")
	}

	var total time.Duration
	var consumed int

	for consumed < len(input) {
		start := consumed
		for consumed < len(input) && (unicode.IsDigit(rune(input[consumed])) || input[consumed] == '.') {
			consumed++
		}
		if start == consumed {
			return 0, fmt.Errorf("invalid duration %q", input)
		}

		valueText := input[start:consumed]
		unitStart := consumed
		for consumed < len(input) && unicode.IsLetter(rune(input[consumed])) {
			consumed++
		}
		if unitStart == consumed {
			return 0, fmt.Errorf("missing duration unit in %q", input)
		}

		unitText := input[unitStart:consumed]
		part, err := parseDurationPart(valueText, unitText)
		if err != nil {
			return 0, err
		}
		total += part
	}

	if total <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}

	return total, nil
}

func parseDurationPart(valueText, unitText string) (time.Duration, error) {
	value, err := strconv.ParseFloat(valueText, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration value %q", valueText)
	}
	if value < 0 {
		return 0, fmt.Errorf("duration values must be non-negative")
	}

	switch unitText {
	case "ns":
		return time.Duration(value), nil
	case "us", "µs":
		return time.Duration(value * float64(time.Microsecond)), nil
	case "ms":
		return time.Duration(value * float64(time.Millisecond)), nil
	case "s":
		return time.Duration(value * float64(time.Second)), nil
	case "m":
		return time.Duration(value * float64(time.Minute)), nil
	case "h":
		return time.Duration(value * float64(time.Hour)), nil
	case "d":
		return time.Duration(value * float64(24*time.Hour)), nil
	case "w":
		return time.Duration(value * float64(7*24*time.Hour)), nil
	default:
		return 0, fmt.Errorf("unsupported duration unit %q", unitText)
	}
}
