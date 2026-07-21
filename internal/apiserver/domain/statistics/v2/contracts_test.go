package v2

import (
	"testing"
	"time"
)

func TestDefaultWindowUsesShanghaiCompleteDays(t *testing.T) {
	now := time.Date(2026, 7, 21, 16, 30, 0, 0, time.UTC) // Shanghai next day 00:30
	window, asOf := DefaultWindow(now, 7)
	if got := window.To.In(Shanghai).Format(time.RFC3339); got != "2026-07-22T00:00:00+08:00" {
		t.Fatalf("to=%s", got)
	}
	if got := asOf.Format("2006-01-02"); got != "2026-07-21" {
		t.Fatalf("as_of=%s", got)
	}
}
