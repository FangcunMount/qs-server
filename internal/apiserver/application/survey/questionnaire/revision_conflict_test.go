package questionnaire

import (
	"context"
	"testing"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestContentServiceMapsRevisionConflictTo409(t *testing.T) {
	t.Parallel()
	service := &contentService{repo: revisionConflictRepository{}}
	err := service.persistQuestionnaire(context.Background(), nil, "Q-CAS", "update")
	if got := baseerrors.ParseCoder(err).Code(); got != code.ErrConflict {
		t.Fatalf("code = %d, want %d", got, code.ErrConflict)
	}
}

type revisionConflictRepository struct {
	domain.Repository
}

func (revisionConflictRepository) Update(context.Context, *domain.Questionnaire) error {
	return domain.ErrRevisionConflict
}
