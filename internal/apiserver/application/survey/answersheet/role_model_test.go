package answersheet

import (
	"context"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/stretchr/testify/require"
)

func TestIndependentRolesDenyAnswerSheetResultsBeforeRepository(t *testing.T) {
	s := &managementService{}
	ctx := context.Background()

	_, err := s.GetByIDInOrg(ctx, 1, 9)
	require.True(t, cberrors.IsCode(err, code.ErrPermissionDenied), "%v", err)

	_, err = s.List(ctx, ListAnswerSheetsDTO{OrgID: 1})
	require.True(t, cberrors.IsCode(err, code.ErrPermissionDenied), "%v", err)
}
