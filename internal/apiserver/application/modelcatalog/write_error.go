package modelcatalog

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// MapDraftWriteError maps draft persistence failures to stable application errors.
func MapDraftWriteError(err error) error {
	if domain.IsRevisionConflict(err) {
		return errors.WithCode(code.ErrConflict, "assessment model revision conflict; refresh and retry")
	}
	return err
}
