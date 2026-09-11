package aibridge

import (
	"context"
	"errors"
	"strconv"

	source "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/source"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Participant accepts only the actor established by the existing delegated QS boundary.
// QS owns authorization; the AI receives an immutable, authorized report snapshot.
type Participant struct {
	Access interface {
		AuthorizeOwnAssessment(context.Context, uint64, uint64) error
	}
	Sources source.Resolver
	Bridge  *Service
}

func (p *Participant) Request(ctx context.Context, actor Actor, testeeID, assessmentID, reportID uint64, requestID string) error {
	if !validID(requestID) || !validNumber(actor.OrgID) || actor.SubjectID == "" || testeeID == 0 || assessmentID == 0 || reportID == 0 {
		return ErrInvalid
	}
	if err := p.Access.AuthorizeOwnAssessment(ctx, testeeID, assessmentID); err != nil {
		return err
	}
	previous, err := p.Bridge.Store.Original(ctx, requestID)
	if err == nil {
		if previous.Actor != actor || previous.TesteeID != strconv.FormatUint(testeeID, 10) || len(previous.AssessmentIDs) != 1 || previous.AssessmentIDs[0] != strconv.FormatUint(assessmentID, 10) || len(previous.Evidence) != 1 || previous.Evidence[0].ReportID != strconv.FormatUint(reportID, 10) {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	current, err := p.Sources.ResolveCurrent(ctx, meta.FromUint64(assessmentID))
	if err != nil {
		return err
	}
	if current == nil || current.Report == nil {
		return source.ErrInconsistent
	}
	report := current.Report
	association := report.Association()
	if association.TesteeID != testeeID || association.AssessmentID.Uint64() != assessmentID || strconv.FormatInt(association.OrgID, 10) != actor.OrgID {
		return ErrInvalid
	}
	// Require the report selected by the caller. A retry cannot silently switch source versions.
	if report.ID().Uint64() != reportID {
		return ErrConflict
	}
	content, err := reportSnapshot(current)
	if err != nil {
		return err
	}
	return p.Bridge.Start(ctx, Start{
		RequestID: requestID, Actor: actor, TesteeID: strconv.FormatUint(testeeID, 10),
		AssessmentIDs: []string{strconv.FormatUint(assessmentID, 10)}, Goal: "解读当前标准报告",
		Evidence: []EvidenceItem{{AssessmentID: strconv.FormatUint(assessmentID, 10), TesteeID: strconv.FormatUint(testeeID, 10), ReportID: report.ID().String(),
			SourceVersion: report.ContentSchemaVersion() + ":" + report.OutcomeID().String(),
			Facts:         []Fact{{Ref: "standard_report", Value: string(content)}}}},
	})
}
