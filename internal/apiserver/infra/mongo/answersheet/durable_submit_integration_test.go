package answersheet_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	appanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	domainquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	mongoanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type integrationOutboxStager struct {
	coll *mongo.Collection
	err  error
}

func (s integrationOutboxStager) Stage(ctx context.Context, events ...event.DomainEvent) error {
	if s.err != nil {
		return s.err
	}
	if _, ok := ctx.(mongo.SessionContext); !ok {
		return errors.New("active transaction required")
	}
	docs := make([]interface{}, 0, len(events))
	for _, evt := range events {
		docs = append(docs, bson.M{"event_id": evt.EventID(), "event_type": evt.EventType(), "aggregate_id": evt.AggregateID()})
	}
	if len(docs) == 0 {
		return nil
	}
	_, err := s.coll.InsertMany(ctx, docs)
	return err
}

func mongoIntegrationRunner(db *mongo.Database) apptransaction.Runner {
	return apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		session, err := db.Client().StartSession()
		if err != nil {
			return err
		}
		defer session.EndSession(ctx)
		_, err = session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (interface{}, error) {
			return nil, fn(txCtx)
		})
		return err
	})
}

func openDurableSubmitMongo(t *testing.T) *mongo.Database {
	t.Helper()
	uri := os.Getenv("QS_SERVER_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("QS_SERVER_TEST_MONGO_URI is not set; reliable-submit Mongo replica-set integration test skipped")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect Mongo: %v", err)
	}
	var hello bson.M
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		t.Fatalf("Mongo hello: %v", err)
	}
	if hello["setName"] == nil {
		_ = client.Disconnect(context.Background())
		t.Skip("QS_SERVER_TEST_MONGO_URI does not point to a replica set")
	}
	base := os.Getenv("QS_SERVER_TEST_MONGO_DB")
	if base == "" {
		base = "qs_server_contract_test"
	}
	db := client.Database(fmt.Sprintf("%s_answersheet_durable_%d", base, time.Now().UnixNano()))
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = db.Drop(cleanupCtx)
		_ = client.Disconnect(cleanupCtx)
	})
	return db
}

