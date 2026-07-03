package systemgovernance

import (
	"fmt"
	"strings"
	"time"
)

const DefaultWindow = "5m"

// ParseWindow normalizes a governance metrics window such as 5m or 1h.
func ParseWindow(raw string) (time.Duration, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = DefaultWindow
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, "", fmt.Errorf("invalid window %q: %w", raw, err)
	}
	if d < time.Minute {
		return 0, "", fmt.Errorf("window must be at least 1m")
	}
	if d > 24*time.Hour {
		return 0, "", fmt.Errorf("window must be at most 24h")
	}
	return d, raw, nil
}
