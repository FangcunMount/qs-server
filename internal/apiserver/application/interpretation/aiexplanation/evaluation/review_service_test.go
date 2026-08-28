package evaluation_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
)

func TestReviewServiceProjectsSyntheticEvidenceAndCompletesDualRoleWorkflow(t *testing.T) {
	provider := &onlineProviderStub{}
	semantic := &onlineSemanticStub{}
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunner(t, promptResolverStub{}, provider, semantic, repository)
	result, err := runner.RunV1(context.Background(), evaluation.OnlineRunCommand{})
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := evaluation.NewEvidenceService(repository, nil, func() time.Time {
		return time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	service, err := evaluation.NewReviewService(evidence)
	if err != nil {
		t.Fatal(err)
	}

	packet, err := service.Find(context.Background(), result.Run.ID())
	if err != nil {
		t.Fatal(err)
	}
	if packet.Status != domainevaluation.StatusAwaitingReview || !packet.CanReview || packet.CanFinalize {
		t.Fatalf("initial review state = %#v", packet)
	}
	if len(packet.Attempts) != domainevaluation.RequiredGenerationAttempts ||
		packet.Progress.RequiredReviews != domainevaluation.RequiredGenerationAttempts*2 ||
		packet.Progress.RecordedReviews != 0 || packet.Progress.MissingReviews != domainevaluation.RequiredGenerationAttempts*2 {
		t.Fatalf("initial review progress = %#v", packet.Progress)
	}
	var assessment map[string]json.RawMessage
	if err := json.Unmarshal(packet.Attempts[0].AssessmentInput, &assessment); err != nil {
		t.Fatal(err)
	}
	if len(assessment) != 2 || assessment["context"] == nil || assessment["facts"] == nil {
		t.Fatalf("review assessment input is not minimized: %s", packet.Attempts[0].AssessmentInput)
	}
	if len(packet.Attempts[0].NormalizedOutput) == 0 || packet.Attempts[0].Semantic == nil || len(packet.Attempts[0].MissingRoles) != 2 {
		t.Fatalf("review attempt evidence = %#v", packet.Attempts[0])
	}
	if _, err := service.Finalize(context.Background(), packet.RunID, "release-owner", "premature"); err == nil {
		t.Fatal("review service must reject finalization before all dual-role reviews are recorded")
	}

	first := packet.Attempts[0]
	packet, err = service.RecordHumanReview(context.Background(), packet.RunID, evaluation.HumanReviewCommand{
		CaseID: first.CaseID, Attempt: first.Attempt, Role: domainevaluation.ReviewRoleAssessmentSemantics,
		Reviewer: "assessment-reviewer", Decision: domainevaluation.ReviewDecisionApprove, Reason: "semantic relationship and evidence reviewed",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RecordHumanReview(context.Background(), packet.RunID, evaluation.HumanReviewCommand{
		CaseID: first.CaseID, Attempt: first.Attempt, Role: domainevaluation.ReviewRoleSafetyProduct,
		Reviewer: "assessment-reviewer", Decision: domainevaluation.ReviewDecisionApprove, Reason: "safety reviewed",
	})
	if err == nil {
		t.Fatal("same reviewer must not satisfy both required roles")
	}
	packet, err = service.RecordHumanReview(context.Background(), packet.RunID, evaluation.HumanReviewCommand{
		CaseID: first.CaseID, Attempt: first.Attempt, Role: domainevaluation.ReviewRoleSafetyProduct,
		Reviewer: "safety-reviewer", Decision: domainevaluation.ReviewDecisionApprove, Reason: "safety and product boundaries reviewed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if packet.Progress.FullyReviewedAttempts != 1 || packet.Progress.RecordedReviews != 2 {
		t.Fatalf("partial review progress = %#v", packet.Progress)
	}

	for _, attempt := range packet.Attempts[1:] {
		for _, review := range []evaluation.HumanReviewCommand{
			{
				CaseID: attempt.CaseID, Attempt: attempt.Attempt, Role: domainevaluation.ReviewRoleAssessmentSemantics,
				Reviewer: "assessment-reviewer", Decision: domainevaluation.ReviewDecisionApprove, Reason: "semantic relationship and evidence reviewed",
			},
			{
				CaseID: attempt.CaseID, Attempt: attempt.Attempt, Role: domainevaluation.ReviewRoleSafetyProduct,
				Reviewer: "safety-reviewer", Decision: domainevaluation.ReviewDecisionApprove, Reason: "safety and product boundaries reviewed",
			},
		} {
			packet, err = service.RecordHumanReview(context.Background(), packet.RunID, review)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if !packet.Progress.AllRequiredReviewsRecorded || !packet.CanFinalize || packet.Progress.RecordedReviews != 70 {
		t.Fatalf("complete review progress = %#v", packet.Progress)
	}

	packet, err = service.Finalize(context.Background(), packet.RunID, "release-owner", "all frozen evidence reviewed")
	if err != nil {
		t.Fatal(err)
	}
	if packet.Status != domainevaluation.StatusApproved || packet.CanReview || packet.CanFinalize || packet.Gate == nil || !packet.Gate.Passed || packet.Finalized == nil {
		t.Fatalf("final review state = %#v", packet)
	}
}

func TestReviewServiceProjectsRemainingRecoveryProviderInvocationCeiling(t *testing.T) {
	fixed := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunnerWithClock(t, promptResolverStub{}, &onlineProviderStub{}, &onlineSemanticStub{}, repository, func() time.Time { return fixed })
	started, err := runner.StartV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := evaluation.NewEvidenceService(repository, nil, func() time.Time { return fixed })
	if err != nil {
		t.Fatal(err)
	}
	service, err := evaluation.NewReviewService(evidence)
	if err != nil {
		t.Fatal(err)
	}

	assertCeiling := func(want int) {
		t.Helper()
		packet, findErr := service.Find(context.Background(), started.Run.ID())
		if findErr != nil {
			t.Fatal(findErr)
		}
		if packet.RecoveryMaxProviderInvocations != want {
			t.Fatalf("recovery provider invocation ceiling = %d, want %d", packet.RecoveryMaxProviderInvocations, want)
		}
	}
	assertCeiling(70)

	if err := repository.value.BeginAttemptExecution(domainevaluation.AttemptExecution{
		CaseID: "PROMPT-EVAL-001", Attempt: 1, Owner: "worker-1", InvocationID: "invocation-1",
		Phase: domainevaluation.AttemptExecutionPrepared, ClaimedAt: fixed, LeaseExpiresAt: fixed.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	assertCeiling(70)

	if err := repository.value.MarkAttemptDispatching("worker-1", fixed.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	// An expired dispatch is recorded as result_unknown and is never replayed,
	// so recovery can invoke only the remaining 34 generation/judge pairs.
	assertCeiling(68)
}

func TestReviewServiceListsOrganizationScopedReviewQueue(t *testing.T) {
	fixed := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunnerWithClock(t, promptResolverStub{}, &onlineProviderStub{}, &onlineSemanticStub{}, repository, func() time.Time { return fixed })
	started, err := runner.StartV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	requested, err := domainevaluation.NewRequested(91, started.Run.Release(), 7, "user:42", "review release candidate", fixed)
	if err != nil {
		t.Fatal(err)
	}
	repository.value = requested
	repository.list = []evaluation.ReviewRunCatalogRecord{catalogRecordFromRun(requested)}
	repository.nextCursor = "next-review-page"
	evidence, err := evaluation.NewEvidenceService(repository, nil, func() time.Time { return fixed })
	if err != nil {
		t.Fatal(err)
	}
	service, err := evaluation.NewReviewService(evidence)
	if err != nil {
		t.Fatal(err)
	}
	status := domainevaluation.StatusCollecting
	page, err := service.List(context.Background(), evaluation.ReviewRunListQuery{OrgID: 7, Status: &status, Cursor: "current", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if repository.listOrgID != 7 || repository.listStatus == nil || *repository.listStatus != status || repository.listCursor != "current" || repository.listLimit != 5 ||
		len(page.Items) != 1 || page.Items[0].RunID != requested.ID() || page.Items[0].RequestedOrgID != 7 || page.NextCursor != "next-review-page" {
		t.Fatalf("review catalog query/page = org:%d status:%v cursor:%q limit:%d page:%#v", repository.listOrgID, repository.listStatus, repository.listCursor, repository.listLimit, page)
	}

	other, err := domainevaluation.NewRequested(92, started.Run.Release(), 8, "user:99", "other organization", fixed)
	if err != nil {
		t.Fatal(err)
	}
	repository.list = []evaluation.ReviewRunCatalogRecord{catalogRecordFromRun(other)}
	if _, err := service.List(context.Background(), evaluation.ReviewRunListQuery{OrgID: 7, Limit: 5}); err == nil {
		t.Fatal("review catalog must reject a cross-organization projection")
	}
}

func TestReviewServiceListDerivesProgressWithoutReturningDetailedEvidence(t *testing.T) {
	fixed := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	repository := &onlineEvidenceRepository{}
	runner := newOnlineRunnerWithClock(t, promptResolverStub{}, &onlineProviderStub{}, &onlineSemanticStub{}, repository, func() time.Time { return fixed })
	result, err := runner.RunV1(context.Background(), evaluation.OnlineRunCommand{})
	if err != nil {
		t.Fatal(err)
	}
	record := catalogRecordFromRun(result.Run)
	record.RequestedOrgID = 7
	record.RequestedBy = "user:42"
	record.RequestReason = "review release candidate"
	repository.list = []evaluation.ReviewRunCatalogRecord{record}
	evidence, err := evaluation.NewEvidenceService(repository, nil, func() time.Time { return fixed })
	if err != nil {
		t.Fatal(err)
	}
	service, err := evaluation.NewReviewService(evidence)
	if err != nil {
		t.Fatal(err)
	}
	status := domainevaluation.StatusAwaitingReview
	page, err := service.List(context.Background(), evaluation.ReviewRunListQuery{OrgID: 7, Status: &status})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("review queue size = %d", len(page.Items))
	}
	item := page.Items[0]
	if len(item.Attempts) != 0 || item.Execution != nil || len(item.Recoveries) != 0 {
		t.Fatalf("review queue returned detailed evidence: %#v", item)
	}
	if item.Progress.GenerationAttempts != domainevaluation.RequiredGenerationAttempts ||
		item.Progress.RequiredReviews != domainevaluation.RequiredGenerationAttempts*2 ||
		item.Progress.MissingReviews != domainevaluation.RequiredGenerationAttempts*2 || !item.CanReview || item.CanFinalize || item.CanCancel {
		t.Fatalf("review queue progress = %#v", item)
	}

	for index := range repository.list[0].Attempts {
		if repository.list[0].Attempts[index].Stage == domainevaluation.AttemptStageGeneration {
			repository.list[0].Attempts[index].Failed = true
			break
		}
	}
	page, err = service.List(context.Background(), evaluation.ReviewRunListQuery{OrgID: 7, Status: &status})
	if err != nil {
		t.Fatal(err)
	}
	failed := page.Items[0]
	if failed.Progress.FailedAttempts != 1 || failed.CanReview || failed.CanFinalize || !failed.CanCancel {
		t.Fatalf("failed review queue state = %#v", failed)
	}
}
