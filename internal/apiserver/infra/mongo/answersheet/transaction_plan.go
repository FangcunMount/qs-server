package answersheet

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventmessage"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	submit "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"go.mongodb.org/mongo-driver/bson"
)

const submissionOwner = "qs.mongo"
const submissionRouteRevision = "m3-1"
const submissionBranch = "answersheet.submitted"
const maxFrozenSubmissionBytes = 1 << 20

// Only the private persisted plan contains the BSON artifact. Branch.Wire is
// still the exact original consumer envelope. Its independent hash and the
// artifact-bearing message fingerprint are both bound by the SDK plan.
type submissionArtifact struct {
	Version  int    `json:"version"`
	Document []byte `json:"document"`
}

func submissionKey(writer uint64, key, operation string) string {
	if key == "" {
		return fmt.Sprintf("answersheet-submit/%d/operation/%s", writer, operation)
	}
	return fmt.Sprintf("answersheet-submit/%d/key/%d:%s", writer, len(key), key)
}

func freezeSubmission(ctx context.Context, sheet *domain.AnswerSheet, meta submit.DurableSubmitMeta, topic string, routes *transport.RouteResolver) (transaction.FrozenPlan, error) {
	if sheet == nil || sheet.ID().IsZero() || meta.WriterID == 0 || topic == "" {
		return transaction.FrozenPlan{}, fmt.Errorf("complete submission identity required")
	}
	if err := sheet.SubmissionContext().Validate(); err != nil {
		return transaction.FrozenPlan{}, err
	}
	fingerprint, err := submit.Fingerprint(sheet)
	if err != nil || meta.Fingerprint != fingerprint || sheet.Filler().UserID() <= 0 || uint64(sheet.Filler().UserID()) != meta.WriterID {
		return transaction.FrozenPlan{}, fmt.Errorf("submission input or writer identity differs")
	}
	events := sheet.Events()
	if len(events) != 1 {
		return transaction.FrozenPlan{}, fmt.Errorf("one original submitted event required")
	}
	var evt domain.AnswerSheetSubmittedEvent
	switch v := events[0].(type) {
	case domain.AnswerSheetSubmittedEvent:
		evt = v
	case *domain.AnswerSheetSubmittedEvent:
		if v == nil {
			return transaction.FrozenPlan{}, fmt.Errorf("nil submitted event")
		}
		evt = *v
	default:
		return transaction.FrozenPlan{}, fmt.Errorf("submitted event required")
	}
	if evt.Data.RequestID != "" && meta.RequestID != "" && evt.Data.RequestID != meta.RequestID {
		return transaction.FrozenPlan{}, fmt.Errorf("submission request identity differs")
	}
	if meta.RequestID != "" {
		evt.Data.RequestID = meta.RequestID
	}
	wire, err := eventmessage.EncodeWire(evt, "api-server")
	if err != nil {
		return transaction.FrozenPlan{}, err
	}
	if len(wire) > 262144 {
		return transaction.FrozenPlan{}, fmt.Errorf("submission wire exceeds transaction limit")
	}
	po := NewAnswerSheetMapper().ToPO(sheet)
	base.ApplyAuditCreate(ctx, po)
	// All generated BSON identity and audit clocks are outside WithTransaction.
	po.BeforeInsert()
	po.DurableAcceptance = &DurableAcceptancePO{SchemaVersion: 2, EventID: evt.EventID(), AcceptedAt: evt.OccurredAt().UTC(), RequestID: evt.Data.RequestID}
	if meta.IdempotencyKey != "" {
		po.SubmitMeta = &SubmitMetaPO{WriterID: meta.WriterID, IdempotencyKey: meta.IdempotencyKey, Fingerprint: fingerprint, RequestID: evt.Data.RequestID, AcceptedAt: evt.OccurredAt().UTC()}
	}
	raw, err := bson.Marshal(po)
	if err != nil {
		return transaction.FrozenPlan{}, err
	}
	payload, err := json.Marshal(submissionArtifact{Version: 1, Document: raw})
	if err != nil || len(payload) > maxFrozenSubmissionBytes {
		return transaction.FrozenPlan{}, fmt.Errorf("invalid frozen submission artifact")
	}
	msg, err := message.New(message.Input{Producer: "qs-server", ID: evt.EventID(), Destination: topic, EventType: domain.EventTypeSubmitted,
		SchemaVersion: "v1", Scope: fmt.Sprintf("org:%d", po.OrgID), ContentType: "application/json", OccurredAt: evt.OccurredAt().UTC().Format(time.RFC3339Nano), Payload: payload})
	if err != nil {
		return transaction.FrozenPlan{}, err
	}
	row, err := routes.PublishRoute(transport.RouteKey{Topic: topic, MessageType: transport.TransactionMessageType, Owner: submissionOwner, RouteRevision: submissionRouteRevision})
	if err != nil {
		return transaction.FrozenPlan{}, err
	}
	var hash [32]byte
	decoded, _ := hex.DecodeString(fingerprint)
	copy(hash[:], decoded)
	plan, err := transaction.NewPlan(transaction.PlanInput{Owner: submissionOwner, Scope: msg.Input().Scope, OperationID: evt.EventID(),
		IdempotencyKey: submissionKey(meta.WriterID, meta.IdempotencyKey, evt.EventID()), InputHash: hash,
		Branches: []transaction.BranchInput{{ID: submissionBranch, PhysicalTopic: row.PhysicalTopic, RouteRevision: submissionRouteRevision, Message: msg, Wire: wire}}})
	if err != nil {
		return transaction.FrozenPlan{}, err
	}
	_, _, err = readSubmissionPlan(plan, routes)
	return plan, err
}

