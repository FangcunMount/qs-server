package aibridge

import (
	"context"
	"errors"
	"strconv"

	source "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reportsource"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Participant accepts only the actor established by the existing delegated QS boundary.
// QS owns authorization; the AI receives an immutable, authorized report snapshot.
type Participant struct {
	IntakeClosed bool
	Access       interface {
		AuthorizeOwnAssessment(context.Context, uint64, uint64) error
	}
	Sources     source.Resolver
	Bridge      *Service
	Eligibility EligibilityReader
}

// PlannedStartReader exposes private frozen input for authorization/idempotency
// only. A plan is not an accepted request and must never replace Store.Original
// for normal business reads. The host still verifies current participant access.
type PlannedStartReader interface {
	ReadPlannedStart(context.Context, Actor, string) (*Start, bool, error)
}

// Read checks current participant access even when the result was generated earlier.
// A nil projection means the durable request exists but no AI state has arrived yet.
func (p *Participant) Read(ctx context.Context, actor Actor, testeeID, assessmentID uint64, requestID string) (*Event, error) {
	request, err := p.ownedRequest(ctx, actor, testeeID, assessmentID, requestID)
	if err != nil {
		return nil, err
	}
	event, err := p.Bridge.Store.Projection(ctx, requestID)
	if err != nil || event == nil {
		return event, err
	}
	if event.RequestID != requestID || event.Actor != actor || event.TesteeID != request.TesteeID {
		return nil, ErrConflict
	}
	if _, err := ValidateArtifact(*event); err != nil {
		return nil, err
	}
	return event, nil
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
		if !requestMatches(previous, actor, testeeID, assessmentID, requestID) || len(previous.Evidence) != 1 || previous.Evidence[0].ReportID != strconv.FormatUint(reportID, 10) {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	if reader, ok := p.Bridge.Store.(PlannedStartReader); ok {
		planned, found, err := reader.ReadPlannedStart(ctx, actor, requestID)
		if err != nil {
			return err
		}
		if found {
			if !requestMatches(planned, actor, testeeID, assessmentID, requestID) || len(planned.Evidence) != 1 || planned.Evidence[0].ReportID != strconv.FormatUint(reportID, 10) {
				return ErrConflict
			}
			// The store must read the original SDK outcome, never authorize a
			// repeated executor. Do not refresh the source or rebuild its body.
			return p.Bridge.Start(ctx, *planned)
		}
	}
	if p.IntakeClosed {
		return ErrManagementUnavailable
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

func (p *Participant) ownedRequest(ctx context.Context, actor Actor, testeeID, assessmentID uint64, requestID string) (*Start, error) {
	if err := p.authorizeRequestLookup(ctx, actor, testeeID, assessmentID, requestID); err != nil {
		return nil, err
	}
	request, err := p.Bridge.Store.Original(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if !requestMatches(request, actor, testeeID, assessmentID, requestID) {
		return nil, ErrNotFound
	}
	return request, nil
}

func (p *Participant) authorizeRequestLookup(ctx context.Context, actor Actor, testeeID, assessmentID uint64, requestID string) error {
	if p == nil || !validID(requestID) || !validNumber(actor.OrgID) || actor.SubjectID == "" || len(actor.SubjectID) > 128 || testeeID == 0 || assessmentID == 0 || p.Access == nil || p.Bridge == nil || p.Bridge.Store == nil {
		return ErrInvalid
	}
	return p.Access.AuthorizeOwnAssessment(ctx, testeeID, assessmentID)
}

func requestMatches(request *Start, actor Actor, testeeID, assessmentID uint64, requestID string) bool {
	return request != nil && request.RequestID == requestID && request.Actor == actor && request.TesteeID == strconv.FormatUint(testeeID, 10) && len(request.AssessmentIDs) == 1 && request.AssessmentIDs[0] == strconv.FormatUint(assessmentID, 10)
}

// ReadOperation checks current access and the original frozen request before any
// operation lookup. Another request owned by the same actor is still inaccessible.
func (p *Participant) ReadOperation(ctx context.Context, actor Actor, testeeID, assessmentID uint64, requestID, commandID string) (MessagingOperation, error) {
	if !validID(commandID) {
		return MessagingOperation{}, ErrInvalid
	}
	if err := p.authorizeRequestLookup(ctx, actor, testeeID, assessmentID, requestID); err != nil {
		return MessagingOperation{}, err
	}
	request, err := p.Bridge.Store.Original(ctx, requestID)
	if errors.Is(err, ErrNotFound) {
		planned, ok := p.Bridge.Store.(PlannedStartReader)
		if !ok {
			return MessagingOperation{}, err
		}
		var found bool
		request, found, err = planned.ReadPlannedStart(ctx, actor, requestID)
		if err == nil && !found {
			err = ErrNotFound
		}
	}
	if err != nil {
		return MessagingOperation{}, err
	}
	if !requestMatches(request, actor, testeeID, assessmentID, requestID) {
		return MessagingOperation{}, ErrNotFound
	}
	reader, ok := p.Bridge.Store.(interface {
		ReadRequestOperation(context.Context, OperationScope, string, string) (MessagingOperation, error)
	})
	if !ok {
		return MessagingOperation{}, ErrManagementUnavailable
	}
	return reader.ReadRequestOperation(ctx, OperationScope{OrganizationID: actor.OrgID, SubjectID: actor.SubjectID}, requestID, commandID)
}
func (p *Participant) MessagingEnabled() bool {
	if p == nil || p.Bridge == nil {
		return false
	}
	_, ok := p.Bridge.Store.(interface {
		ReadRequestOperation(context.Context, OperationScope, string, string) (MessagingOperation, error)
	})
	return ok
}
