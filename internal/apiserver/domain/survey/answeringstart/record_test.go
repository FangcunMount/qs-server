package answeringstart

import (
	sheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestStartReplayDoesNotBorrowNewOwnership(t *testing.T) {
	i := Intent{OrgID: 1, UserID: 2, TesteeID: 3, RequestKey: "first-attempt", QuestionnaireCode: "q", QuestionnaireVersion: "1", Origin: sheet.OriginRef{Type: sheet.OriginTypeSelfService}}
	store := uint64(10)
	r, err := New(i, 4, &store, 1, time.Now())
	require.NoError(t, err)
	store = 20
	require.True(t, r.Matches(i))
	require.Equal(t, uint64(10), *r.Context().StoreID())
	i.TesteeID = 5
	require.False(t, r.Matches(i))
	i = r.Intent()
	i.QuestionnaireVersion = "2"
	require.False(t, r.Matches(i))
	_, err = Restore(r.Intent(), "damaged", r.Context())
	require.Error(t, err)
	_, err = Restore(r.Intent(), r.Hash(), sheet.StartContext{})
	require.Error(t, err)
}
