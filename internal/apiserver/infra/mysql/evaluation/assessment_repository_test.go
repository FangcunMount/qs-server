package evaluation

import (
	"testing"
)

func TestNewAssessmentRepositoryCreatesCommandRepository(t *testing.T) {
	repo, ok := NewAssessmentRepository(nil).(*assessmentRepository)
	if !ok {
		t.Fatalf("repository type = %T, want *assessmentRepository", repo)
	}
	if repo.mapper == nil {
		t.Fatalf("mapper = nil")
	}
}
