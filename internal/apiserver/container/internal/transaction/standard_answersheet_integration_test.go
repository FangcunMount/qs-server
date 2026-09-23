//go:build integration && reliable_messaging_m4 && reliable_messaging_m4_integration

package transaction

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	appanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	domainquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	mongoanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	mongoevent "go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type failAfterStandardStage struct {
	inner *mongostandard.Stager
	err   error
}

func (s failAfterStandardStage) Stage(ctx context.Context, events ...event.DomainEvent) error {
	if err := s.inner.Stage(ctx, events...); err != nil {
		return err
	}
	return s.err
}

func standardSubmissionSheet(t *testing.T, id uint64, answer string, admission ...domainanswersheet.Admission) *domainanswersheet.AnswerSheet {
	t.Helper()
	ref, err := domainanswersheet.NewQuestionnaireRef("QNR-M4", "1.0.0", "M4")
	require.NoError(t, err)
	submission, err := domainanswersheet.NewSubmissionContext(
		actor.NewFillerRef(301, actor.FillerTypeSelf), actor.NewTesteeRef(meta.FromUint64(401)),
		meta.FromUint64(501), "task-m4", admission...,
	)
	require.NoError(t, err)
	value, err := domainanswersheet.NewAnswer(meta.NewCode("Q1"), domainquestionnaire.TypeText, domainanswersheet.NewStringValue(answer), 0)
	require.NoError(t, err)
	sheet, err := domainanswersheet.Submit(meta.FromUint64(id), ref, submission, []domainanswersheet.Answer{value}, time.Now())
	require.NoError(t, err)
	return sheet
}

