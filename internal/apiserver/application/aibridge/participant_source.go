package aibridge

import (
	"context"
	"errors"
	source "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reportsource"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"strconv"
)

// ParticipantSource describes current QS facts, not AI Profile eligibility or quota.
type ParticipantSource struct{ Status, ReportID, SourceVersion string }

func (p *Participant) Source(ctx context.Context, actor Actor, testeeID, assessmentID uint64) (*ParticipantSource, error) {
	if !validNumber(actor.OrgID) || actor.SubjectID == "" || len(actor.SubjectID) > 128 || testeeID == 0 || assessmentID == 0 || p == nil || p.Access == nil || p.Sources == nil {
		return nil, ErrInvalid
	}
	if err := p.Access.AuthorizeOwnAssessment(ctx, testeeID, assessmentID); err != nil {
		return nil, err
	}
	current, err := p.Sources.ResolveCurrent(ctx, meta.FromUint64(assessmentID))
	if errors.Is(err, source.ErrNotReady) {
		return &ParticipantSource{Status: "not_ready"}, nil
	}
	if errors.Is(err, source.ErrNotApplicable) {
		return &ParticipantSource{Status: "not_applicable"}, nil
	}
	if err != nil {
		return nil, err
	}
	if current == nil || current.Report == nil {
		return nil, source.ErrInconsistent
	}
	r := current.Report
	association := r.Association()
	if association.TesteeID != testeeID || association.AssessmentID.Uint64() != assessmentID || strconv.FormatInt(association.OrgID, 10) != actor.OrgID {
		return nil, source.ErrInconsistent
	}
	if _, err := reportSnapshot(current); err != nil {
		return nil, err
	}
	return &ParticipantSource{Status: "ready", ReportID: r.ID().String(), SourceVersion: r.ContentSchemaVersion() + ":" + r.OutcomeID().String()}, nil
}
