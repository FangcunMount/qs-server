package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSelectorProgressReportsActiveAndCompletedShards(t *testing.T) {
	var output bytes.Buffer
	progress := &selectorProgress{
		out:       &output,
		startedAt: time.Now(),
		total:     2,
		active:    make(map[string]time.Time),
	}
	progress.Start("2026-08-05")
	progress.Snapshot()
	progress.Finish("2026-08-05", 12, 2*time.Second)

	text := output.String()
	for _, want := range []string{
		"0/2", "started=2026-08-05", "active=2026-08-05", "1/2", "50.0%", "rows=12", "finished=2026-08-05",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("progress output missing %q:\n%s", want, text)
		}
	}
}

func TestSelectorProgressCanBeDisabled(t *testing.T) {
	var output bytes.Buffer
	progress := &selectorProgress{
		out:       &output,
		disabled:  true,
		startedAt: time.Now(),
		total:     1,
		active:    make(map[string]time.Time),
	}
	progress.Start("2026-08-05")
	progress.Snapshot()
	progress.Finish("2026-08-05", 1, time.Second)
	if output.Len() != 0 {
		t.Fatalf("disabled progress output = %q", output.String())
	}
}
