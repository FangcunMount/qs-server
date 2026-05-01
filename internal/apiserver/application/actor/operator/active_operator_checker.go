package operator

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type activeOperatorChecker struct {
	reader actorreadmodel.OperatorReader
}

func NewActiveOperatorChecker(reader actorreadmodel.OperatorReader) ActiveOperatorChecker {
	return &activeOperatorChecker{reader: reader}
}

func (c *activeOperatorChecker) RequireActive(ctx context.Context, orgID int64, userID int64) (*OperatorResult, error) {
	if c == nil || c.reader == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "operator repository not configured")
	}
	op, err := c.reader.FindOperatorByUser(ctx, orgID, userID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrPermissionDenied, "operator not found in current organization")
		}
		return nil, errors.Wrap(err, "operator lookup failed")
	}
	if !op.IsActive {
		return nil, errors.WithCode(code.ErrPermissionDenied, "operator is inactive")
	}
	return toOperatorResultFromRow(op), nil
}
