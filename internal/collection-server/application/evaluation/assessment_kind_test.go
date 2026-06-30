package evaluation

import (
	"errors"
	"testing"
)

func TestNormalizeAssessmentKind(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "empty", raw: "", want: ""},
		{name: "medical", raw: "medical", want: "scale"},
		{name: "medical uppercase", raw: "Medical", want: "scale"},
		{name: "personality", raw: "personality", want: "personality"},
		{name: "invalid", raw: "foo", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeAssessmentKind(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if !errors.Is(err, ErrInvalidAssessmentKind) {
					t.Fatalf("expected ErrInvalidAssessmentKind, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
