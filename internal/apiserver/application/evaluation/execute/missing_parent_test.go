package execute

import (
	"context"
	"errors"
	"testing"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
)

type missingPersistedAssessment struct {
	fakeAssessmentRepo
	reads []domainassessment.ID
}

func (r *missingPersistedAssessment) FindByID(_ context.Context, id domainassessment.ID) (*domainassessment.Assessment, error) {
	r.reads = append(r.reads, id)
	return nil, domainassessment.ErrNotFound
}

type missingParentRunRepository struct {
	stubRunRepo
	claims int
}

func (r *missingParentRunRepository) Claim(ctx context.Context, request evaluationrun.ClaimRequest) (evaluationrun.ClaimResult, error) {
	r.claims++
	return r.stubRunRepo.Claim(ctx, request)
}

func TestEvaluateMissingPersistedAssessmentDoesNotClaimOrCommitOutcome(t *testing.T) {
	t.Parallel()

	repo := &missingPersistedAssessment{}
	runs := &missingParentRunRepository{}
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault}
	// The canonical Outcome write boundary is the injected Committer. It must
	// not be entered at all for a missing persisted Assessment.
	committer := &evaluationCommitterStub{}
	tx := &engineRecordingTxRunner{}
	stager := &engineRecordingEventStager{}
	svc := NewEngine(repo, stubInputResolver{},
		withTestEvaluator(evaluator),
		WithRunRepository(runs),
		WithEvaluationCommitter(committer),
		WithTransactionalOutbox(tx, stager),
	)
	const assessmentID = uint64(7001)
	for attempt := 0; attempt < 2; attempt++ {
		err := svc.Evaluate(context.Background(), assessmentID)
		if !evalerrors.IsAssessmentNotFound(err) || !errors.Is(err, domainassessment.ErrNotFound) {
			t.Fatalf("attempt %d: error=%v, want original missing Assessment", attempt, err)
		}
	}
	if len(repo.reads) != 2 || repo.reads[0].Uint64() != assessmentID || repo.reads[1] != repo.reads[0] {
		t.Fatalf("durable parent reads=%v, want original Assessment twice", repo.reads)
	}
	if repo.saveCalls != 0 || runs.claims != 0 || len(runs.saved) != 0 || runs.latest != nil || committer.calls != 0 || evaluator.calls != 0 || tx.called || len(stager.eventTypes) != 0 || len(stager.scheduledEventTypes) != 0 {
		t.Fatalf("missing parent caused downstream effects: assessment_saves=%d claims=%d run_saves=%d latest=%v outcome_commits=%d evaluations=%d transaction=%t events=%v scheduled=%v",
			repo.saveCalls, runs.claims, len(runs.saved), runs.latest, committer.calls, evaluator.calls, tx.called, stager.eventTypes, stager.scheduledEventTypes)
	}
}
