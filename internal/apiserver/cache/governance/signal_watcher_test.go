package cachegovernance

import "testing"

func TestShouldWarmPublishedSignal(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   bool
	}{
		{name: "published", action: "published", want: true},
		{name: "normalized published", action: " Published ", want: true},
		{name: "unpublished", action: "unpublished", want: false},
		{name: "archived", action: "archived", want: false},
		{name: "missing action", action: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldWarmPublishedSignal(tt.action); got != tt.want {
				t.Fatalf("shouldWarmPublishedSignal(%q) = %t, want %t", tt.action, got, tt.want)
			}
		})
	}
}
