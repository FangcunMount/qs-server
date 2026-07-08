package publishing

import (
	stderrors "errors"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

var (
	ErrInvalidArgument = binding.ErrInvalidArgument
	ErrInvalidState    = stderrors.New("assessment model invalid state")
)
