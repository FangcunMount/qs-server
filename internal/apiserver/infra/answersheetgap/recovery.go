package answersheetgap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	mongoanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

var ErrUnsafeRecoverySource = errors.New("original answersheet authority is unavailable, changed or already accepted")

// RecoveryPlan contains review identifiers only, never answers or the wire.
// It is not authority to reset Outbox or create a new evaluation attempt.
type RecoveryPlan struct {
	AnswerSheetID     string `json:"answer_sheet_id"`
	EventID           string `json:"event_id"`
	OrgID             uint64 `json:"org_id"`
	SourceFingerprint string `json:"source_fingerprint"`
	original          message.Message
}

func (p RecoveryPlan) RecoveryEventID() string  { return p.EventID }
func (p RecoveryPlan) Message() message.Message { return p.original }

type recoveryConfirmation struct {
	Version              uint64     `bson:"version"`
	Attempts             uint64     `bson:"attempt_count"`
	TransportConfirmedAt *time.Time `bson:"transport_confirmed_at"`
}

// CaptureOriginalRecovery reads one original primary Mongo snapshot and checks
// MySQL for ANY Assessment, including soft-deleted ones. The stores are not a
// shared transaction: callers must capture again after durable authorization;
// a racing normal consumer is still governed by the original Intake unique key.
func (s *Scanner) CaptureOriginalRecovery(ctx context.Context, sheetID, orgID uint64, acceptedBefore, now time.Time) (RecoveryPlan, error) {
	if s == nil || s.mongo == nil || s.mysql == nil || sheetID == 0 || sheetID > math.MaxInt64 || orgID == 0 || orgID > math.MaxInt64 || acceptedBefore.IsZero() || now.IsZero() || acceptedBefore.After(now) {
		return RecoveryPlan{}, ErrUnsafeRecoverySource
	}
	session, err := s.mongo.Client().StartSession()
	if err != nil {
		return RecoveryPlan{}, err
	}
	defer session.EndSession(context.Background())
	if err := session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())); err != nil {
		return RecoveryPlan{}, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = session.AbortTransaction(cleanup)
	}()
	sc := mongo.NewSessionContext(ctx, session)
	filter := bson.M{"durable_acceptance.schema_version": 1, "deleted_at": nil, "domain_id": sheetID}
	projection := bson.M{"_id": 0, "domain_id": 1, "org_id": 1, "testee_id": 1, "filler_id": 1, "filler_type": 1, "questionnaire_code": 1, "questionnaire_version": 1, "task_id": 1, "admission": 1, "attribution": 1, "start_context": 1, "durable_acceptance": 1, "filled_at": 1}
	raw, err := s.mongo.Collection("answersheets").FindOne(sc, filter, options.FindOne().SetHint("idx_answersheet_durable_audit").SetProjection(projection).SetMaxTime(5*time.Second)).Raw()
	if err != nil {
		return RecoveryPlan{}, err
	}
	var sheet sheetRow
	if err := bson.Unmarshal(raw, &sheet); err != nil {
		return RecoveryPlan{}, err
	}
	if sheet.DomainID != sheetID || sheet.OrgID != orgID || sheet.DurableAcceptance.EventID == "" {
		return RecoveryPlan{}, ErrUnsafeRecoverySource
	}
	identity := bson.D{{Key: "producer", Value: producer}, {Key: "message_id", Value: sheet.DurableAcceptance.EventID}, {Key: "destination", Value: destination}}
	intentRaw, err := s.mongo.Collection("rm_outbox").FindOne(sc, bson.M{"_id": identity}, options.FindOne().SetHint("_id_").SetMaxTime(5*time.Second)).Raw()
	if err != nil {
		return RecoveryPlan{}, err
	}
	var intent outboxRow
	var confirmation recoveryConfirmation
	if err := bson.Unmarshal(intentRaw, &intent); err != nil {
		return RecoveryPlan{}, err
	}
	if err := bson.Unmarshal(intentRaw, &confirmation); err != nil {
		return RecoveryPlan{}, err
	}
	var effects []struct{ ID uint64 }
	// No deleted_at filter: even a deleted effect is proof of prior acceptance.
	if err := s.mysql.WithContext(ctx).Table("assessment").Select("id").Where("answer_sheet_id = ?", sheetID).Limit(1).Find(&effects).Error; err != nil {
		return RecoveryPlan{}, err
	}
	if len(effects) != 0 {
		return RecoveryPlan{}, ErrUnsafeRecoverySource
	}
	return validateRecovery(raw, sheet, intent, confirmation, orgID, acceptedBefore, now)
}