func TestDurableSubmissionTransactionAgainstMongoReplicaSet(t *testing.T) {
	db := openDurableSubmitMongo(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	idempotency := db.Collection("answersheet_submit_idempotency")
	if _, err := idempotency.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "idempotency_key", Value: 1}}, Options: options.Index().SetName("uk_idempotency_key").SetUnique(true),
	}); err != nil {
		t.Fatalf("create legacy index: %v", err)
	}
	repo, err := mongoanswersheet.NewRepository(db)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	indexCursor, err := db.Collection("answersheets").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	var indexes []bson.M
	if err := indexCursor.All(ctx, &indexes); err != nil {
		t.Fatalf("decode indexes: %v", err)
	}
	indexNames := map[string]bool{}
	for _, index := range indexes {
		indexNames[fmt.Sprint(index["name"])] = true
	}
	if !indexNames["uk_answersheet_submit_intent"] {
		t.Fatalf("answersheet indexes = %v, want embedded submit intent unique index", indexNames)
	}

	outbox := db.Collection("domain_event_outbox")
	store := appanswersheet.NewTransactionalSubmissionDurableStore(mongoIntegrationRunner(db), repo, integrationOutboxStager{coll: outbox}, nil)
	sheet := newIntegrationSheet(t, 90010001, "ok")
	fingerprint, err := submitport.Fingerprint(sheet)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	metaInfo := appanswersheet.DurableSubmitMeta{WriterID: 301, IdempotencyKey: "integration-idem-0001", Fingerprint: fingerprint}
	stored, existed, err := store.CreateDurably(ctx, sheet, metaInfo)
	if err != nil || existed || stored == nil {
		t.Fatalf("CreateDurably() = stored=%v existed=%v err=%v", stored != nil, existed, err)
	}
	assertMongoCount(t, ctx, db.Collection("answersheets"), bson.M{"domain_id": uint64(90010001)}, 1)
	assertMongoCount(t, ctx, db.Collection("answersheets"), bson.M{"submit_meta.writer_id": uint64(301), "submit_meta.idempotency_key": metaInfo.IdempotencyKey}, 1)
	assertMongoCount(t, ctx, idempotency, bson.M{"writer_id": uint64(301), "idempotency_key": metaInfo.IdempotencyKey}, 0)
	assertMongoCount(t, ctx, outbox, bson.M{"aggregate_id": sheet.ID().String()}, 1)

	stageFailure := errors.New("injected outbox stage failure")
	failingStore := appanswersheet.NewTransactionalSubmissionDurableStore(mongoIntegrationRunner(db), repo, integrationOutboxStager{coll: outbox, err: stageFailure}, nil)
	failedSheet := newIntegrationSheet(t, 90010002, "rollback")
	failedFingerprint, _ := submitport.Fingerprint(failedSheet)
	_, _, err = failingStore.CreateDurably(ctx, failedSheet, appanswersheet.DurableSubmitMeta{WriterID: 301, IdempotencyKey: "integration-idem-0002", Fingerprint: failedFingerprint})
	if !errors.Is(err, stageFailure) {
		t.Fatalf("stage failure error = %v, want injected error", err)
	}
	assertMongoCount(t, ctx, db.Collection("answersheets"), bson.M{"domain_id": uint64(90010002)}, 0)
	assertMongoCount(t, ctx, idempotency, bson.M{"idempotency_key": "integration-idem-0002"}, 0)

	concurrentKey := "integration-idem-concurrent"
	sheets := []*domainanswersheet.AnswerSheet{newIntegrationSheet(t, 90010003, "same"), newIntegrationSheet(t, 90010004, "same")}
	concurrentFingerprint, _ := submitport.Fingerprint(sheets[0])
	var wg sync.WaitGroup
	ids := make(chan uint64, len(sheets))
	errs := make(chan error, len(sheets))
	for _, candidate := range sheets {
		wg.Add(1)
		go func(candidate *domainanswersheet.AnswerSheet) {
			defer wg.Done()
			got, _, submitErr := store.CreateDurably(ctx, candidate, appanswersheet.DurableSubmitMeta{WriterID: 301, IdempotencyKey: concurrentKey, Fingerprint: concurrentFingerprint})
			if submitErr != nil {
				errs <- submitErr
				return
			}
			ids <- got.ID().Uint64()
		}(candidate)
	}
	wg.Wait()
	close(errs)
	for submitErr := range errs {
		t.Fatalf("concurrent durable submit: %v", submitErr)
	}
	close(ids)
	var winner uint64
	for id := range ids {
		if winner == 0 {
			winner = id
		} else if id != winner {
			t.Fatalf("concurrent ids differ: %d and %d", winner, id)
		}
	}
	assertMongoCount(t, ctx, db.Collection("answersheets"), bson.M{"submit_meta.writer_id": uint64(301), "submit_meta.idempotency_key": concurrentKey}, 1)

	conflictingKey := "integration-idem-conflicting"
	conflictingSheets := []*domainanswersheet.AnswerSheet{newIntegrationSheet(t, 90010005, "left"), newIntegrationSheet(t, 90010006, "right")}
	conflictErrs := make(chan error, len(conflictingSheets))
	for _, candidate := range conflictingSheets {
		fingerprint, fingerprintErr := submitport.Fingerprint(candidate)
		if fingerprintErr != nil {
			t.Fatal(fingerprintErr)
		}
		wg.Add(1)
		go func(candidate *domainanswersheet.AnswerSheet, candidateFingerprint string) {
			defer wg.Done()
			_, _, submitErr := store.CreateDurably(ctx, candidate, appanswersheet.DurableSubmitMeta{WriterID: 301, IdempotencyKey: conflictingKey, Fingerprint: candidateFingerprint})
			conflictErrs <- submitErr
		}(candidate, fingerprint)
	}
	wg.Wait()
	close(conflictErrs)
	var successes, conflicts int
	for submitErr := range conflictErrs {
		switch {
		case submitErr == nil:
			successes++
		case errors.Is(submitErr, submitport.ErrIdempotencyConflict):
			conflicts++
		default:
			t.Fatalf("conflicting concurrent durable submit: %v", submitErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("conflicting concurrent results: successes=%d conflicts=%d", successes, conflicts)
	}
	assertMongoCount(t, ctx, db.Collection("answersheets"), bson.M{"submit_meta.writer_id": uint64(301), "submit_meta.idempotency_key": conflictingKey}, 1)
	assertMongoCount(t, ctx, db.Collection("answersheets"), bson.M{"domain_id": bson.M{"$in": []uint64{90010005, 90010006}}}, 1)
}

func newIntegrationSheet(t *testing.T, id uint64, value string) *domainanswersheet.AnswerSheet {
	t.Helper()
	ref, err := domainanswersheet.NewQuestionnaireRef("QNR-INTEGRATION", "1.0.0", "Integration")
	if err != nil {
		t.Fatal(err)
	}
	submission, err := domainanswersheet.NewSubmissionContext(
		actor.NewFillerRef(301, actor.FillerTypeSelf), actor.NewTesteeRef(meta.FromUint64(401)), meta.FromUint64(501), "task-integration",
	)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := domainanswersheet.NewAnswer(meta.NewCode("Q1"), domainquestionnaire.TypeText, domainanswersheet.NewStringValue(value), 0)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := domainanswersheet.Submit(meta.FromUint64(id), ref, submission, []domainanswersheet.Answer{answer}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return sheet
}

func assertMongoCount(t *testing.T, ctx context.Context, coll *mongo.Collection, filter interface{}, want int64) {
	t.Helper()
	got, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		t.Fatalf("count %s: %v", coll.Name(), err)
	}
	if got != want {
		t.Fatalf("count %s = %d, want %d", coll.Name(), got, want)
	}
}
