package utils

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseCustomDuration handles strings like "30m", "1.5h", "2d".
func ParseCustomDuration(s string) (time.Duration, error) {
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(m|h|d)$`)
	matches := re.FindStringSubmatch(strings.TrimSpace(s))
	if len(matches) != 3 {
		return 0, fmt.Errorf("unsupported format, use e.g. 30m, 1.5h, 2d")
	}
	num, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, err
	}
	unit := matches[2]
	switch unit {
	case "m":
		return time.Duration(num * float64(time.Minute)), nil
	case "h":
		return time.Duration(num * float64(time.Hour)), nil
	case "d":
		return time.Duration(num * 24 * float64(time.Hour)), nil
	default:
		return 0, fmt.Errorf("unknown unit %q", unit)
	}
}
