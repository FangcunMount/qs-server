package modelcatalog

import (
	stderrors "errors"

	assessmentmodelpkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

var (
	ErrNotFound            = stderrors.New("assessment model not found")
	ErrVersionRequired     = stderrors.New("assessment model version is required")
	ErrAmbiguousVersion    = stderrors.New("multiple published assessment models matched")
	ErrInvalidArgument     = binding.ErrInvalidArgument
	ErrInvalidState        = assessmentmodelpkg.ErrInvalidState
	ErrNormVersionConflict = stderrors.New("norm table version conflicts with existing content")
)

func IsNotFound(err error) bool {
	return stderrors.Is(err, ErrNotFound)
}

func IsVersionRequired(err error) bool {
	return stderrors.Is(err, ErrVersionRequired)
}
