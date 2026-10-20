// Package evaluationsubmit binds evaluation.requested to the original pending
// Assessment CAS and the shared qs.mysql transaction owner.
package evaluationsubmit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	boundary "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationtransaction"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"gorm.io/gorm"
)

type Submitter struct {
	boundary port.Committer
	repo     assessment.Repository
	mapper   *repository.AssessmentMapper
}

var _ evaluationtransaction.Submitter = (*Submitter)(nil)

// New borrows the already constructed shared owner and its exact primary.
// It creates neither a second producer nor schema/network/background work.
func New(db *gorm.DB, owner *boundary.Committer) (*Submitter, error) {
	if owner == nil || !owner.UsesPrimary(db) {
		return nil, transaction.ErrBinding
	}
	return &Submitter{boundary: owner, repo: repository.NewAssessmentRepository(db), mapper: repository.NewAssessmentMapper()}, nil
}

type artifact struct {
	Version    int                      `json:"version"`
	Assessment *repository.AssessmentPO `json:"assessment"`
}

func key(id string) string { return "evaluation-submit/" + id }

func inputHash(po *repository.AssessmentPO) [32]byte {
	// A SQL submission clock/event ID is generated only once by the winner.
	// Immutable admission and model fields form stable business input; later
	// scoring/failure projections are not part of this operation's identity.
	input := struct {
		ID, AnswerSheetID, TesteeID                                                                        uint64
		OrgID                                                                                              int64
		QuestionnaireCode, QuestionnaireVersion, OriginType                                                string
		OriginID, Conducting, ModelKind, ModelSubKind, ModelAlgorithm, ModelCode, ModelVersion, ModelTitle *string
		CreatedAt                                                                                          time.Time
	}{ID: po.ID.Uint64(), AnswerSheetID: po.AnswerSheetID, TesteeID: po.TesteeID, OrgID: po.OrgID,
		QuestionnaireCode: po.QuestionnaireCode, QuestionnaireVersion: po.QuestionnaireVersion, OriginType: po.OriginType,
		OriginID: po.OriginID, Conducting: po.ConductingContext, ModelKind: po.EvaluationModelKind, ModelSubKind: po.EvaluationModelSubKind, ModelAlgorithm: po.EvaluationModelAlgorithm, ModelCode: po.EvaluationModelCode, ModelVersion: po.EvaluationModelVersion, ModelTitle: po.EvaluationModelTitle, CreatedAt: po.CreatedAt}
	raw, _ := json.Marshal(input) // Only closed scalar/time fields; no opaque domain values.
	return sha256.Sum256(raw)
}

func (s *Submitter) Submit(ctx context.Context, pending *assessment.Assessment, at time.Time) (committed *assessment.Assessment, submitErr error) {
	defer func() {
		if submitErr != nil {
			submitErr = errors.Join(port.ErrUnresolved, submitErr)
		}
	}()
	if pending == nil || pending.ID().IsZero() || at.IsZero() {
		return nil, transaction.ErrInvalidPlan
	}
	existing, err := s.boundary.Find(ctx, fmt.Sprintf("org:%d", pending.OrgID()), key(pending.ID().String()))
	if err == nil {
		return s.recover(ctx, existing.Execution, pending)
	}
	if !errors.Is(err, transaction.ErrNotFound) {
		return nil, err
	}
	if !pending.Status().IsPending() {
		return nil, fmt.Errorf("assessment submission has no original transaction plan")
	}
	// Reconstruct an isolated copy. SubmitAt creates the event here, before
	// reservation, half preparation or any retryable SQL callback.
	prepared := s.mapper.ToDomain(s.mapper.ToPO(pending))
	if err := prepared.SubmitAt(at.UTC().Truncate(time.Millisecond)); err != nil {
		return nil, err
	}
	po := s.mapper.ToPO(prepared)
	raw, err := json.Marshal(artifact{1, po})
	if err != nil {
		return nil, err
	}
	q := port.Request{Key: key(pending.ID().String()), InputHash: inputHash(po), Artifact: raw, Events: prepared.Events()}
	r, err := s.boundary.Reserve(ctx, q)
	if err != nil {
		return nil, err
	}
	frozen, err := s.read(r.Execution)
	if err != nil {
		return nil, err
	}
	if frozen.ID != po.ID || frozen.OrgID != po.OrgID || inputHash(frozen) != q.InputHash {
		return nil, transaction.ErrInputConflict
	}
	if !r.Acquired {
		return s.recover(ctx, r.Execution, pending)
	}
	winner, err := s.boundary.Artifact(r.Execution.Plan)
	if err != nil || !bytes.Equal(winner, raw) {
		return nil, transaction.ErrInvalidPlan
	}
	writer, ok := s.repo.(assessment.PendingSubmissionRepository)
	if !ok {
		return nil, fmt.Errorf("original pending submission CAS repository required")
	}
	result, err := s.boundary.Commit(ctx, r.Execution, func(txCtx context.Context) error { return writer.SavePendingSubmission(txCtx, prepared) })
	if result.Local != transaction.LocalCommitted {
		if err != nil {
			return nil, err
		}
		return nil, port.ErrUnresolved
	}
	prepared.ClearEvents()
	return prepared, nil
}

