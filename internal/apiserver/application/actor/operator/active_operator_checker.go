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

func (c *activeOperatorChecker) ResolveActive(ctx context.Context, userID int64, requestedOrgID int64) (*OperatorResult, error) {
	if c == nil || c.reader == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "operator repository not configured")
	}
	if requestedOrgID > 0 {
		return c.RequireActive(ctx, requestedOrgID, userID)
	}
	rows, err := c.reader.ListOperators(ctx, actorreadmodel.OperatorFilter{
		UserID:     userID,
		ActiveOnly: true,
		Limit:      2,
	})
	if err != nil {
		return nil, errors.Wrap(err, "operator membership lookup failed")
	}
	switch len(rows) {
	case 0:
		return nil, errors.WithCode(code.ErrPermissionDenied, "operator not found in any organization")
	case 1:
		if !rows[0].IsActive {
			return nil, errors.WithCode(code.ErrPermissionDenied, "operator is inactive")
		}
		return toOperatorResultFromRow(&rows[0]), nil
	default:
		return nil, errors.WithCode(code.ErrInvalidArgument, "multiple active organizations; specify X-Org-Id")
	}
}
