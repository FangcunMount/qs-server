// Package answersheetgap audits the host-owned AnswerSheet to Assessment effect.
// It cannot publish events, create Assessments, or change Outbox state.
package answersheetgap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/gorm"
)

const (
	producer    = "qs-server"
	destination = "qs.evaluation.lifecycle"
	eventType   = "answersheet.submitted"
)

type Disposition string

const (
	Present         Disposition = "assessment_present"
	NotRequired     Disposition = "not_required"
	Missing         Disposition = "missing_confirmed"
	DeliveryPending Disposition = "delivery_pending"
	Unknown         Disposition = "unknown"
	ManualRequired  Disposition = "manual_required"
)

type Finding struct {
	AnswerSheetID uint64      `json:"answer_sheet_id"`
	EventID       string      `json:"event_id"`
	Disposition   Disposition `json:"disposition"`
	Reason        string      `json:"reason,omitempty"`
	AssessmentID  uint64      `json:"assessment_id,omitempty"`
}

type Page struct {
	Findings  []Finding
	NextID    uint64
	Exhausted bool
}

// Scanner performs bounded, read-only cross-store checks. A published Outbox
// state is never treated as proof that any consumer has completed its effect.
type Scanner struct {
	mongo *mongo.Database
	mysql *gorm.DB
}

func New(mongoDB *mongo.Database, mysqlDB *gorm.DB) (*Scanner, error) {
	if mongoDB == nil || mysqlDB == nil {
		return nil, fmt.Errorf("answersheet gap scanner requires Mongo and MySQL")
	}
	return &Scanner{mongo: mongoDB, mysql: mysqlDB}, nil
}

type sheetRow struct {
	DomainID          uint64 `bson:"domain_id"`
	OrgID             uint64 `bson:"org_id"`
	TesteeID          uint64 `bson:"testee_id"`
	FillerID          int64  `bson:"filler_id"`
	QuestionnaireCode string `bson:"questionnaire_code"`
	QuestionnaireVer  string `bson:"questionnaire_version"`
	TaskID            string `bson:"task_id"`
	Admission         *struct {
		Purpose        string `bson:"purpose"`
		ModelKind      string `bson:"model_kind"`
		ModelCode      string `bson:"model_code"`
		ModelVersion   string `bson:"model_version"`
		ModelAlgorithm string `bson:"model_algorithm"`
	} `bson:"admission"`
	DurableAcceptance struct {
		EventID    string    `bson:"event_id"`
		AcceptedAt time.Time `bson:"accepted_at"`
	} `bson:"durable_acceptance"`
}

type outboxRow struct {
	Producer      string `bson:"producer"`
	MessageID     string `bson:"message_id"`
	Destination   string `bson:"destination"`
	EventType     string `bson:"event_type"`
	SchemaVersion string `bson:"schema_version"`
	Scope         string `bson:"scope"`
	ContentType   string `bson:"content_type"`
	OccurredAt    string `bson:"occurred_at"`
	Payload       []byte `bson:"payload"`
	Fingerprint   []byte `bson:"fingerprint"`
	State         string `bson:"state"`
}

// ScanPage fixes both an ID upper bound and an acceptance cutoff for each
// call. The caller persists the cursor and chooses a grace period; this
// scanner does not run a scheduler or repair business state.
func (s *Scanner) ScanPage(ctx context.Context, afterID, upperID uint64, acceptedBefore time.Time, limit int) (Page, error) {
	if s == nil || s.mongo == nil || s.mysql == nil || upperID == 0 || upperID < afterID || acceptedBefore.IsZero() || limit < 1 || limit > 500 {
		return Page{}, fmt.Errorf("invalid answersheet gap scan request")
	}
	filter := bson.M{
		"durable_acceptance.schema_version": 1,
		"durable_acceptance.accepted_at":    bson.M{"$lte": acceptedBefore.UTC()},
		"deleted_at":                        nil,
		"domain_id":                         bson.M{"$gt": afterID, "$lte": upperID},
	}
	cur, err := s.mongo.Collection("answersheets").Find(ctx, filter, options.Find().SetHint("idx_answersheet_durable_audit").SetSort(bson.D{{Key: "domain_id", Value: 1}}).SetLimit(int64(limit)).SetProjection(bson.M{
		"domain_id": 1, "org_id": 1, "testee_id": 1, "filler_id": 1,
		"questionnaire_code": 1, "questionnaire_version": 1, "task_id": 1,
		"admission": 1, "durable_acceptance": 1,
	}))
	if err != nil {
		return Page{}, fmt.Errorf("scan accepted answersheets: %w", err)
	}
	defer cur.Close(ctx)
	var sheets []sheetRow
	if err := cur.All(ctx, &sheets); err != nil {
		return Page{}, fmt.Errorf("decode accepted answersheets: %w", err)
	}
	page := Page{Findings: make([]Finding, 0, len(sheets)), Exhausted: len(sheets) < limit}
	if len(sheets) == 0 {
		return page, nil
	}
	ids := make([]uint64, 0, len(sheets))
	for _, sheet := range sheets {
		page.NextID = sheet.DomainID
		ids = append(ids, sheet.DomainID)
	}
	if page.NextID >= upperID {
		page.Exhausted = true
	}
	var assessments []struct {
		ID            uint64 `gorm:"column:id"`
		OrgID         uint64 `gorm:"column:org_id"`
		AnswerSheetID uint64 `gorm:"column:answer_sheet_id"`
	}
	if err := s.mysql.WithContext(ctx).Table("assessment").Select("id,org_id,answer_sheet_id").Where("answer_sheet_id IN ? AND deleted_at IS NULL", ids).Find(&assessments).Error; err != nil {
		for _, sheet := range sheets {
			page.Findings = append(page.Findings, Finding{AnswerSheetID: sheet.DomainID, EventID: sheet.DurableAcceptance.EventID, Disposition: Unknown, Reason: "assessment_query_failed"})
		}
		return page, fmt.Errorf("query assessment effects: %w", err)
	}
	bySheet := make(map[uint64][]struct{ id, orgID uint64 }, len(assessments))
	for _, a := range assessments {
		bySheet[a.AnswerSheetID] = append(bySheet[a.AnswerSheetID], struct{ id, orgID uint64 }{a.ID, a.OrgID})
	}
	for _, sheet := range sheets {
		finding := s.classify(ctx, sheet, bySheet[sheet.DomainID])
		page.Findings = append(page.Findings, finding)
	}
	return page, nil
}

