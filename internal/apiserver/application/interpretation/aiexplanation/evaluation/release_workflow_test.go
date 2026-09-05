package evaluation_test

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/administration"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/governance"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/stretchr/testify/require"
)

// Exercises real administration, runner, evidence and Profile services together.
// Provider/judge, storage, authorization and transaction transport are test doubles;
// this proves workflow behavior, not model quality or Mongo transaction durability.
func TestReleaseWorkflowFromAdministratorStartToProfilePublication(t *testing.T) {
	ctx := context.Background()
	clock := &onlineV2Clock{now: time.Now().UTC().Add(-time.Minute)}
	provider, semantic := &onlineProviderStub{}, &onlineSemanticStub{}
	runner, evidenceService, outbox, committer, repository := newOnlineReleaseHarness(t, clock, provider, semantic, onlineSafetyStub{})
	profiles := &releaseProfileRepository{}
	profileService, err := governance.NewService(profiles, repository, time.Now)
	require.NoError(t, err)
	admin := administration.NewService(nil, profileService, releaseAccess{},
		administration.WithEvaluationV2(runner, evidenceService, committer, func() meta.ID { return meta.ID(9801) }))
	owner := administration.Actor{OrgID: 12, OperatorUserID: 42}
	run, err := admin.StartEvaluationV2(ctx, owner, administration.StartEvaluationV2Command{
		Confirm: true, ExpectedProviderInvocations: 140, Reason: "Exercise frozen release workflow",
	})
	require.NoError(t, err)
	require.Equal(t, "v2", run.Release.GatePolicy.Version)
	for step := 0; step < 140 && run.Status == domainevaluation.EvidenceStatusCollecting; step++ {
		action, nextErr := run.NextAction()
		require.NoError(t, nextErr)
		require.Len(t, outbox.events, step+1)
		result, stepErr := runner.RunStepV2(ctx, onlineV2Command(run, action, outbox.events[step].EventID()))
		require.NoError(t, stepErr)
		run = result.Evidence
	}
	require.Equal(t, domainevaluation.EvidenceStatusAwaitingReview, run.Status)
	require.Equal(t, 35, provider.calls)
	require.Equal(t, 35, semantic.calls)

	suite, err := evaluation.LoadV4()
	require.NoError(t, err)
	definition := suite.ProfileFixture.Definition
	draft, err := admin.CreateProfileDraft(ctx, owner, administration.CreateProfileDraftCommand{
		Definition: definition, ExpectedFingerprint: run.Release.Profile.Fingerprint, Reason: "Create exact evaluated Profile",
	})
	require.NoError(t, err)
	publish := administration.PublishProfileCommand{
		ProfileID: draft.ProfileID(), ProfileVersion: draft.Version(), EvaluationRunID: run.RunID, Reason: "Publish approved release",
	}
	_, err = admin.PublishProfile(ctx, owner, publish)
	require.Error(t, err, "unapproved evidence must not publish a Profile")
	require.Equal(t, domainprofile.StatusDraft, profiles.value.Status())

	for index, role := range []domainevaluation.ReviewRole{domainevaluation.ReviewRoleAssessmentSemantics, domainevaluation.ReviewRoleSafetyProduct} {
		batch := administration.ReviewV2BatchCommand{Role: role}
		for _, slot := range run.Slots {
			batch.Reviews = append(batch.Reviews, administration.ReviewV2BatchItemCommand{
				CandidateID: slot.Candidate.ID, Decision: domainevaluation.ReviewDecisionApprove, Reason: "Synthetic fixture reviewed",
			})
		}
		run, err = admin.RecordReviewsV2(ctx, administration.Actor{OrgID: 12, OperatorUserID: int64(100 + index)}, run.RunID, batch)
		require.NoError(t, err)
	}
	require.Len(t, run.HumanReviews, 70)
	run, err = admin.FinalizeEvaluationV2(ctx, owner, run.RunID, "All frozen evidence has been reviewed")
	require.NoError(t, err)
	require.Equal(t, domainevaluation.EvidenceStatusApproved, run.Status, "gate reasons: %+v", run.GateResult.Reasons)
	require.Equal(t, domainprofile.StatusDraft, profiles.value.Status(), "finalization must not publish automatically")
	published, err := admin.PublishProfile(ctx, owner, publish)
	require.NoError(t, err)
	require.Equal(t, domainprofile.StatusPublished, published.Status())
	require.Equal(t, run.RunID, published.PublishedEvidenceRunID())
	require.Equal(t, run.Release.Profile.Fingerprint, published.Fingerprint())
	_, err = admin.PublishProfile(ctx, owner, publish)
	require.Error(t, err, "published Profile cannot be overwritten")
}

type releaseAccess struct{}

func (releaseAccess) AuthorizeRead(context.Context, administration.Actor) error { return nil }
func (releaseAccess) AuthorizeReview(context.Context, administration.Actor, domainevaluation.ReviewRole) error {
	return nil
}
func (releaseAccess) AuthorizeGovernance(context.Context, administration.Actor) error { return nil }

type releaseProfileRepository struct {
	value *domainprofile.AIExplanationProfile
}

func (r *releaseProfileRepository) Save(_ context.Context, value *domainprofile.AIExplanationProfile) error {
	r.value = value
	return nil
}
func (r *releaseProfileRepository) FindByKey(_ context.Context, id, version string) (*domainprofile.AIExplanationProfile, error) {
	if r.value == nil || r.value.ProfileID() != id || r.value.Version() != version {
		return nil, domainprofile.ErrNotFound
	}
	return r.value, nil
}
func (r *releaseProfileRepository) ListPublishedByBaseSelector(_ context.Context, audience policy.Audience, kind modelcatalog.Kind, decision modelcatalog.DecisionKind) ([]*domainprofile.AIExplanationProfile, error) {
	if r.value != nil && r.value.Status() == domainprofile.StatusPublished {
		selector := r.value.Selector()
		if selector.Audience == audience && selector.ModelKind == kind && selector.DecisionKind == decision {
			return []*domainprofile.AIExplanationProfile{r.value}, nil
		}
	}
	return nil, nil
}
