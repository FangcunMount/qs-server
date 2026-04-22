package service

import (
	"testing"

	clinicianapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	assessmententrydomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	cliniciandomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	relationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
)

func TestPickPreferredCareContext(t *testing.T) {
	t.Parallel()

	result := &clinicianapp.TesteeRelationListResult{
		Items: []*clinicianapp.TesteeRelationResult{
			nil,
			{
				Relation: &clinicianapp.RelationResult{RelationType: string(relationdomain.RelationTypeCollaborator)},
				Clinician: &clinicianapp.ClinicianResult{
					ID:   3,
					Name: "collaborator",
				},
			},
			{
				Relation: &clinicianapp.RelationResult{RelationType: string(relationdomain.RelationTypePrimary)},
				Clinician: &clinicianapp.ClinicianResult{
					ID:   1,
					Name: "primary",
				},
			},
			{
				Relation: &clinicianapp.RelationResult{RelationType: string(relationdomain.RelationTypeAttending)},
				Clinician: &clinicianapp.ClinicianResult{
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
	if got := resolveClinicianRole(&clinicianapp.ClinicianResult{Title: "主任医师", ClinicianType: "doctor"}); got != "主任医师" {
		t.Fatalf("resolveClinicianRole(title) = %q, want 主任医师", got)
	}
	if got := resolveClinicianRole(&clinicianapp.ClinicianResult{ClinicianType: "counselor"}); got != "counselor" {
		t.Fatalf("resolveClinicianRole(type) = %q, want counselor", got)
	}
}

func TestBuildAssessmentEntryTitle(t *testing.T) {
	t.Parallel()

	if got := buildAssessmentEntryTitle(nil); got != "" {
		t.Fatalf("buildAssessmentEntryTitle(nil) = %q, want empty", got)
	}

	item := assessmententrydomain.NewAssessmentEntry(
		1,
		cliniciandomain.ID(9),
		"token-1",
		assessmententrydomain.TargetTypeQuestionnaire,
		"PHQ9",
		"v2",
		true,
		nil,
	)
	if got := buildAssessmentEntryTitle(item); got != "questionnaire:PHQ9@v2" {
		t.Fatalf("buildAssessmentEntryTitle(versioned) = %q, want questionnaire:PHQ9@v2", got)
	}

	itemNoVersion := assessmententrydomain.NewAssessmentEntry(
		1,
		cliniciandomain.ID(9),
		"token-2",
		assessmententrydomain.TargetTypeScale,
		"GAD7",
		"",
		true,
		nil,
	)
	if got := buildAssessmentEntryTitle(itemNoVersion); got != "scale:GAD7" {
		t.Fatalf("buildAssessmentEntryTitle(unversioned) = %q, want scale:GAD7", got)
	}
}
