package plan

import (
	"testing"
	"time"
)

func TestNormalizePlanTriggerTime(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default", input: "", want: DefaultPlanTriggerTime},
		{name: "hhmm", input: "08:30", want: "08:30:00"},
		{name: "hhmmss", input: "08:30:15", want: "08:30:15"},
		{name: "invalid", input: "25:00", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePlanTriggerTime(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizePlanTriggerTime returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestApplyPlanTriggerTime(t *testing.T) {
	base := time.Date(2026, 4, 3, 0, 0, 0, 0, time.Local)

	got, err := ApplyPlanTriggerTime(base, "08:30")
	if err != nil {
		t.Fatalf("ApplyPlanTriggerTime returned error: %v", err)
	}

	want := time.Date(2026, 4, 3, 8, 30, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("expected %s, got %s", want, got)
	}
}