func (s *Scanner) classify(ctx context.Context, sheet sheetRow, assessments []struct{ id, orgID uint64 }) Finding {
	f := Finding{AnswerSheetID: sheet.DomainID, EventID: sheet.DurableAcceptance.EventID}
	if sheet.DomainID == 0 || sheet.OrgID == 0 || sheet.DurableAcceptance.EventID == "" || sheet.Admission == nil {
		f.Disposition, f.Reason = ManualRequired, "incomplete_frozen_answer_sheet"
		return f
	}
	id := bson.D{{Key: "producer", Value: producer}, {Key: "message_id", Value: f.EventID}, {Key: "destination", Value: destination}}
	var row outboxRow
	if err := s.mongo.Collection("rm_outbox").FindOne(ctx, bson.M{"_id": id}).Decode(&row); err != nil {
		if err == mongo.ErrNoDocuments {
			f.Disposition, f.Reason = ManualRequired, "original_outbox_missing"
		} else {
			f.Disposition, f.Reason = Unknown, "original_outbox_query_failed"
		}
		return f
	}
	if err := validateOriginal(sheet, row); err != nil {
		f.Disposition, f.Reason = ManualRequired, "original_identity_or_payload_conflict"
		return f
	}
	if len(assessments) > 1 || len(assessments) == 1 && assessments[0].orgID != sheet.OrgID {
		f.Disposition, f.Reason = ManualRequired, "assessment_identity_conflict"
		return f
	}
	if sheet.Admission.Purpose == string(payload.AdmissionPurposeIndependentQuestionnaire) {
		if len(assessments) != 0 {
			f.Disposition, f.Reason = ManualRequired, "independent_questionnaire_has_assessment"
			return f
		}
		f.Disposition = NotRequired
		return f
	}
	if sheet.Admission.Purpose != string(payload.AdmissionPurposeAssessment) {
		f.Disposition, f.Reason = ManualRequired, "unknown_frozen_admission"
		return f
	}
	if len(assessments) == 1 {
		f.Disposition, f.AssessmentID = Present, assessments[0].id
		return f
	}
	switch row.State {
	case "published":
		f.Disposition = Missing
	case "pending", "publishing", "retry_wait":
		f.Disposition, f.Reason = DeliveryPending, "relay_not_confirmed"
	default:
		f.Disposition, f.Reason = ManualRequired, "outbox_state_requires_governance"
	}
	return f
}

func validateOriginal(sheet sheetRow, row outboxRow) error {
	if row.Producer != producer || row.MessageID != sheet.DurableAcceptance.EventID || row.Destination != destination || row.EventType != eventType || row.Scope != fmt.Sprintf("org:%d", sheet.OrgID) || row.State == "" {
		return fmt.Errorf("outbox identity differs from accepted answer sheet")
	}
	m, err := message.New(message.Input{Producer: row.Producer, ID: row.MessageID, Destination: row.Destination, EventType: row.EventType, SchemaVersion: row.SchemaVersion, Scope: row.Scope, ContentType: row.ContentType, OccurredAt: row.OccurredAt, Payload: row.Payload})
	if err != nil {
		return err
	}
	hash := m.Fingerprint()
	if !bytes.Equal(hash[:], row.Fingerprint) {
		return fmt.Errorf("immutable outbox fingerprint mismatch")
	}
	wire, recognized, err := legacy.Decode(row.Payload)
	if err != nil || !recognized {
		return fmt.Errorf("original message wire is invalid")
	}
	env, err := domain.DecodeEnvelope(wire.Payload)
	if err != nil {
		return err
	}
	if wire.UUID != row.MessageID || env.ID != row.MessageID || env.EventType != eventType || env.AggregateType != "AnswerSheet" || env.AggregateID != strconv.FormatUint(sheet.DomainID, 10) || wire.Metadata["event_type"] != eventType {
		return fmt.Errorf("original event identity mismatch")
	}
	var data payload.AnswerSheetSubmittedData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return err
	}
	if data.AnswerSheetID != env.AggregateID || data.OrgID != sheet.OrgID || data.TesteeID != sheet.TesteeID || data.FillerID != uint64(sheet.FillerID) || sheet.FillerID <= 0 || data.QuestionnaireCode != sheet.QuestionnaireCode || data.QuestionnaireVersion != sheet.QuestionnaireVer || data.TaskID != sheet.TaskID || data.Admission == nil || string(data.Admission.Purpose) != sheet.Admission.Purpose || data.Admission.ModelKind != sheet.Admission.ModelKind || data.Admission.ModelCode != sheet.Admission.ModelCode || data.Admission.ModelVersion != sheet.Admission.ModelVersion || data.Admission.ModelAlgorithm != sheet.Admission.ModelAlgorithm {
		return fmt.Errorf("original event facts or frozen admission mismatch")
	}
	return nil
}
