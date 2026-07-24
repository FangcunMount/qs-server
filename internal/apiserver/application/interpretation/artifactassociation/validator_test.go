package artifactassociation

import (
	"reflect"
	"testing"
)

func TestValidator(t *testing.T) {
	t.Parallel()

	strict := Association{
		AssessmentID: 1, OrgID: 2, HasOrgID: true, TesteeID: 3,
		OutcomeID: 4, HasOutcomeID: true, GenerationID: 5, HasGenerationID: true,
	}
	tests := []struct {
		name   string
		source Association
		want   []Field
	}{
		{name: "valid", source: strict},
		{
			name: "missing archive org is inconsistent",
			source: Association{
				AssessmentID: 1, TesteeID: 3, OutcomeID: 4, HasOutcomeID: true,
				GenerationID: 5, HasGenerationID: true,
			},
			want: []Field{FieldOrgID},
		},
		{
			name: "all correlation fields",
			source: Association{
				AssessmentID: 10, OrgID: 20, HasOrgID: true, TesteeID: 30,
				OutcomeID: 40, HasOutcomeID: true, GenerationID: 50, HasGenerationID: true,
			},
			want: []Field{FieldAssessmentID, FieldOrgID, FieldTesteeID, FieldOutcomeID, FieldGenerationID},
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NewValidator().Validate(strict, tc.source)
			if !reflect.DeepEqual(got.Mismatch, tc.want) {
				t.Fatalf("mismatch = %v, want %v", got.Mismatch, tc.want)
			}
			wantStatus := StatusValid
			if len(tc.want) > 0 {
				wantStatus = StatusMismatch
			}
			if got.Status != wantStatus {
				t.Fatalf("status = %q, want %q", got.Status, wantStatus)
			}
		})
	}
}

func TestValidatorAllowsLegacyCatalogWithoutCorrelationUntilBackfilled(t *testing.T) {
	t.Parallel()

	catalog := Association{AssessmentID: 1, OrgID: 2, HasOrgID: true, TesteeID: 3}
	source := Association{
		AssessmentID: 1, OrgID: 2, HasOrgID: true, TesteeID: 3,
		OutcomeID: 4, HasOutcomeID: true, GenerationID: 5, HasGenerationID: true,
	}
	if got := NewValidator().Validate(catalog, source); got.Status != StatusValid {
		t.Fatalf("legacy catalog result = %#v, want valid during backfill", got)
	}
}