func (s *Submitter) read(execution transaction.Execution) (*repository.AssessmentPO, error) {
	raw, err := s.boundary.Artifact(execution.Plan)
	if err != nil {
		return nil, err
	}
	var a artifact
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(a)
	po := a.Assessment
	if err != nil || !bytes.Equal(canonical, raw) || a.Version != 1 || po == nil || po.ID.IsZero() || po.Status != assessment.StatusSubmitted.String() || po.SubmittedAt == nil {
		return nil, transaction.ErrInvalidPlan
	}
	in := execution.Plan.Input()
	if in.Owner != "qs.mysql" || in.Scope != fmt.Sprintf("org:%d", po.OrgID) || in.IdempotencyKey != key(po.ID.String()) || in.InputHash != inputHash(po) || len(in.Branches) != 1 {
		return nil, transaction.ErrInvalidPlan
	}
	b := in.Branches[0]
	consumer := b.Message.Input()
	consumer.Payload = bytes.Clone(b.Wire)
	original, err := message.New(consumer)
	if err != nil {
		return nil, err
	}
	ref := standard.ReferenceFromMessage(original)
	envelope, err := standard.VerifyReference(consumer, ref.Fingerprint, ref)
	if err != nil {
		return nil, err
	}
	var body assessment.EvaluationRequestedData
	if json.Unmarshal(envelope.Data, &body) != nil || consumer.EventType != assessment.EventTypeRequested || envelope.AggregateID != po.ID.String() || body.OrgID != po.OrgID || body.AssessmentID != int64(po.ID.Uint64()) || body.TesteeID != po.TesteeID || body.AnswerSheetID != fmt.Sprint(po.AnswerSheetID) || body.QuestionnaireCode != po.QuestionnaireCode || body.QuestionnaireVer != po.QuestionnaireVersion || !body.RequestedAt.Equal(*po.SubmittedAt) || body.ExpectedAttempt != 0 || body.AttemptOrigin != "" || body.ActionRequestID != "" || body.Mode != "" {
		return nil, transaction.ErrInvalidPlan
	}
	if model := s.mapper.ToDomain(po).EvaluationModelRef(); model == nil || body.ModelKind != model.Kind().String() || body.ModelCode != model.Code().String() || body.ModelVersion != model.Version() || body.ModelAlgorithm != string(model.Algorithm()) {
		return nil, transaction.ErrInvalidPlan
	}
	return po, nil
}

func (s *Submitter) recover(ctx context.Context, e transaction.Execution, pending *assessment.Assessment) (*assessment.Assessment, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	po, err := s.read(e)
	if err != nil {
		return nil, err
	}
	if po.ID != pending.ID() || po.OrgID != pending.OrgID() || s.boundary.Confirm(ctx, e) != transaction.LocalCommitted {
		return nil, port.ErrUnresolved
	}
	stored, err := s.repo.FindByID(ctx, po.ID)
	if err != nil {
		return nil, err
	}
	if stored == nil || stored.Status().IsPending() || stored.SubmittedAt() == nil || !stored.SubmittedAt().Equal(*po.SubmittedAt) || inputHash(s.mapper.ToPO(stored)) != inputHash(po) {
		return nil, port.ErrUnresolved
	}
	stored.ClearEvents()
	return stored, nil
}
