package answersheet

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// RetirementEvidenceStore is a maintenance-only adapter for existing durable
// submissions. It neither opens a connection nor owns a transaction.
type RetirementEvidenceStore struct{ collection *mongo.Collection }

func NewRetirementEvidenceStore(db *mongo.Database) (*RetirementEvidenceStore, error) {
	if db == nil {
		return nil, fmt.Errorf("%w: answersheet database is missing", retirementevidence.ErrUnverifiable)
	}
	return &RetirementEvidenceStore{collection: db.Collection((AnswerSheetPO{}).CollectionName())}, nil
}

// RetirementEvidenceBaseline exposes the exact versioned payload projection to
// a source verifier, while retaining an uneditable selected BSON CAS baseline.
type RetirementEvidenceBaseline struct {
	snapshot         *retirementevidence.Snapshot
	payload          eventpayload.AnswerSheetSubmittedData
	eventID, binding string
}

func (b *RetirementEvidenceBaseline) EventID() string {
	if b == nil {
		return ""
	}
	return b.eventID
}
func (b *RetirementEvidenceBaseline) BindingSHA256() string {
	if b == nil {
		return ""
	}
	return b.binding
}
func (b *RetirementEvidenceBaseline) Payload() eventpayload.AnswerSheetSubmittedData {
	if b == nil {
		return eventpayload.AnswerSheetSubmittedData{}
	}
	out := b.payload
	if out.Admission != nil {
		value := *out.Admission
		out.Admission = &value
	}
	if out.Attribution != nil {
		value := *out.Attribution
		out.Attribution = &value
	}
	return out
}
func (b *RetirementEvidenceBaseline) ValidateEvidence(proof *evidence.EventEvidenceV1) error {
	if b == nil {
		return retirementevidence.ErrUnverifiable
	}
	return retirementevidence.ValidateHistorical(proof, b.eventID, b.binding)
}

// Prepare reads one existing record; it never supplies an absent acceptance
// marker, event ID, request ID or timestamp from a new client request.
func (s *RetirementEvidenceStore) Prepare(ctx context.Context, id uint64) (*RetirementEvidenceBaseline, error) {
	if s == nil || s.collection == nil || id == 0 {
		return nil, retirementevidence.ErrUnverifiable
	}
	snapshot, err := retirementevidence.Read(ctx, s.collection, bson.M{"domain_id": id, "deleted_at": nil}, "durable_acceptance.event_evidence")
	if err != nil {
		return nil, err
	}
	var row AnswerSheetPO
	if err := snapshot.Decode(&row); err != nil {
		return nil, err
	}
	marker := row.DurableAcceptance
	if row.DomainID.Uint64() != id || marker == nil || marker.SchemaVersion != 1 || marker.AcceptedAt.IsZero() || row.FillerID < 0 || row.QuestionnaireCode == "" || row.QuestionnaireVersion == "" || row.FillerType == "" {
		return nil, fmt.Errorf("%w: original submission acceptance or ownership is incomplete", retirementevidence.ErrUnverifiable)
	}
	requestID := marker.RequestID
	if row.SubmitMeta != nil && row.SubmitMeta.RequestID != "" {
		if requestID != "" && requestID != row.SubmitMeta.RequestID {
			return nil, fmt.Errorf("%w: original submission request IDs disagree", retirementevidence.ErrConflict)
		}
		requestID = row.SubmitMeta.RequestID
	}
	payload := eventpayload.AnswerSheetSubmittedData{AnswerSheetID: row.DomainID.String(), OrgID: row.OrgID, TesteeID: row.TesteeID, QuestionnaireCode: row.QuestionnaireCode, QuestionnaireVersion: row.QuestionnaireVersion, FillerID: uint64(row.FillerID), FillerType: row.FillerType, TaskID: row.TaskID, RequestID: requestID, SubmittedAt: row.FilledAt}
	if v := row.Admission; v != nil {
		payload.Admission = &eventpayload.AssessmentAdmission{Purpose: eventpayload.AdmissionPurpose(v.Purpose), QuestionnaireCode: v.QuestionnaireCode, QuestionnaireVersion: v.QuestionnaireVersion, ModelKind: v.ModelKind, ModelAlgorithm: v.ModelAlgorithm, ModelCode: v.ModelCode, ModelVersion: v.ModelVersion, ModelTitle: v.ModelTitle}
	}
	if v := row.Attribution; v != nil {
		payload.Attribution = &eventpayload.AttributionSnapshot{OriginType: v.OriginType, OriginID: v.OriginID, ClinicianID: v.ClinicianID, EntryID: v.EntryID, PlanID: v.PlanID, EnrollmentID: v.EnrollmentID, TaskID: v.TaskID, CapturedAt: v.CapturedAt, Version: v.Version, Mode: v.Mode}
	}
	binding, err := eventevidencebinding.AnswerSheet(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", retirementevidence.ErrUnverifiable, err)
	}
	return &RetirementEvidenceBaseline{snapshot: snapshot, payload: payload, eventID: marker.EventID, binding: binding}, nil
}

// Backfill writes only durable_acceptance.event_evidence and leaves the stored
// event/request IDs, SubmitMeta fingerprint and millisecond clocks untouched.
// The caller starts/retries/commits/aborts the transaction.
func (s *RetirementEvidenceStore) Backfill(ctx context.Context, baseline *RetirementEvidenceBaseline, proof *evidence.EventEvidenceV1) error {
	if s == nil {
		return retirementevidence.ErrUnverifiable
	}
	if err := retirementevidence.RequireTransaction(ctx, s.collection); err != nil {
		return err
	}
	if err := baseline.ValidateEvidence(proof); err != nil {
		return err
	}
	return baseline.snapshot.Apply(ctx, s.collection, proof)
}
