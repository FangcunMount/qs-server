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
