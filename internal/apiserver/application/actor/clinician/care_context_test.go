package clinician

import (
	"testing"

	relationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
)

func TestPickPreferredCareContext(t *testing.T) {
	t.Parallel()

	result := &TesteeRelationListResult{
		Items: []*TesteeRelationResult{
			nil,
			{
				Relation: &RelationResult{RelationType: string(relationdomain.RelationTypeCollaborator)},
				Clinician: &ClinicianResult{
					ID:   3,
					Name: "collaborator",
				},
			},
			{
				Relation: &RelationResult{RelationType: string(relationdomain.RelationTypePrimary)},
				Clinician: &ClinicianResult{
					ID:   1,
					Name: "primary",
				},
			},
			{
				Relation: &RelationResult{RelationType: string(relationdomain.RelationTypeAttending)},
				Clinician: &ClinicianResult{
					ID:   2,
					Name: "attending",
				},
			},
		},
	}

	selected := pickPreferredCareContext(result)
	if selected == nil || selected.Clinician == nil || selected.Clinician.Name != "primary" {
		t.Fatalf("pickPreferredCareContext() = %+v, want primary clinician", selected)
	}
}

func TestRelationTypePriority(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw  string
		want int
	}{
		{raw: string(relationdomain.RelationTypePrimary), want: 0},
		{raw: string(relationdomain.RelationTypeAttending), want: 1},
		{raw: string(relationdomain.RelationTypeCollaborator), want: 2},
		{raw: string(relationdomain.RelationTypeAssigned), want: 3},
		{raw: string(relationdomain.RelationTypeCreator), want: 4},
		{raw: "unknown", want: 100},
	}

	for _, tc := range cases {
		if got := relationTypePriority(tc.raw); got != tc.want {
			t.Fatalf("relationTypePriority(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestResolveClinicianRole(t *testing.T) {
	t.Parallel()

	if got := resolveClinicianRole(nil); got != "" {
		t.Fatalf("resolveClinicianRole(nil) = %q, want empty", got)
	}
	if got := resolveClinicianRole(&ClinicianResult{Title: "主任医师", ClinicianType: "doctor"}); got != "主任医师" {
		t.Fatalf("resolveClinicianRole(title) = %q, want 主任医师", got)
	}
	if got := resolveClinicianRole(&ClinicianResult{ClinicianType: "counselor"}); got != "counselor" {
		t.Fatalf("resolveClinicianRole(type) = %q, want counselor", got)
	}
}
