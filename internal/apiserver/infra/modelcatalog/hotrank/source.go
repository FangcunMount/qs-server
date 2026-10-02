package hotrank

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/hotrank"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const DayRebuildIndex = "idx_answersheet_hotrank_day"

// MongoDaySource borrows the host's database. It only reads metadata, never
// answers, and never writes facts, Outbox state, or a new recovery event.
type MongoDaySource struct{ db *mongo.Database }

func NewMongoDaySource(db *mongo.Database) (*MongoDaySource, error) {
	if db == nil {
		return nil, fmt.Errorf("hot rank source requires the host Mongo database")
	}
	return &MongoDaySource{db: db}, nil
}

type sourceSheet struct {
	ID                   uint64    `bson:"domain_id"`
	OrgID                uint64    `bson:"org_id"`
	TesteeID             uint64    `bson:"testee_id"`
	FillerID             int64     `bson:"filler_id"`
	QuestionnaireCode    string    `bson:"questionnaire_code"`
	QuestionnaireVersion string    `bson:"questionnaire_version"`
	FilledAt             time.Time `bson:"filled_at"`
	DurableAcceptance    struct {
		SchemaVersion uint32    `bson:"schema_version"`
		EventID       string    `bson:"event_id"`
		AcceptedAt    time.Time `bson:"accepted_at"`
	} `bson:"durable_acceptance"`
}
type sourceIntent struct {
	Producer      string `bson:"producer"`
	ID            string `bson:"message_id"`
	Destination   string `bson:"destination"`
	EventType     string `bson:"event_type"`
	SchemaVersion string `bson:"schema_version"`
	Scope         string `bson:"scope"`
	ContentType   string `bson:"content_type"`
	OccurredAt    string `bson:"occurred_at"`
	Payload       []byte `bson:"payload"`
	Fingerprint   []byte `bson:"fingerprint"`
}

func (s *MongoDaySource) RequireIndex(ctx context.Context) error {
	cur, err := s.db.Collection("answersheets").Indexes().List(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = cur.Close(ctx) }()
	for cur.Next(ctx) {
		var index struct {
			Name    string `bson:"name"`
			Key     bson.D `bson:"key"`
			Partial bson.M `bson:"partialFilterExpression"`
			Sparse  bool   `bson:"sparse"`
		}
		if err := cur.Decode(&index); err != nil {
			return err
		}
		if index.Name != DayRebuildIndex {
			continue
		}
		if len(index.Key) != 2 || index.Key[0].Key != "filled_at" || index.Key[1].Key != "domain_id" || fmt.Sprint(index.Key[0].Value) != "1" || fmt.Sprint(index.Key[1].Value) != "1" || len(index.Partial) != 0 || index.Sparse {
			return fmt.Errorf("hot rank day index definition differs from complete source contract")
		}
		return nil
	}
	if err := cur.Err(); err != nil {
		return err
	}
	return fmt.Errorf("hot rank day index missing; apply the explicit host migration first")
}