func readSubmissionPlan(plan transaction.FrozenPlan, routes *transport.RouteResolver) (*AnswerSheetPO, string, error) {
	in := plan.Input()
	if !plan.Valid() || in.Owner != submissionOwner || len(in.Branches) != 1 {
		return nil, "", transaction.ErrInvalidPlan
	}
	b := in.Branches[0]
	m := b.Message.Input()
	row, err := routes.RecordedRoute(transport.RouteKey{Topic: m.Destination, MessageType: transport.TransactionMessageType, Owner: submissionOwner, RouteRevision: submissionRouteRevision}, b.PhysicalTopic)
	if err != nil || b.ID != submissionBranch || b.RouteRevision != submissionRouteRevision || b.PhysicalTopic != row.PhysicalTopic || in.OperationID != m.ID || m.EventType != domain.EventTypeSubmitted || m.Producer != "qs-server" || len(m.Payload) > maxFrozenSubmissionBytes {
		return nil, "", transaction.ErrInvalidPlan
	}
	var artifact submissionArtifact
	if err := json.Unmarshal(m.Payload, &artifact); err != nil || artifact.Version != 1 || bson.Raw(artifact.Document).Validate() != nil {
		return nil, "", transaction.ErrInvalidPlan
	}
	canonical, err := json.Marshal(artifact)
	if err != nil || !bytes.Equal(canonical, m.Payload) {
		return nil, "", transaction.ErrInvalidPlan
	}
	var po AnswerSheetPO
	if err := bson.Unmarshal(artifact.Document, &po); err != nil {
		return nil, "", err
	}
	a := po.DurableAcceptance
	if a == nil || a.SchemaVersion != 2 || a.EventEvidence != nil || a.TransactionEvidence != nil || a.EventID != m.ID || a.AcceptedAt.IsZero() || po.ID.IsZero() || po.DomainID.IsZero() || po.CreatedAt.IsZero() || po.FillerID <= 0 || in.Scope != fmt.Sprintf("org:%d", po.OrgID) {
		return nil, "", transaction.ErrInvalidPlan
	}
	sheet := NewAnswerSheetMapper().ToBO(&po)
	if sheet == nil {
		return nil, "", transaction.ErrInvalidPlan
	}
	fingerprint, err := submit.Fingerprint(sheet)
	if err != nil || fingerprint != hex.EncodeToString(in.InputHash[:]) {
		return nil, "", transaction.ErrInputConflict
	}
	key := ""
	if v := po.SubmitMeta; v != nil {
		key = v.IdempotencyKey
		if key == "" || v.WriterID != uint64(po.FillerID) || v.Fingerprint != fingerprint || v.RequestID != a.RequestID || !v.AcceptedAt.Equal(a.AcceptedAt) {
			return nil, "", transaction.ErrInvalidPlan
		}
	}
	if in.IdempotencyKey != submissionKey(uint64(po.FillerID), key, in.OperationID) {
		return nil, "", transaction.ErrRequestIdentity
	}
	// The private artifact is not a consumer envelope. Check the independently
	// frozen wire using its original message fingerprint, then bind its typed
	// business projection to the BSON fact.
	m.Payload = b.Wire
	original, err := message.New(m)
	if err != nil {
		return nil, "", err
	}
	ref := eventmessage.ReferenceFromMessage(original)
	envelope, err := eventmessage.VerifyReference(m, ref.Fingerprint, ref)
	if err != nil {
		return nil, "", err
	}
	if envelope.AggregateType != domain.AggregateType || envelope.AggregateID != po.DomainID.String() || !envelope.OccurredAt.UTC().Truncate(time.Millisecond).Equal(a.AcceptedAt) {
		return nil, "", transaction.ErrInvalidPlan
	}
	var payload domain.AnswerSheetSubmittedData
	if err := json.Unmarshal(envelope.Data, &payload); err != nil {
		return nil, "", err
	}
	binding, err := eventevidencebinding.AnswerSheet(payload)
	if err != nil {
		return nil, "", err
	}
	actual, err := submissionBinding(po)
	if err != nil || actual != binding {
		return nil, "", fmt.Errorf("frozen submission business binding differs")
	}
	return &po, binding, nil
}

