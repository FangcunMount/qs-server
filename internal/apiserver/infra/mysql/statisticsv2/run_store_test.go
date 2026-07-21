package statisticsv2

import (
	"strings"
	"testing"
)

func TestTruncateRunTextPreservesUnicodeCharacterBoundary(t *testing.T) {
	value := strings.Repeat("审", 1001)
	got := truncateRunText(value, 1000)
	if len([]rune(got)) != 1000 || !strings.HasSuffix(got, "审") {
		t.Fatalf("runes=%d suffix=%q", len([]rune(got)), got[len(got)-3:])
	}
}
