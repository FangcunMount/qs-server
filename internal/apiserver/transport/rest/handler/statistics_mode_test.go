package handler

import (
	"strings"
	"testing"

	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

func TestNormalizeStatisticsRunModeKeepsWireCompatibilityAtTransportBoundary(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		validateOnly bool
		want         statisticsDomain.RunMode
		wantError    string
	}{
		{name: "canonical validate", raw: "validate", want: statisticsDomain.RunModeValidate},
		{name: "legacy validate only", validateOnly: true, want: statisticsDomain.RunModeValidate},
		{name: "legacy default publish", want: statisticsDomain.RunModePublish},
		{name: "matching legacy and canonical", raw: "validate", validateOnly: true, want: statisticsDomain.RunModeValidate},
		{name: "conflicting inputs", raw: "repair", validateOnly: true, wantError: "conflicts"},
		{name: "invalid canonical mode", raw: "unknown", wantError: "invalid statistics run mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeStatisticsRunMode(tt.raw, tt.validateOnly)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("mode=%q err=%v", got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("mode=%q err=%v want=%q", got, err, tt.want)
			}
		})
	}
}
