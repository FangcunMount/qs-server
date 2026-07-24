package interpretation

import (
	"testing"
)

func TestMismatchedAssociationFieldsArtifactRequiresOrg(t *testing.T) {
	catalog := ReportCatalogPO{AssessmentID: 1, OrgID: 10, TesteeID: 100}
	source := catalogSourceEnvelope{AssessmentID: 1, OrgID: 10, HasOrgID: true, TesteeID: 100}
	if fields := mismatchedAssociationFields(catalog, source); len(fields) != 0 {
		t.Fatalf("matched association = %v, want none", fields)
	}

	cases := []struct {
		name   string
		source catalogSourceEnvelope
		want   []string
	}{
		{
			name:   "assessment",
			source: catalogSourceEnvelope{AssessmentID: 2, OrgID: 10, HasOrgID: true, TesteeID: 100},
			want:   []string{"assessment_id"},
		},
		{
			name:   "org",
			source: catalogSourceEnvelope{AssessmentID: 1, OrgID: 11, HasOrgID: true, TesteeID: 100},
			want:   []string{"org_id"},
		},
		{
			name:   "testee",
			source: catalogSourceEnvelope{AssessmentID: 1, OrgID: 10, HasOrgID: true, TesteeID: 101},
			want:   []string{"testee_id"},
		},
		{
			name:   "all",
			source: catalogSourceEnvelope{AssessmentID: 9, OrgID: 8, HasOrgID: true, TesteeID: 7},
			want:   []string{"assessment_id", "org_id", "testee_id"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mismatchedAssociationFields(catalog, tc.source)
			if len(got) != len(tc.want) {
				t.Fatalf("fields = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("fields = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestMismatchedAssociationFieldsArchiveOrgTransition(t *testing.T) {
	catalog := ReportCatalogPO{AssessmentID: 1, OrgID: 10, TesteeID: 100}

	// Historical archive without org_id is not safe to serve until repaired.
	unproven := catalogSourceEnvelope{AssessmentID: 1, HasOrgID: false, TesteeID: 100}
	if fields := mismatchedAssociationFields(catalog, unproven); len(fields) != 1 || fields[0] != "org_id" {
		t.Fatalf("unproven org mismatch = %v, want [org_id]", fields)
	}
	unprovenWrongTestee := catalogSourceEnvelope{AssessmentID: 1, HasOrgID: false, TesteeID: 999}
	if fields := mismatchedAssociationFields(catalog, unprovenWrongTestee); len(fields) != 2 || fields[0] != "org_id" || fields[1] != "testee_id" {
		t.Fatalf("unproven org must not relax testee check: %v", fields)
	}
	unprovenWrongAssessment := catalogSourceEnvelope{AssessmentID: 2, HasOrgID: false, TesteeID: 100}
	if fields := mismatchedAssociationFields(catalog, unprovenWrongAssessment); len(fields) != 2 || fields[0] != "assessment_id" || fields[1] != "org_id" {
		t.Fatalf("unproven org must not relax assessment check: %v", fields)
	}

	// Archive with org_id: org participates in fail-closed compare.
	provenMismatch := catalogSourceEnvelope{AssessmentID: 1, OrgID: 99, HasOrgID: true, TesteeID: 100}
	if fields := mismatchedAssociationFields(catalog, provenMismatch); len(fields) != 1 || fields[0] != "org_id" {
		t.Fatalf("proven org mismatch = %v, want [org_id]", fields)
	}
	provenMatch := catalogSourceEnvelope{AssessmentID: 1, OrgID: 10, HasOrgID: true, TesteeID: 100}
	if fields := mismatchedAssociationFields(catalog, provenMatch); len(fields) != 0 {
		t.Fatalf("proven org match = %v, want none", fields)
	}
}
