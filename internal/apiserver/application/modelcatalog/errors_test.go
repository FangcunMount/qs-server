package modelcatalog

import (
	"testing"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestMapDraftWriteErrorMapsRevisionConflictTo409(t *testing.T) {
	t.Parallel()
	err := MapDraftWriteError(domain.ErrRevisionConflict)
	if got := baseerrors.ParseCoder(err).Code(); got != code.ErrConflict {
		t.Fatalf("code = %d, want %d", got, code.ErrConflict)
	}
}

func TestValidationFailedErrorMapsDomainIssues(t *testing.T) {
	t.Parallel()
	err := NewValidationFailedError([]domain.DomainValidationIssue{{
		Field:   "definition_v2.measure",
		Code:    "measure.required",
		Message: "measure is required",
		Level:   domain.ValidationLevelError,
	}})
	validation, ok := ValidationFailedFrom(err)
	if !ok {
		t.Fatalf("error = %T, want *ValidationFailedError", err)
	}
	if validation.Result.Passed || validation.Result.Valid {
		t.Fatalf("result = %#v, want failed validation", validation.Result)
	}
	if got := validation.Result.Issues[0]; got.Field != "definition_v2.measure" || got.Code != "measure.required" || got.Level != "error" {
		t.Fatalf("issue = %#v", got)
	}
}
