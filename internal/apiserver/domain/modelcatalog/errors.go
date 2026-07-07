package modelcatalog

import (
	stderrors "errors"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

var (
	ErrNotFound         = stderrors.New("assessment model not found")
	ErrVersionRequired  = stderrors.New("assessment model version is required")
	ErrAmbiguousVersion = stderrors.New("multiple published assessment models matched")
	ErrInvalidArgument  = identity.ErrInvalidArgument
	ErrInvalidState     = catalog.ErrInvalidState
)

func IsNotFound(err error) bool {
	return stderrors.Is(err, ErrNotFound)
}

func IsVersionRequired(err error) bool {
	return stderrors.Is(err, ErrVersionRequired)
}
