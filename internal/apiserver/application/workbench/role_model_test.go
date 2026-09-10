package workbench

import (
	"context"
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestClinicalWorkbenchRequiresResultsBeforeQueries(t *testing.T) {
	_, err := (&service{}).GetSummary(context.Background(), Scope{})
	require.True(t, cberrors.IsCode(err, code.ErrPermissionDenied))
	_, err = (&service{}).ListQueue(context.Background(), ListQueueDTO{})
	require.True(t, cberrors.IsCode(err, code.ErrPermissionDenied))
}
