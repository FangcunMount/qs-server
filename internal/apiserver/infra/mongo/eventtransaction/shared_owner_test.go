package eventtransaction_test

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	guarded "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

type submissionRoutes struct{ topic string }

func (r submissionRoutes) GetTopicForEvent(string) (string, bool) { return r.topic, true }

type constructorRunner struct{ t *testing.T }

func (r constructorRunner) WithinTransaction(context.Context, func(context.Context) error) error {
	r.t.Fatal("constructor opened a transaction")
	return nil
}

func TestSubmissionAdaptersBorrowOneOwnerAndRejectDifferentDatabase(t *testing.T) {
	owner, _, db, _, _ := guarded.SharedOwnerFixture(t)
	repo := answersheet.NewRepositoryWithoutSchema(db)
	runner := constructorRunner{t: t}
	first, err := answersheet.NewRocketMQSubmissionStoreWithOwner(repo, runner, submissionRoutes{"answers.lifecycle"}, owner)
	require.NoError(t, err)
	second, err := answersheet.NewRocketMQSubmissionStoreWithOwner(repo, runner, submissionRoutes{"answers.lifecycle"}, owner)
	require.NoError(t, err)
	require.Same(t, owner.Runtime(), first.Runtime())
	require.Same(t, first.Runtime(), second.Runtime())
	require.False(t, owner.Runtime().Stats().Ready)
	wrong := answersheet.NewRepositoryWithoutSchema(db.Client().Database("wrong_database"))
	_, err = answersheet.NewRocketMQSubmissionStoreWithOwner(wrong, runner, submissionRoutes{"answers.lifecycle"}, owner)
	require.ErrorIs(t, err, transaction.ErrBinding)
	_, err = answersheet.NewRocketMQSubmissionStoreWithOwner(repo, runner, submissionRoutes{"unregistered.topic"}, owner)
	require.ErrorIs(t, err, transaction.ErrBinding)
	_, err = answersheet.NewRocketMQSubmissionStoreWithOwner(repo, runner, submissionRoutes{"answers.lifecycle"}, nil)
	require.ErrorIs(t, err, transaction.ErrBinding)
}
