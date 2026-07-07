package catalog

import (
	stderrors "errors"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

var (
	ErrInvalidArgument = identity.ErrInvalidArgument
	ErrInvalidState    = stderrors.New("assessment model invalid state")
)
