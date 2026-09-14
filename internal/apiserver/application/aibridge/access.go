package aibridge

import (
	"context"
	"errors"
	"strconv"

	evaluationtestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
)

var ErrAccessDenied = errors.New("AI workflow access denied")
var ErrAccessUnavailable = errors.New("AI workflow authorization unavailable")

// CurrentAccess rechecks durable participant identity; submission-time delegation
// is not a permanent grant. Transport must restrict this use case to the AI workload.
type CurrentAccess struct {
	Testees interface {
		GetTestee(context.Context, uint64) (*actorreadmodel.TesteeRow, error)
	}
	Links interface {
		HasActiveProfileLink(context.Context, string, string) (bool, error)
	}
	Assessments interface {
		AuthorizeAssessment(context.Context, evaluationtestee.Actor, uint64) error
	}
}

func (a *CurrentAccess) Authorize(ctx context.Context, actor Actor, testeeID string, assessmentIDs []string) error {
	if !validNumber(actor.OrgID) || actor.SubjectID == "" || len(actor.SubjectID) > 128 || !validNumber(testeeID) || len(assessmentIDs) == 0 || len(assessmentIDs) > 10 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range assessmentIDs {
		if !validNumber(id) || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
	}
	if a == nil || a.Testees == nil || a.Links == nil || a.Assessments == nil {
		return ErrAccessUnavailable
	}
	id, _ := strconv.ParseUint(testeeID, 10, 64)
	testee, err := a.Testees.GetTestee(ctx, id)
	if err != nil {
		return errors.Join(ErrAccessUnavailable, err)
	}
	if testee == nil || testee.ID != id || testee.OrgID <= 0 || strconv.FormatInt(testee.OrgID, 10) != actor.OrgID || testee.ProfileID == nil || *testee.ProfileID == 0 {
		return ErrAccessDenied
	}
	allowed, err := a.Links.HasActiveProfileLink(ctx, actor.SubjectID, strconv.FormatUint(*testee.ProfileID, 10))
	if err != nil {
		return errors.Join(ErrAccessUnavailable, err)
	}
	if !allowed {
		return ErrAccessDenied
	}
	for _, assessment := range assessmentIDs {
		assessmentID, _ := strconv.ParseUint(assessment, 10, 64)
		if err := a.Assessments.AuthorizeAssessment(ctx, evaluationtestee.Actor{TesteeID: id}, assessmentID); err != nil {
			return err
		}
	}
	return nil
}

// ResolveParticipantActor binds an authenticated participant to QS-owned business
// context. A consumer login need not contain an organization claim. A supplied
// organization must match; it is never replaced with a different organization.
func (a *CurrentAccess) ResolveParticipantActor(ctx context.Context, subjectID string, claimedOrgID, testeeID, assessmentID uint64) (Actor, error) {
	if subjectID == "" || len(subjectID) > 128 || testeeID == 0 || assessmentID == 0 {
		return Actor{}, ErrInvalid
	}
	if a == nil || a.Testees == nil || a.Links == nil || a.Assessments == nil {
		return Actor{}, ErrAccessUnavailable
	}
	testee, err := a.Testees.GetTestee(ctx, testeeID)
	if err != nil {
		return Actor{}, errors.Join(ErrAccessUnavailable, err)
	}
	if testee == nil || testee.ID != testeeID || testee.OrgID <= 0 || (claimedOrgID != 0 && claimedOrgID != uint64(testee.OrgID)) {
		return Actor{}, ErrAccessDenied
	}
	actor := Actor{OrgID: strconv.FormatInt(testee.OrgID, 10), SubjectID: subjectID}
	// Reuse the execution-time authority: current profile link and assessment
	// ownership must both hold before this actor can reach any workflow operation.
	if err := a.Authorize(ctx, actor, strconv.FormatUint(testeeID, 10), []string{strconv.FormatUint(assessmentID, 10)}); err != nil {
		return Actor{}, err
	}
	return actor, nil
}
