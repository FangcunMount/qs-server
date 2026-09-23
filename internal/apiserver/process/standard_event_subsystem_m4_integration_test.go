//go:build reliable_messaging_m4 && reliable_messaging_m4_integration

package process

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
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

// This proof uses the real process selection hook, QS original UoW, both
// standard status readers, two SDK relays and a real NSQ broker. All endpoints
// are pinned to an invocation-owned disposable compose network.
func TestM4ProcessBootstrapRunsSelectedStandardProfiles(t *testing.T) {
	dsn := os.Getenv("RM_QS_BOOTSTRAP_MYSQL_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m4_qs_bootstrap" {
		t.Fatal("disposable m4_qs_bootstrap MySQL at mysql:3306 required")
	}
	uri := os.Getenv("RM_QS_BOOTSTRAP_MONGO_URI")
	if !strings.HasPrefix(uri, "mongodb://mongo:27017/") || !strings.Contains(uri, "replicaSet=rm-test") {
		t.Fatal("disposable rm-test Mongo at mongo:27017 required")
	}
	if os.Getenv("RM_QS_BOOTSTRAP_NSQ_ADDR") != "nsqd:4150" {
		t.Fatal("disposable NSQ at nsqd:4150 required")
	}
	catalogPath := os.Getenv("RM_QS_BOOTSTRAP_CATALOG")
	if catalogPath != "/tmp/m4-qs-bootstrap/configs/events.yaml" {
		t.Fatal("copied invocation-owned event catalog required")
	}
	migrationPath := os.Getenv("RM_QS_BOOTSTRAP_MYSQL_MIGRATION")
	if migrationPath != "/tmp/m4-qs-bootstrap/mysql/000084_standard_reliable_outbox.up.sql" {
		t.Fatal("copied invocation-owned standard MySQL migration required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"qs_rm_replay_items", "qs_rm_replay_requests", "rm_outbox"} {
		if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
			t.Fatal(err)
		}
	}
	ddl, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(ddl), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		for _, table := range []string{"qs_rm_replay_items", "qs_rm_replay_requests", "rm_outbox"} {
			_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table)
		}
	}()
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	mongoClient, err := mongo.Connect(ctx, mongooptions.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer mongoClient.Disconnect(context.Background())
	mongoDB := mongoClient.Database("m4_qs_bootstrap")
	defer mongoDB.Drop(context.Background())
	if err := mongoDB.CreateCollection(ctx, "rm_outbox"); err != nil {
		t.Fatal(err)
	}
	if err := mongoDB.CreateCollection(ctx, "qs_rm_replay_requests"); err != nil {
		t.Fatal(err)
	}
	if _, err := mongoDB.Collection("rm_outbox").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "state", Value: 1}, {Key: "next_attempt_at", Value: 1}, {Key: "_id", Value: 1}}, Options: mongooptions.Index().SetName("ix_rm_outbox_due")},
		{Keys: bson.D{{Key: "state", Value: 1}, {Key: "lease_until", Value: 1}, {Key: "_id", Value: 1}}, Options: mongooptions.Index().SetName("ix_rm_outbox_lease")},
		{Keys: bson.D{{Key: "message_id", Value: 1}}, Options: mongooptions.Index().SetName("ix_rm_outbox_message_id")},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := mongoDB.Collection("qs_rm_replay_requests").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "org_id", Value: 1}, {Key: "created_at", Value: 1}, {Key: "_id", Value: 1}},
		Options: mongooptions.Index().SetName("ix_qs_rm_replay_requests_org_time"),
	}); err != nil {
		t.Fatal(err)
	}
	wire, err := eventcatalog.Load(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	catalog := eventcatalog.NewCatalog(wire)
	cfg := &config.Config{Options: options.NewOptions()}
	cfg.MessagingOptions.Enabled = true
	cfg.MessagingOptions.Provider = "nsq"
	cfg.MessagingOptions.NSQAddr = "nsqd:4150"
	cfg.Eventing.StandardOutbox.Mongo = true
	cfg.Eventing.StandardOutbox.Assessment = true
	deps := (&server{config: cfg}).buildEventSubsystemResourceDeps()
	deps.buildSubscriberFactory = nil // No projection subscriber in this sender-side proof.
	deps.mongo = eventsubsystem.ProfileOptions{Interval: 200 * time.Millisecond, PublishWorkers: 2}
	deps.assessment = eventsubsystem.ProfileOptions{Interval: 200 * time.Millisecond, PublishWorkers: 2}
	deps.consumers = map[string]eventsubsystem.ConsumerOptions{"modelcatalog.hot_rank_projection": {Enabled: false}}
	subsystem, err := buildResourceEventSubsystem(gormDB, mongoDB, nil, catalog, &fakePublisher{}, eventruntime.PublishModeMQ, nil, deps)
	if err != nil {
		t.Fatal(err)
	}
	defer subsystem.Close()
	if _, ok := subsystem.Profile(eventcatalog.OutboxProfileMongoDomain).Stager.(*mongostandard.Stager); !ok {
		t.Fatal("Mongo process profile kept the old writer")
	}
	if _, ok := subsystem.Profile(eventcatalog.OutboxProfileAssessmentMySQL).Stager.(*mysqlstandard.Stager); !ok {
		t.Fatal("MySQL process profile kept the old writer")
	}
	if err := subsystem.Start(ctx); err != nil {
		t.Fatal(err)
	}
	evt := event.Event[map[string]any]{
		BaseEvent: event.BaseEvent{
			ID: "m4-process-probe", EventTypeValue: "evaluation.requested", AggregateTypeValue: "Assessment",
			AggregateIDValue: "101", OccurredAtValue: time.Now(),
		},
		Data: map[string]any{"org_id": 501},
	}
	binding := subsystem.Profile(eventcatalog.OutboxProfileAssessmentMySQL)
	if err := qsmysql.NewUnitOfWork(gormDB).WithinTransaction(ctx, func(txCtx context.Context) error {
		return binding.Stager.Stage(txCtx, evt)
	}); err != nil {
		t.Fatal(err)
	}
	binding.PostCommit.AfterCommit(ctx, []event.DomainEvent{evt}, time.Now())
	mongoEvent := event.Event[map[string]any]{
		BaseEvent: event.BaseEvent{
			ID: "m4-process-mongo-probe", EventTypeValue: "answersheet.submitted", AggregateTypeValue: "AnswerSheet",
			AggregateIDValue: "201", OccurredAtValue: time.Now(),
		},
		Data: map[string]any{"org_id": 501},
	}
	mongoBinding := subsystem.Profile(eventcatalog.OutboxProfileMongoDomain)
	session, err := mongoClient.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(ctx)
	if _, err := session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (any, error) {
		return nil, mongoBinding.Stager.Stage(txCtx, mongoEvent)
	}); err != nil {
		t.Fatal(err)
	}
	mongoBinding.PostCommit.AfterCommit(ctx, []event.DomainEvent{mongoEvent}, time.Now())
	deadline := time.Now().Add(10 * time.Second)
	for {
		var state string
		if err := db.QueryRowContext(ctx, "SELECT state FROM rm_outbox WHERE message_id='m4-process-probe'").Scan(&state); err != nil {
			t.Fatal(err)
		}
		var mongoRow struct {
			State string `bson:"state"`
		}
		if err := mongoDB.Collection("rm_outbox").FindOne(ctx, bson.M{"message_id": "m4-process-mongo-probe"}).Decode(&mongoRow); err != nil {
			t.Fatal(err)
		}
		status, err := subsystem.StatusService().GetStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		sdkProfiles, healthyProfiles, liveOutboxes := 0, 0, 0
		for _, profile := range status.Profiles {
			if profile.RelayKind == "sdk" {
				sdkProfiles++
				if profile.ScanHealthy != nil && *profile.ScanHealthy {
					healthyProfiles++
				}
			}
		}
		for _, outbox := range status.Outboxes {
			if !outbox.Degraded && len(outbox.Buckets) == 4 {
				liveOutboxes++
			}
		}
		if state == "published" && mongoRow.State == "published" && sdkProfiles == 2 && healthyProfiles == 2 && liveOutboxes == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("process profiles did not publish and report both stores: mysql=%s mongo=%s profiles=%+v outboxes=%+v", state, mongoRow.State, status.Profiles, status.Outboxes)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := subsystem.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "DROP TABLE rm_outbox"); err != nil {
		t.Fatal(err)
	}
	if _, err := buildResourceEventSubsystem(gormDB, mongoDB, nil, catalog, &fakePublisher{}, eventruntime.PublishModeMQ, nil, deps); err == nil || !strings.Contains(err.Error(), "M4 MySQL standard schema is incomplete") {
		t.Fatalf("missing standard table did not fail before candidate startup: %v", err)
	}
}
