package testee

import "testing"

func TestIsSeeddataMockSource(t *testing.T) {
	tests := []struct {
		source Source
		want   bool
	}{
		{SourceSeeddata, true},
		{SourceDailySimulation, true},
		{SourceManual, false},
		{SourceImport, false},
		{SourceAssessmentEntry, false},
		{SourceUnknown, false},
	}
	for _, tt := range tests {
		if got := IsSeeddataMockSource(tt.source); got != tt.want {
			t.Fatalf("IsSeeddataMockSource(%q) = %v, want %v", tt.source, got, tt.want)
		}
	}
}
