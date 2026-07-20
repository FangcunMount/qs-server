package questionnaire_test

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestQuestionnaireBumpRevision(t *testing.T) {
	t.Parallel()
	q, err := domain.NewQuestionnaire(meta.NewCode("Q-1"), "Title", domain.WithRevision(3))
	if err != nil {
		t.Fatalf("NewQuestionnaire: %v", err)
	}
	q.BumpRevision()
	if q.GetRevision() != 4 {
		t.Fatalf("revision = %d, want 4", q.GetRevision())
	}
}

func TestIsRevisionConflict(t *testing.T) {
	t.Parallel()
	if !domain.IsRevisionConflict(domain.ErrRevisionConflict) {
		t.Fatal("expected IsRevisionConflict(ErrRevisionConflict)")
	}
	if domain.IsRevisionConflict(domain.ErrNotFound) {
		t.Fatal("did not expect IsRevisionConflict(ErrNotFound)")
	}
}
