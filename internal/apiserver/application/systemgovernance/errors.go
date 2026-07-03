package systemgovernance

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func errActionsUnavailable() error {
	return errors.WithCode(code.ErrInternalServerError, "governance action executor unavailable")
}