func validateRecovery(raw bson.Raw, sheet sheetRow, row outboxRow, confirmation recoveryConfirmation, orgID uint64, acceptedBefore, now time.Time) (RecoveryPlan, error) {
	if sheet.OrgID != orgID || orgID == 0 || sheet.DomainID == 0 || sheet.Admission == nil || sheet.Admission.Purpose != string(eventpayload.AdmissionPurposeAssessment) || sheet.Admission.ModelCode == "" || sheet.Admission.ModelVersion == "" || sheet.TesteeID == 0 || sheet.FillerID <= 0 || sheet.DurableAcceptance.AcceptedAt.IsZero() || sheet.DurableAcceptance.AcceptedAt.After(acceptedBefore) || acceptedBefore.After(now) || row.State != "published" || row.SchemaVersion != "v1" || row.ContentType != "application/json" || confirmation.Version == 0 || confirmation.Attempts == 0 || confirmation.TransportConfirmedAt == nil || confirmation.TransportConfirmedAt.IsZero() || confirmation.TransportConfirmedAt.After(now) {
		return RecoveryPlan{}, ErrUnsafeRecoverySource
	}
	if err := validateOriginal(sheet, row); err != nil {
		return RecoveryPlan{}, ErrUnsafeRecoverySource
	}
	var frozen struct {
		Admission         *mongoanswersheet.AdmissionPO         `bson:"admission"`
		DurableAcceptance *mongoanswersheet.DurableAcceptancePO `bson:"durable_acceptance"`
		FilledAt          time.Time                             `bson:"filled_at"`
		FillerType        string                                `bson:"filler_type"`
	}
	if err := bson.Unmarshal(raw, &frozen); err != nil || frozen.Admission == nil || frozen.DurableAcceptance == nil || frozen.DurableAcceptance.SchemaVersion != 1 {
		return RecoveryPlan{}, ErrUnsafeRecoverySource
	}
	wire, _, err := legacy.Decode(row.Payload)
	if err != nil {
		return RecoveryPlan{}, ErrUnsafeRecoverySource
	}
	env, err := domain.DecodeEnvelope(wire.Payload)
	if err != nil {
		return RecoveryPlan{}, ErrUnsafeRecoverySource
	}
	var data eventpayload.AnswerSheetSubmittedData
	if err := json.Unmarshal(env.Data, &data); err != nil || data.Admission == nil {
		return RecoveryPlan{}, ErrUnsafeRecoverySource
	}
	a, eventAdmission := frozen.Admission, data.Admission
	if a.QuestionnaireCode != sheet.QuestionnaireCode || a.QuestionnaireVersion != sheet.QuestionnaireVer || a.QuestionnaireCode != eventAdmission.QuestionnaireCode || a.QuestionnaireVersion != eventAdmission.QuestionnaireVersion || a.ModelTitle != eventAdmission.ModelTitle || frozen.FillerType == "" || frozen.FillerType != data.FillerType || frozen.FilledAt.IsZero() || !frozen.FilledAt.Equal(data.SubmittedAt.Truncate(time.Millisecond)) {
		return RecoveryPlan{}, ErrUnsafeRecoverySource
	}
	original, err := message.New(message.Input{Producer: row.Producer, ID: row.MessageID, Destination: row.Destination, EventType: row.EventType, SchemaVersion: row.SchemaVersion, Scope: row.Scope, ContentType: row.ContentType, OccurredAt: row.OccurredAt, Payload: row.Payload})
	if err != nil {
		return RecoveryPlan{}, ErrUnsafeRecoverySource
	}
	fingerprint := original.Fingerprint()
	if !bytes.Equal(fingerprint[:], row.Fingerprint) {
		return RecoveryPlan{}, ErrUnsafeRecoverySource
	}
	sourceHash := sha256.Sum256(raw)
	review, err := json.Marshal(struct {
		Source       [32]byte
		Message      [32]byte
		Confirmation recoveryConfirmation
	}{sourceHash, fingerprint, confirmation})
	if err != nil {
		return RecoveryPlan{}, err
	}
	reviewHash := sha256.Sum256(review)
	return RecoveryPlan{AnswerSheetID: strconv.FormatUint(sheet.DomainID, 10), EventID: row.MessageID, OrgID: orgID, SourceFingerprint: hex.EncodeToString(reviewHash[:]), original: original}, nil
}