func materializeSubmission(e transaction.Execution, routes *transport.RouteResolver) (bson.Raw, *domain.AnswerSheet, error) {
	po, binding, err := readSubmissionPlan(e.Plan, routes)
	if err != nil {
		return nil, nil, err
	}
	refs := e.References()
	if len(refs) != 1 {
		return nil, nil, transaction.ErrInvalidPlan
	}
	proof, err := evidence.NewTransactional(eventmessage.ReferenceFromMessage(e.Plan.Input().Branches[0].Message), refs[0], binding, po.DurableAcceptance.AcceptedAt)
	if err != nil {
		return nil, nil, err
	}
	po.DurableAcceptance.TransactionEvidence = proof
	raw, err := bson.Marshal(po)
	return raw, NewAnswerSheetMapper().ToBO(po), err
}

func submissionBinding(po AnswerSheetPO) (string, error) {
	if po.DurableAcceptance == nil || po.FillerID <= 0 {
		return "", transaction.ErrInvalidPlan
	}
	p := domain.AnswerSheetSubmittedData{AnswerSheetID: po.DomainID.String(), OrgID: po.OrgID, TesteeID: po.TesteeID, FillerID: uint64(po.FillerID), FillerType: po.FillerType,
		QuestionnaireCode: po.QuestionnaireCode, QuestionnaireVersion: po.QuestionnaireVersion, TaskID: po.TaskID, SubmittedAt: po.FilledAt, RequestID: po.DurableAcceptance.RequestID}
	// Reuse domain projection of immutable admission/attribution without
	// constructing a new event (which would allocate a new UUID and clock).
	if v := admissionFromPO(po.Admission); !v.IsZero() {
		p.Admission = v.ToEventPayload()
	}
	if v := po.Attribution; v != nil {
		p.Attribution = &eventpayload.AttributionSnapshot{OriginType: v.OriginType, OriginID: v.OriginID, ClinicianID: v.ClinicianID, EntryID: v.EntryID, PlanID: v.PlanID, EnrollmentID: v.EnrollmentID, TaskID: v.TaskID, CapturedAt: v.CapturedAt, Version: v.Version, Mode: v.Mode}
	}
	return eventevidencebinding.AnswerSheet(p)
}
