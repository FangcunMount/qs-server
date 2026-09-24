//go:build reliable_messaging_m4 && reliable_messaging_m4_integration

package process

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	mysqllegacy "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	qsmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	mysqldriver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// This is the M5 first-batch process selection: Mongo switches as one profile,
// MySQL stays on its original writer/runner, and hot rank keeps its own NSQ
// channel. The hot-rank business projection is verified by the answer-chain
// proof; this test verifies its production subscriber assembly and delivery.
func TestM5MongoOnlyProcessKeepsLegacyMySQLAndHotRankSubscription(t *testing.T) {
	dsn := os.Getenv("RM_QS_MONGO_ONLY_MYSQL_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m5_qs_mongo_only" {
		t.Fatal("disposable m5_qs_mongo_only MySQL at mysql:3306 required")
	}
	uri := os.Getenv("RM_QS_BOOTSTRAP_MONGO_URI")
	if !strings.HasPrefix(uri, "mongodb://mongo:27017/") || !strings.Contains(uri, "replicaSet=rm-test") ||
		os.Getenv("RM_QS_BOOTSTRAP_NSQ_ADDR") != "nsqd:4150" {
		t.Fatal("disposable Mongo replica set and nsqd:4150 required")
	}
	if os.Getenv("RM_QS_BOOTSTRAP_CATALOG") != "/tmp/m4-qs-bootstrap/configs/events.yaml" ||
		os.Getenv("RM_QS_BOOTSTRAP_AUDIT_MIGRATION") != "/tmp/m4-qs-bootstrap/mysql/000048_add_system_governance_action_runs.up.sql" {
		t.Fatal("invocation-owned event catalog and governance migration required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	auditDDL, err := os.ReadFile(os.Getenv("RM_QS_BOOTSTRAP_AUDIT_MIGRATION"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.ExecContext(ctx, string(auditDDL)); err != nil {
		t.Fatal(err)
	}
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gormDB.AutoMigrate(&mysqllegacy.OutboxPO{}); err != nil {
		t.Fatal(err)
	}
	mongoClient, err := mongo.Connect(ctx, mongooptions.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer mongoClient.Disconnect(context.Background())
	mongoDB := mongoClient.Database("m5_qs_mongo_only")
	defer mongoDB.Drop(context.Background())
	for _, name := range []string{"rm_outbox", "qs_rm_replay_requests"} {
		if err := mongoDB.CreateCollection(ctx, name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := mongoDB.Collection("rm_outbox").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "state", Value: 1}, {Key: "next_attempt_at", Value: 1}, {Key: "_id", Value: 1}}, Options: mongooptions.Index().SetName("ix_rm_outbox_due")},
		{Keys: bson.D{{Key: "state", Value: 1}, {Key: "lease_until", Value: 1}, {Key: "_id", Value: 1}}, Options: mongooptions.Index().SetName("ix_rm_outbox_lease")},
		{Keys: bson.D{{Key: "next_attempt_at", Value: 1}, {Key: "_id", Value: 1}}, Options: mongooptions.Index().SetName("ix_rm_outbox_active_due").SetPartialFilterExpression(bson.M{"state": bson.M{"$in": bson.A{"pending", "retry_wait", "publishing"}}})},
		{Keys: bson.D{{Key: "message_id", Value: 1}}, Options: mongooptions.Index().SetName("ix_rm_outbox_message_id")},
		{Keys: bson.D{{Key: "scope", Value: 1}, {Key: "state", Value: 1}, {Key: "last_error_code", Value: 1}, {Key: "updated_at", Value: -1}, {Key: "_id", Value: 1}}, Options: mongooptions.Index().SetName("ix_rm_outbox_scope_governance")},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := mongoDB.Collection("qs_rm_replay_requests").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "org_id", Value: 1}, {Key: "created_at", Value: 1}, {Key: "_id", Value: 1}},
		Options: mongooptions.Index().SetName("ix_qs_rm_replay_requests_org_time"),
	}); err != nil {
		t.Fatal(err)
	}
	wire, err := eventcatalog.Load(os.Getenv("RM_QS_BOOTSTRAP_CATALOG"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Options: options.NewOptions()}
	cfg.MessagingOptions.Enabled = true
	cfg.MessagingOptions.Provider = "nsq"
	cfg.MessagingOptions.NSQAddr = "nsqd:4150"
	cfg.MessagingOptions.NSQLookupdAddr = "nsqlookupd:4161"
	cfg.Eventing.StandardOutbox.Mongo = true
	cfg.Eventing.StandardOutbox.Assessment = false
	deps := (&server{config: cfg}).buildEventSubsystemResourceDeps()
	deps.mongo = eventsubsystem.ProfileOptions{Interval: 100 * time.Millisecond, PublishWorkers: 2}
	deps.assessment = eventsubsystem.ProfileOptions{Interval: 100 * time.Millisecond, BatchSize: 20, PublishWorkers: 2}
	subsystem, err := buildResourceEventSubsystem(gormDB, mongoDB, nil, eventcatalog.NewCatalog(wire),
		&fakePublisher{}, eventruntime.PublishModeMQ, nil, deps)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := subsystem.Close(); err != nil {
			t.Errorf("close mixed event subsystem: %v", err)
		}
	}()
	if _, ok := subsystem.Profile(eventcatalog.OutboxProfileMongoDomain).Stager.(*mongostandard.Stager); !ok {
		t.Fatal("selected Mongo profile did not use the standard stager")
	}
	if _, ok := subsystem.Profile(eventcatalog.OutboxProfileAssessmentMySQL).Stager.(*mysqllegacy.Store); !ok {
		t.Fatal("unselected MySQL profile did not keep the legacy stager")
	}
	const topic, channel = "qs.evaluation.lifecycle", "qs-apiserver-modelcatalog-hot-rank-v1"
	for _, path := range []string{
		"http://nsqd:4151/topic/create?topic=" + topic,
		"http://nsqd:4151/channel/create?topic=" + topic + "&channel=" + channel,
	} {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, path, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("create NSQ topic/channel: %s", response.Status)
		}
	}
	projectionEvents := make(chan string, 1)
	if err := subsystem.RegisterConsumer("modelcatalog.hot_rank_projection", func(_ context.Context, eventType string, _ []byte) error {
		select {
		case projectionEvents <- eventType:
		default:
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := subsystem.Start(ctx); err != nil {
		t.Fatal(err)
	}
	answer := event.Event[map[string]any]{BaseEvent: event.BaseEvent{
		ID: "m5-mongo-only-answer", EventTypeValue: eventcatalog.AnswerSheetSubmitted,
		AggregateTypeValue: "AnswerSheet", AggregateIDValue: "201", OccurredAtValue: time.Now(),
	}, Data: map[string]any{"org_id": 501}}
	session, err := mongoClient.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	if _, err := session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (any, error) {
		return nil, subsystem.Profile(eventcatalog.OutboxProfileMongoDomain).Stager.Stage(txCtx, answer)
	}); err != nil {
		t.Fatal(err)
	}
	subsystem.Profile(eventcatalog.OutboxProfileMongoDomain).PostCommit.AfterCommit(ctx, []event.DomainEvent{answer}, time.Now())
	select {
	case got := <-projectionEvents:
		if got != eventcatalog.AnswerSheetSubmitted {
			t.Fatalf("independent hot-rank event=%q", got)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("standard Mongo event did not reach the production hot-rank subscriber")
	}
	assessment := event.Event[map[string]any]{BaseEvent: event.BaseEvent{
		ID: "m5-mysql-legacy-probe", EventTypeValue: eventcatalog.EvaluationRequested,
		AggregateTypeValue: "Assessment", AggregateIDValue: "301", OccurredAtValue: time.Now(),
	}, Data: map[string]any{"org_id": 501}}
	if err := qsmysql.NewUnitOfWork(gormDB).WithinTransaction(ctx, func(txCtx context.Context) error {
		return subsystem.Profile(eventcatalog.OutboxProfileAssessmentMySQL).Stager.Stage(txCtx, assessment)
	}); err != nil {
		t.Fatal(err)
	}
	subsystem.Profile(eventcatalog.OutboxProfileAssessmentMySQL).PostCommit.AfterCommit(ctx, []event.DomainEvent{assessment}, time.Now())
	deadline := time.Now().Add(10 * time.Second)
	for {
		var mongoRow struct{ State string `bson:"state"` }
		var mysqlState string
		if err := mongoDB.Collection("rm_outbox").FindOne(ctx, bson.M{"message_id": answer.EventID()}).Decode(&mongoRow); err != nil {
			t.Fatal(err)
		}
		if err := sqlDB.QueryRowContext(ctx, "SELECT status FROM domain_event_outbox WHERE event_id=?", assessment.EventID()).Scan(&mysqlState); err != nil {
			t.Fatal(err)
		}
		if mongoRow.State == "published" && mysqlState == "published" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("mixed profile publish states: mongo=%s mysql=%s", mongoRow.State, mysqlState)
		}
		time.Sleep(100 * time.Millisecond)
	}
	status, err := subsystem.StatusService().GetStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	profiles := map[string]string{}
	for _, profile := range status.Profiles {
		profiles[string(profile.Name)] = profile.RelayKind
	}
	if profiles["mongo_domain_events"] != "sdk" || profiles["assessment_mysql_events"] != "legacy" {
		t.Fatalf("mixed process profile kinds=%v", profiles)
	}
	fmt.Fprintln(os.Stdout, "PASS M5 Mongo-only standard profile, legacy MySQL profile, independent hot-rank NSQ subscriber")
}
