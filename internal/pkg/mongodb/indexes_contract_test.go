package mongo_indexes_test

import (
	"testing"

	mongo_indexes "github.com/FangcunMount/qs-server/internal/pkg/mongodb"
)

func TestRequiredUnifiedIndexNamesContract(t *testing.T) {
	t.Parallel()

	required := mongo_indexes.RequiredUnifiedIndexNames()
	for collection, names := range map[string][]string{
		"assessment_models": {
			"idx_assessment_models_head_code",
			"idx_assessment_models_snapshot_identity_version_v2",
			"idx_assessment_models_active_code",
			"idx_assessment_models_active_questionnaire",
			"idx_assessment_models_active_catalog",
			"idx_assessment_models_release_history",
		},
		"questionnaires": {
			"idx_questionnaires_head_code",
			"idx_questionnaires_snapshot_version",
			"idx_questionnaires_active_code",
			"idx_questionnaires_release_history",
		},
		"assessment_norms": {
			"idx_assessment_norms_table_version",
		},
	} {
		got := required[collection]
		if len(got) != len(names) {
			t.Fatalf("%s index count = %d, want %d (%v)", collection, len(got), len(names), got)
		}
		for i := range names {
			if got[i] != names[i] {
				t.Fatalf("%s[%d] = %q, want %q", collection, i, got[i], names[i])
			}
		}
	}

	forbidden := mongo_indexes.ForbiddenLegacyIndexNames()
	if forbidden["assessment_models"][0] != "idx_assessment_models_code" {
		t.Fatalf("forbidden assessment_models = %#v", forbidden["assessment_models"])
	}
	if forbidden["questionnaires"][0] != "idx_code_version" {
		t.Fatalf("forbidden questionnaires = %#v", forbidden["questionnaires"])
	}
}
