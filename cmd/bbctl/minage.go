package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const minAgeFormatHint = `use a number of days like "7d" or a Go duration like "168h"`

// ParseMinAge parses the value of the --min-age flag.
//
// It accepts a whole number of days with a "d" suffix (e.g. "7d") or any
// non-negative Go duration (e.g. "168h", "90m"). An empty string or "0"
// disables the minimum age.
func ParseMinAge(s string) (time.Duration, error) {
	if s == "" || s == "0" {
		return 0, nil
	}
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.ParseUint(days, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid minimum age %q: %s", s, minAgeFormatHint)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid minimum age %q: %s", s, minAgeFormatHint)
	}
	return d, nil
}