// Capture reads the whole day in one primary snapshot, including soft-deleted
// submissions: the existing rank counts submissions, not currently visible rows.
// An unmarked legacy row, missing original intent or a truncated window rejects
// the entire repair. It does not silently lower the source total.
func (s *MongoDaySource) Capture(ctx context.Context, day string) (RebuildSnapshot, error) {
	start, err := rebuildDay(day, time.Now())
	if err != nil {
		return RebuildSnapshot{}, err
	}
	if err := s.RequireIndex(ctx); err != nil {
		return RebuildSnapshot{}, err
	}
	session, err := s.db.Client().StartSession()
	if err != nil {
		return RebuildSnapshot{}, err
	}
	defer session.EndSession(context.Background())
	if err := session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())); err != nil {
		return RebuildSnapshot{}, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = session.AbortTransaction(cleanup)
	}()
	sc := mongo.NewSessionContext(ctx, session)
	captured := time.Now().UTC()
	cur, err := s.db.Collection("answersheets").Find(sc, bson.M{"filled_at": bson.M{"$gte": start.UTC(), "$lt": start.AddDate(0, 0, 1).UTC()}}, options.Find().SetHint(DayRebuildIndex).SetLimit(MaxRebuildFacts+1).SetMaxTime(5*time.Second).SetSort(bson.D{{Key: "filled_at", Value: 1}, {Key: "domain_id", Value: 1}}).SetProjection(bson.M{"domain_id": 1, "org_id": 1, "testee_id": 1, "filler_id": 1, "questionnaire_code": 1, "questionnaire_version": 1, "filled_at": 1, "durable_acceptance": 1}))
	if err != nil {
		return RebuildSnapshot{}, err
	}
	defer func() { _ = cur.Close(sc) }()
	var sheets []sourceSheet
	if err := cur.All(sc, &sheets); err != nil {
		return RebuildSnapshot{}, err
	}
	if len(sheets) > MaxRebuildFacts {
		return RebuildSnapshot{}, fmt.Errorf("hot rank day exceeds bounded budget; no partial rebuild allowed")
	}
	snapshot := RebuildSnapshot{Day: day, CapturedAt: captured, Facts: make([]port.SubmissionFact, 0, len(sheets)), OriginalFingerprints: map[string]string{}, Complete: true}
	ids := make([]bson.D, 0, len(sheets))
	seen := map[string]bool{}
	for _, sheet := range sheets {
		if sheet.ID == 0 || sheet.OrgID == 0 || sheet.TesteeID == 0 || sheet.FillerID <= 0 || sheet.DurableAcceptance.SchemaVersion != 1 || sheet.DurableAcceptance.EventID == "" || sheet.DurableAcceptance.AcceptedAt.IsZero() || seen[sheet.DurableAcceptance.EventID] {
			return RebuildSnapshot{}, ErrRebuildConflict
		}
		seen[sheet.DurableAcceptance.EventID] = true
		ids = append(ids, bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: sheet.DurableAcceptance.EventID}, {Key: "destination", Value: "qs.evaluation.lifecycle"}})
	}
	if len(ids) == 0 {
		return snapshot, nil
	}
	intents, err := s.db.Collection("rm_outbox").Find(sc, bson.M{"_id": bson.M{"$in": ids}}, options.Find().SetHint("_id_").SetLimit(MaxRebuildFacts+1).SetMaxTime(5*time.Second))
	if err != nil {
		return RebuildSnapshot{}, err
	}
	defer func() { _ = intents.Close(sc) }()
	var rows []sourceIntent
	if err := intents.All(sc, &rows); err != nil {
		return RebuildSnapshot{}, err
	}
	if len(rows) != len(sheets) {
		return RebuildSnapshot{}, fmt.Errorf("original hot rank intent coverage is incomplete")
	}
	byID := map[string]sourceIntent{}
	for _, row := range rows {
		if _, exists := byID[row.ID]; exists {
			return RebuildSnapshot{}, ErrRebuildConflict
		}
		byID[row.ID] = row
	}
	for _, sheet := range sheets {
		row, exists := byID[sheet.DurableAcceptance.EventID]
		if !exists {
			return RebuildSnapshot{}, ErrRebuildConflict
		}
		fact, err := validateHotRankOriginal(sheet, row)
		if err != nil {
			return RebuildSnapshot{}, err
		}
		snapshot.Facts = append(snapshot.Facts, fact)
		snapshot.OriginalFingerprints[fact.EventID] = hex.EncodeToString(row.Fingerprint)
	}
	return snapshot, nil
}

func validateHotRankOriginal(sheet sourceSheet, row sourceIntent) (port.SubmissionFact, error) {
	if row.Producer != "qs-server" || row.ID != sheet.DurableAcceptance.EventID || row.Destination != "qs.evaluation.lifecycle" || row.EventType != "answersheet.submitted" || row.Scope != fmt.Sprintf("org:%d", sheet.OrgID) {
		return port.SubmissionFact{}, ErrRebuildConflict
	}
	original, err := message.New(message.Input{Producer: row.Producer, ID: row.ID, Destination: row.Destination, EventType: row.EventType, SchemaVersion: row.SchemaVersion, Scope: row.Scope, ContentType: row.ContentType, OccurredAt: row.OccurredAt, Payload: row.Payload})
	if err != nil {
		return port.SubmissionFact{}, err
	}
	hash := original.Fingerprint()
	if !bytes.Equal(hash[:], row.Fingerprint) {
		return port.SubmissionFact{}, ErrRebuildConflict
	}
	wire, recognized, err := legacy.Decode(row.Payload)
	if err != nil || !recognized {
		return port.SubmissionFact{}, ErrRebuildConflict
	}
	env, err := domain.DecodeEnvelope(wire.Payload)
	if err != nil {
		return port.SubmissionFact{}, err
	}
	if wire.UUID != row.ID || env.ID != row.ID || env.EventType != row.EventType || env.AggregateType != "AnswerSheet" || env.AggregateID != strconv.FormatUint(sheet.ID, 10) || wire.Metadata["event_type"] != row.EventType || !env.OccurredAt.Truncate(time.Millisecond).Equal(sheet.DurableAcceptance.AcceptedAt.Truncate(time.Millisecond)) {
		return port.SubmissionFact{}, ErrRebuildConflict
	}
	var data payload.AnswerSheetSubmittedData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return port.SubmissionFact{}, err
	}
	if data.AnswerSheetID != env.AggregateID || data.OrgID != sheet.OrgID || data.TesteeID != sheet.TesteeID || sheet.FillerID <= 0 || data.FillerID != uint64(sheet.FillerID) || data.QuestionnaireCode != sheet.QuestionnaireCode || data.QuestionnaireVersion != sheet.QuestionnaireVersion || !data.SubmittedAt.Truncate(time.Millisecond).Equal(sheet.FilledAt.Truncate(time.Millisecond)) {
		return port.SubmissionFact{}, ErrRebuildConflict
	}
	return port.SubmissionFact{EventID: row.ID, QuestionnaireCode: data.QuestionnaireCode, SubmittedAt: data.SubmittedAt}, nil
}
