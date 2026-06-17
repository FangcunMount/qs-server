package main

import (
	"testing"
	"time"
)

func TestFormatProgressDuration(t *testing.T) {
	if got := formatProgressDuration(45 * time.Second); got != "45s" {
		t.Fatalf("formatProgressDuration() = %q, want 45s", got)
	}
	if got := formatProgressDuration(125 * time.Second); got != "2m05s" {
		t.Fatalf("formatProgressDuration() = %q, want 2m05s", got)
	}
}

func TestTruncateProgressText(t *testing.T) {
	if got := truncateProgressText("short", 10); got != "short" {
		t.Fatalf("truncateProgressText() = %q", got)
	}
	if got := truncateProgressText("0123456789abcdef", 10); got != "0123456..." {
		t.Fatalf("truncateProgressText() = %q", got)
	}
}