func TestStandardAnswerSheetOriginalTransaction(t *testing.T) {
	uri := os.Getenv("RM_QS_MONGO_URI")
	if !strings.HasPrefix(uri, "mongodb://mongo:27017/") || !strings.Contains(uri, "replicaSet=rm-test") {
		t.Fatal("disposable rm-test replica-set URI required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err)
	defer client.Disconnect(context.Background())
	db := client.Database("rm_qs_standard_answersheet")
	defer db.Drop(context.Background())
	require.NoError(t, db.CreateCollection(ctx, "rm_outbox"))
	outbox := db.Collection("rm_outbox")
	_, err = outbox.Indexes().CreateMany(ctx, sdkmongo.Indexes())
	require.NoError(t, err)
	repo, err := mongoanswersheet.NewRepository(db)
	require.NoError(t, err)
	config, err := eventcatalog.Parse([]byte(`version: "1"
topics:
  evaluation:
    name: qs.evaluation.lifecycle
events:
  answersheet.submitted:
    topic: evaluation
    delivery: durable_outbox
    aggregate: AnswerSheet
    domain: survey
    handler: answersheet_submitted_handler
`))
	require.NoError(t, err)
	stager, err := mongostandard.NewStager(outbox, eventcatalog.NewCatalog(config), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	limiter := &transactionLimiterSpy{}
	runner := NewMongoRunner(db, MongoRunnerOptions{Boundary: "answersheet_submit_m4", Limiter: limiter})
	durable := appanswersheet.NewTransactionalSubmissionDurableStore(runner, repo, stager, nil)

	sheet := standardSubmissionSheet(t, 90010001, "accepted")
	submitted := sheet.Events()[0]
	wire, err := standardoutbox.EncodeWire(submitted, eventruntime.SourceAPIServer)
	require.NoError(t, err)
	fingerprint, err := submitport.Fingerprint(sheet)
	require.NoError(t, err)
	metaInfo := appanswersheet.DurableSubmitMeta{WriterID: 301, IdempotencyKey: "m4-accepted", Fingerprint: fingerprint}
	got, existed, err := durable.CreateDurably(ctx, sheet, metaInfo)
	require.NoError(t, err)
	require.False(t, existed)
	require.Equal(t, sheet.ID(), got.ID())
	var accepted struct {
		DurableAcceptance struct {
			EventID string `bson:"event_id"`
		} `bson:"durable_acceptance"`
	}
	require.NoError(t, db.Collection("answersheets").FindOne(ctx, bson.M{"domain_id": uint64(90010001)}).Decode(&accepted))
	require.Equal(t, submitted.EventID(), accepted.DurableAcceptance.EventID)
	require.EqualValues(t, 1, countStandardDocs(t, ctx, db.Collection("answersheets"), bson.M{}))
	require.EqualValues(t, 1, countStandardDocs(t, ctx, outbox, bson.M{}))
	require.Zero(t, countStandardDocs(t, ctx, db.Collection("domain_event_outbox"), bson.M{}))
	store, err := sdkmongo.New(outbox)
	require.NoError(t, err)
	claims, err := store.ClaimDue(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, submitted.EventID(), claims[0].Message.Input().ID)
	require.Equal(t, "org:501", claims[0].Message.Input().Scope)
	require.True(t, strings.HasSuffix(claims[0].Message.Input().OccurredAt, "+08:00"))
	require.Equal(t, wire, claims[0].Message.Input().Payload)
	require.NoError(t, store.Confirm(ctx, claims[0]))
	got, existed, err = durable.CreateDurably(ctx, sheet, metaInfo)
	require.NoError(t, err)
	require.True(t, existed)
	require.Equal(t, sheet.ID(), got.ID())
	require.EqualValues(t, 1, countStandardDocs(t, ctx, outbox, bson.M{}))

	abort := errors.New("abort after standard Outbox stage")
	failing := appanswersheet.NewTransactionalSubmissionDurableStore(runner, repo, failAfterStandardStage{inner: stager, err: abort}, nil)
	failed := standardSubmissionSheet(t, 90010002, "rolled back")
	failedEventID := failed.Events()[0].EventID()
	failedFingerprint, err := submitport.Fingerprint(failed)
	require.NoError(t, err)
	_, _, err = failing.CreateDurably(ctx, failed, appanswersheet.DurableSubmitMeta{WriterID: 301, IdempotencyKey: "m4-rollback", Fingerprint: failedFingerprint})
	require.ErrorIs(t, err, abort)
	require.Zero(t, countStandardDocs(t, ctx, db.Collection("answersheets"), bson.M{"domain_id": uint64(90010002)}))
	require.Zero(t, countStandardDocs(t, ctx, outbox, bson.M{"message_id": failedEventID}))
	require.Equal(t, limiter.acquired, limiter.released)

	// The database commit succeeds, but the caller loses its result and its
	// request context. Acceptance must come from the original intent's durable
	// AnswerSheet, not from submitting the transaction a second time.
	unknownSheet := standardSubmissionSheet(t, 90010003, "committed but reply lost")
	unknownEventID := unknownSheet.Events()[0].EventID()
	unknownFingerprint, err := submitport.Fingerprint(unknownSheet)
	require.NoError(t, err)
	unknownMeta := appanswersheet.DurableSubmitMeta{
		WriterID: 301, IdempotencyKey: "m4-commit-unknown", Fingerprint: unknownFingerprint,
	}
	requestCtx, cancelRequest := context.WithCancel(ctx)
	defer cancelRequest()
	unknownRunner := apptransaction.RunnerFunc(func(runCtx context.Context, fn func(context.Context) error) error {
		if err := runner.WithinTransaction(runCtx, fn); err != nil {
			return err
		}
		cancelRequest()
		return standardSubmissionUnknownCommit{}
	})
	unknownStore := appanswersheet.NewTransactionalSubmissionDurableStore(unknownRunner, repo, stager, nil)
	recovered, existed, err := unknownStore.CreateDurably(requestCtx, unknownSheet, unknownMeta)
	require.NoError(t, err)
	require.True(t, existed)
	require.Equal(t, unknownSheet.ID(), recovered.ID())
	require.EqualValues(t, 1, countStandardDocs(t, ctx, db.Collection("answersheets"), bson.M{"domain_id": uint64(90010003)}))
	require.EqualValues(t, 1, countStandardDocs(t, ctx, outbox, bson.M{"message_id": unknownEventID}))
	recovered, existed, err = durable.CreateDurably(ctx, unknownSheet, unknownMeta)
	require.NoError(t, err)
	require.True(t, existed)
	require.Equal(t, unknownSheet.ID(), recovered.ID())
	require.EqualValues(t, 1, countStandardDocs(t, ctx, outbox, bson.M{"message_id": unknownEventID}))

	// An unknown result without a committed intent must not be acknowledged.
	// A later retry with the same intent may then create exactly one pair.
	uncommitted := standardSubmissionSheet(t, 90010004, "not committed")
	uncommittedEventID := uncommitted.Events()[0].EventID()
	uncommittedFingerprint, err := submitport.Fingerprint(uncommitted)
	require.NoError(t, err)
	uncommittedMeta := appanswersheet.DurableSubmitMeta{
		WriterID: 301, IdempotencyKey: "m4-commit-absent", Fingerprint: uncommittedFingerprint,
	}
	noCommitRunner := apptransaction.RunnerFunc(func(context.Context, func(context.Context) error) error {
		return standardSubmissionUnknownCommit{}
	})
	noCommitStore := appanswersheet.NewTransactionalSubmissionDurableStore(noCommitRunner, repo, stager, nil)
	result, existed, err := noCommitStore.CreateDurably(ctx, uncommitted, uncommittedMeta)
	require.Error(t, err)
	require.Nil(t, result)
	require.False(t, existed)
	require.Zero(t, countStandardDocs(t, ctx, db.Collection("answersheets"), bson.M{"domain_id": uint64(90010004)}))
	require.Zero(t, countStandardDocs(t, ctx, outbox, bson.M{"message_id": uncommittedEventID}))
	result, existed, err = durable.CreateDurably(ctx, uncommitted, uncommittedMeta)
	require.NoError(t, err)
	require.False(t, existed)
	require.Equal(t, uncommitted.ID(), result.ID())
	require.EqualValues(t, 1, countStandardDocs(t, ctx, db.Collection("answersheets"), bson.M{"domain_id": uint64(90010004)}))
	require.EqualValues(t, 1, countStandardDocs(t, ctx, outbox, bson.M{"message_id": uncommittedEventID}))
	require.Equal(t, limiter.acquired, limiter.released)

	// The server returns a real UnknownTransactionCommitResult on the first
	// commit reply. The driver must retry commit, not the AnswerSheet callback.
	var commits, unknownReplies, inserts, updates atomic.Int32
	monitor := &mongoevent.CommandMonitor{
		Started: func(_ context.Context, evt *mongoevent.CommandStartedEvent) {
			switch evt.CommandName {
			case "commitTransaction":
				commits.Add(1)
			case "insert":
				inserts.Add(1)
			case "update":
				updates.Add(1)
			}
		},
		Succeeded: func(_ context.Context, evt *mongoevent.CommandSucceededEvent) {
			if evt.CommandName == "commitTransaction" && evt.Reply.Lookup("writeConcernError").Type != 0 {
				unknownReplies.Add(1)
			}
		},
	}
	monitoredClient, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetMonitor(monitor))
	require.NoError(t, err)
	defer monitoredClient.Disconnect(context.Background())
	monitoredDB := monitoredClient.Database(db.Name())
	monitoredRepo, err := mongoanswersheet.NewRepository(monitoredDB)
	require.NoError(t, err)
	monitoredStager, err := mongostandard.NewStager(monitoredDB.Collection("rm_outbox"), eventcatalog.NewCatalog(config), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	monitoredRunner := NewMongoRunner(monitoredDB, MongoRunnerOptions{Boundary: "answersheet_submit_m4_unknown", Limiter: &transactionLimiterSpy{}})
	monitoredStore := appanswersheet.NewTransactionalSubmissionDurableStore(monitoredRunner, monitoredRepo, monitoredStager, nil)
	admin := client.Database("admin")
	require.NoError(t, admin.RunCommand(ctx, bson.D{
		{Key: "configureFailPoint", Value: "failCommand"},
		{Key: "mode", Value: bson.M{"times": 1}},
		{Key: "data", Value: bson.M{
			"failCommands":      bson.A{"commitTransaction"},
			"writeConcernError": bson.M{"code": 64, "errmsg": "isolated unknown commit result"},
			"errorLabels":       bson.A{"UnknownTransactionCommitResult"},
		}},
	}).Err())
	driverUnknown := standardSubmissionSheet(t, 90010005, "driver commit retry")
	driverUnknownEventID := driverUnknown.Events()[0].EventID()
	driverUnknownFingerprint, err := submitport.Fingerprint(driverUnknown)
	require.NoError(t, err)
	driverUnknownMeta := appanswersheet.DurableSubmitMeta{
		WriterID: 301, IdempotencyKey: "m4-driver-commit-unknown", Fingerprint: driverUnknownFingerprint,
	}
	result, existed, err = monitoredStore.CreateDurably(ctx, driverUnknown, driverUnknownMeta)
	require.NoError(t, err)
	require.False(t, existed)
	require.Equal(t, driverUnknown.ID(), result.ID())
	require.EqualValues(t, 2, commits.Load())
	require.EqualValues(t, 1, unknownReplies.Load())
	require.EqualValues(t, 1, inserts.Load())
	require.EqualValues(t, 1, updates.Load())
	require.EqualValues(t, 1, countStandardDocs(t, ctx, db.Collection("answersheets"), bson.M{"domain_id": uint64(90010005)}))
	require.EqualValues(t, 1, countStandardDocs(t, ctx, outbox, bson.M{"message_id": driverUnknownEventID}))
	result, existed, err = durable.CreateDurably(ctx, driverUnknown, driverUnknownMeta)
	require.NoError(t, err)
	require.True(t, existed)
	require.Equal(t, driverUnknown.ID(), result.ID())
	require.EqualValues(t, 1, countStandardDocs(t, ctx, outbox, bson.M{"message_id": driverUnknownEventID}))
	require.NoError(t, admin.RunCommand(ctx, bson.D{
		{Key: "configureFailPoint", Value: "failCommand"}, {Key: "mode", Value: "off"},
	}).Err())
}

type standardSubmissionUnknownCommit struct{}

func (standardSubmissionUnknownCommit) Error() string { return "isolated unknown Mongo commit result" }

func (standardSubmissionUnknownCommit) HasErrorLabel(label string) bool {
	return label == "UnknownTransactionCommitResult"
}

func countStandardDocs(t *testing.T, ctx context.Context, coll *mongo.Collection, filter bson.M) int64 {
	t.Helper()
	count, err := coll.CountDocuments(ctx, filter)
	require.NoError(t, err)
	return count
}
