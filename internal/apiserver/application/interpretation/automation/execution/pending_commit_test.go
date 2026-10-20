package execution

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/stretchr/testify/require"
)

type pendingCommitterStub struct {
	InterpretationCommitter
	check func(int) (*CommitResult, bool, error)
	calls int
}

func (s *pendingCommitterStub) RecoverPending(context.Context, generation.Key, int64) (*CommitResult, bool, error) {
	s.calls++
	return s.check(s.calls)
}

type countedStarter struct {
	original Starter
	calls    int
}

func (s *countedStarter) Start(ctx context.Context, q StartRequest) (*StartResult, error) {
	s.calls++
	return s.original.Start(ctx, q)
}

func TestPendingTerminalUnknownNeverClaimsOrBuildsAgain(t *testing.T) {
	for _, stage := range []string{"before-claim", "after-claim"} {
		t.Run(stage, func(t *testing.T) {
			builder := &executorBuilder{}
			service, _, _, _, _, _ := newExecutorFixture(t, builder)
			start := &countedStarter{original: service.starter}
			service.starter = start
			check := &pendingCommitterStub{InterpretationCommitter: service.committer, check: func(call int) (*CommitResult, bool, error) {
				if stage == "after-claim" && call == 1 {
					return nil, false, nil
				}
				return nil, true, port.ErrUnresolved
			}}
			service.committer = check
			_, err := service.Execute(t.Context(), executorInput(), "original")
			require.ErrorIs(t, err, port.ErrUnresolved)
			require.Zero(t, builder.calls)
			expected := 0
			if stage == "after-claim" {
				expected = 1
			}
			require.Equal(t, expected, start.calls)
			_, err = service.Execute(t.Context(), executorInput(), "redelivery")
			require.ErrorIs(t, err, port.ErrUnresolved)
			require.Equal(t, expected, start.calls)
			require.Zero(t, builder.calls)
		})
	}
}

func TestPendingTerminalRecoveryReturnsOriginalArtifactWithoutBuilder(t *testing.T) {
	builder := &executorBuilder{}
	service, _, runs, _, _, _ := newExecutorFixture(t, builder)
	first, err := service.Execute(t.Context(), executorInput(), "original")
	require.NoError(t, err)
	r, err := runs.FindByID(t.Context(), first.Generation.LatestRunID())
	require.NoError(t, err)
	service.committer = &pendingCommitterStub{InterpretationCommitter: service.committer, check: func(int) (*CommitResult, bool, error) {
		return &CommitResult{Generation: first.Generation, Run: r, InterpretReport: first.InterpretReport}, true, nil
	}}
	start := &countedStarter{original: service.starter}
	service.starter = start
	again, err := service.Execute(t.Context(), executorInput(), "new-instance")
	require.NoError(t, err)
	require.Equal(t, first.InterpretReport.ID(), again.InterpretReport.ID())
	require.Zero(t, start.calls)
	require.Equal(t, 1, builder.calls)
}

func TestPendingTerminalIncompleteResultFailsClosedBeforeStart(t *testing.T) {
	service, _, _, _, _, _ := newExecutorFixture(t, &executorBuilder{})
	service.committer = &pendingCommitterStub{InterpretationCommitter: service.committer, check: func(int) (*CommitResult, bool, error) { return nil, true, nil }}
	start := &countedStarter{original: service.starter}
	service.starter = start
	_, err := service.Execute(t.Context(), executorInput(), "unknown")
	require.ErrorIs(t, err, port.ErrUnresolved)
	require.Zero(t, start.calls)
}
