package questionnaire

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
)

func TestSameImmutableQuestionnaireContentIgnoresReleaseStateButRejectsQuestionChange(t *testing.T) {
	active := &QuestionnairePO{Code: "Q-1", Version: "2", Status: "published", RecordRole: domain.RecordRolePublishedSnapshot.String(), ReleaseStatus: string(domain.ReleaseStatusActive), Questions: []QuestionPO{{Code: "Q1", Title: "Before"}}}
	archived := *active
	archived.ReleaseStatus = string(domain.ReleaseStatusArchived)
	if !sameImmutableQuestionnaireContent(active, &archived) {
		t.Fatal("release metadata must not alter immutable content identity")
	}
	conflict := archived
	conflict.Questions = []QuestionPO{{Code: "Q1", Title: "After"}}
	if sameImmutableQuestionnaireContent(active, &conflict) {
		t.Fatal("question change under the same release version must conflict")
	}
}
