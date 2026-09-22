//go:build reliable_messaging

package transaction

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	run "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/stretchr/testify/require"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// This verifies the original durable execution-claim boundary. It neither calls
// a model nor proves that reclaiming an unknown external operation is safe.
func TestReliableMessagingExecutionClaims(t *testing.T) {
	dsn := os.Getenv("RM_QS_ASSESSMENT_DSN")
	if dsn == "" {
		t.Fatal("isolated MySQL DSN required; must not skip")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := gorm.Open(driver.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, db.AutoMigrate(&checkpoint.RuntimeCheckpointPO{}))
	repo := checkpoint.NewRunRepository(db)
	now := time.Now().UTC().Truncate(time.Millisecond)
	request := port.ClaimRequest{AssessmentID: 91, Token: "original", ClaimedAt: now, LeaseUntil: now.Add(time.Minute)}
	original, err := repo.Claim(ctx, request)
	require.NoError(t, err)
	require.True(t, original.Claimed)
	require.NoError(t, original.Run.AttachInputSnapshot("frozen-input-v1"))
	require.NoError(t, repo.SaveClaimed(ctx, original.Run))
	// Actual SQL locking serializes duplicate requests while an owner is live.
	results := make(chan port.ClaimResult, 6)
	errors := make(chan error, 6)
	for i := range 6 {
		go func(i int) {
			duplicate := request
			duplicate.Token = fmt.Sprintf("duplicate-%d", i)
			claimed, e := repo.Claim(ctx, duplicate)
			results <- claimed
			errors <- e
		}(i)
	}
	for range 6 {
		require.NoError(t, <-errors)
		duplicate := <-results
		require.False(t, duplicate.Claimed)
		require.Equal(t, original.Run.ID(), duplicate.Run.ID())
		require.Equal(t, 1, duplicate.Run.Attempt().Number)
	}
	// The repository accepts an explicit clock. Advance that input, not SQL data;
	// this checks its claim contract, not natural wall-clock timeout or skew safety.
	request.Token = "recovered"
	request.ClaimedAt = now.Add(2 * time.Minute)
	request.LeaseUntil = now.Add(3 * time.Minute)
	recovered, err := repo.Claim(ctx, request)
	require.NoError(t, err)
	require.True(t, recovered.Claimed)
	require.Equal(t, original.Run.ID(), recovered.Run.ID())
	require.Equal(t, 1, recovered.Run.Attempt().Number)
	require.Equal(t, "frozen-input-v1", recovered.Run.InputSnapshotRef())
	require.Equal(t, retrygovernance.AttemptOriginLeaseRecovery, recovered.Run.Origin())
	require.NoError(t, original.Run.Succeed(now.Add(2*time.Minute)))
	require.ErrorIs(t, repo.SaveClaimed(ctx, original.Run), port.ErrClaimLost)
	require.NoError(t, recovered.Run.Succeed(now.Add(2*time.Minute)))
	require.NoError(t, repo.SaveClaimed(ctx, recovered.Run))
	finished, err := repo.Claim(ctx, request)
	require.NoError(t, err)
	require.False(t, finished.Claimed)
	require.Equal(t, run.StatusSucceeded, finished.Run.Attempt().Status)

	// An ordinary replay cannot authorize a failed run's next business attempt.
	request.AssessmentID = 92
	request.Token = "failed-owner"
	failed, err := repo.Claim(ctx, request)
	require.NoError(t, err)
	require.True(t, failed.Claimed)
	require.NoError(t, failed.Run.Fail(request.ClaimedAt, run.Failure{Kind: run.FailureKindDependency, Retryable: true, Message: "controlled dependency failure"}))
	require.NoError(t, failed.Run.AttachRetryEvent("authorized-event"))
	require.NoError(t, repo.SaveClaimed(ctx, failed.Run))
	decision := failed.Run.RetryDecision()
	require.NotNil(t, decision)
	require.NotNil(t, decision.NextAttemptAt)
	request.ClaimedAt = decision.NextAttemptAt.Add(time.Second)
	request.LeaseUntil = request.ClaimedAt.Add(time.Minute)
	request.Token = "retry"
	ordinary, err := repo.Claim(ctx, request)
	require.NoError(t, err)
	require.False(t, ordinary.Claimed)
	require.Equal(t, 1, ordinary.Run.Attempt().Number)
	request.ExpectedAttempt = 1
	request.Origin = retrygovernance.AttemptOriginAutomatic
	request.RetryEventID = "wrong-event"
	wrong, err := repo.Claim(ctx, request)
	require.NoError(t, err)
	require.False(t, wrong.Claimed)
	request.RetryEventID = "authorized-event"
	authorized, err := repo.Claim(ctx, request)
	require.NoError(t, err)
	require.True(t, authorized.Claimed)
	require.Equal(t, 2, authorized.Run.Attempt().Number)
	duplicate, err := repo.Claim(ctx, request)
	require.NoError(t, err)
	require.False(t, duplicate.Claimed)
	require.Equal(t, authorized.Run.ID(), duplicate.Run.ID())
	var n int64
	require.NoError(t, db.Model(&checkpoint.RuntimeCheckpointPO{}).Count(&n).Error)
	require.EqualValues(t, 3, n, "one recovered initial attempt plus failed and authorized retry")
}
