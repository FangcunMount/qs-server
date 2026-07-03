package systemgovernance

import (
	"testing"
	"time"
)

func TestParseWindowDefaultsAndValidates(t *testing.T) {
	duration, label, err := ParseWindow("")
	if err != nil {
		t.Fatalf("ParseWindow() error = %v", err)
	}
	if label != DefaultWindow || duration != 5*time.Minute {
		t.Fatalf("ParseWindow() = (%v, %q), want 5m", duration, label)
	}
	if _, _, err := ParseWindow("30s"); err == nil {
		t.Fatal("ParseWindow(30s) error = nil, want validation error")
	}
}
