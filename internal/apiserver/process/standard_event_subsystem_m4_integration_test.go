//go:build reliable_messaging_m4 && reliable_messaging_m4_integration

package process

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	platformmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/platform"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	governanceinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	qsmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
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
	auditMigrationPath := os.Getenv("RM_QS_BOOTSTRAP_AUDIT_MIGRATION")
	if auditMigrationPath != "/tmp/m4-qs-bootstrap/mysql/000048_add_system_governance_action_runs.up.sql" {
		t.Fatal("copied invocation-owned governance audit migration required")
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
	for _, table := range []string{"qs_rm_replay_items", "qs_rm_replay_requests", "rm_outbox", "system_governance_action_runs"} {
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
	auditDDL, err := os.ReadFile(auditMigrationPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(auditDDL)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, table := range []string{"qs_rm_replay_items", "qs_rm_replay_requests", "rm_outbox", "system_governance_action_runs"} {
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
	// Exercise the actual platform facade with the status readers exported by
	// process assembly. Direct ledger tests do not prove this runtime seam.
	actionCtx := actorctx.WithGrantingUserID(ctx, 110004)
	governance := platformmod.BuildRESTSystemGovernanceFacade(platformmod.RESTSystemGovernanceInput{
		EventOutboxes: subsystem.Outboxes(), MySQLDB: gormDB, MongoDB: mongoDB,
	})
	if _, err := db.ExecContext(ctx, `UPDATE rm_outbox SET state='quarantined',last_error_code='publish_unknown',failure_count=30,updated_at=UTC_TIMESTAMP(6) WHERE message_id='m4-process-probe'`); err != nil {
		t.Fatal(err)
	}
	if _, err := mongoDB.Collection("rm_outbox").UpdateOne(ctx, bson.M{"message_id": "m4-process-mongo-probe"},
		bson.M{"$set": bson.M{"state": "quarantined", "last_error_code": "publish_unknown", "failure_count": int64(30)},
			"$currentDate": bson.M{"updated_at": true}}); err != nil {
		t.Fatal(err)
	}
	for _, outbox := range subsystem.Outboxes() {
		reader, ok := outbox.Reader.(systemgov.OutboxGovernanceReader)
		if !ok {
			t.Fatalf("selected profile %s lacks tenant-scoped governance view", outbox.Name)
		}
		summary, err := reader.ReadOutboxGovernance(ctx, 501)
		if err != nil || summary.ManualRequired != 1 || summary.Automatic != 0 {
			t.Fatalf("selected profile %s hid manual row: summary=%+v err=%v", outbox.Name, summary, err)
		}
		items, err := reader.ListOutboxCandidates(ctx, 501, 10)
		if err != nil || len(items) != 1 || items[0].Store != outbox.Name || items[0].Attempt != 30 ||
			items[0].Disposition != "manual_required" || items[0].UpdatedAt.IsZero() {
			t.Fatalf("selected profile %s candidate view is incomplete: items=%+v err=%v", outbox.Name, items, err)
		}
		other, err := reader.ReadOutboxGovernance(ctx, 502)
		if err != nil || other != (systemgov.OutboxGovernanceSummary{}) {
			t.Fatalf("selected profile %s leaked another organization: summary=%+v err=%v", outbox.Name, other, err)
		}
		otherItems, err := reader.ListOutboxCandidates(ctx, 502, 10)
		if err != nil || len(otherItems) != 0 {
			t.Fatalf("selected profile %s leaked candidates to another organization: items=%+v err=%v", outbox.Name, otherItems, err)
		}
	}
	var mysqlVersion, originalMySQLVersion uint64
	if err := db.QueryRowContext(ctx, `SELECT version FROM rm_outbox WHERE message_id='m4-process-probe'`).Scan(&originalMySQLVersion); err != nil {
		t.Fatal(err)
	}
	var mongoVersion, originalMongoVersion struct {
		Version uint64 `bson:"version"`
	}
	if err := mongoDB.Collection("rm_outbox").FindOne(ctx, bson.M{"message_id": "m4-process-mongo-probe"}).Decode(&originalMongoVersion); err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct{ store, eventID string }{
		{"assessment-mysql-outbox", "m4-process-probe"},
		{"mongo-domain-events", "m4-process-mongo-probe"},
	} {
		request := systemgov.ActionRunRequest{
			RequestID: "approve-" + target.eventID, Confirm: true,
			Input: map[string]interface{}{
				"store": target.store, "reason": "isolated operator review",
				"targets": []interface{}{map[string]interface{}{"event_id": target.eventID, "expected_attempt_count": 30}},
			},
		}
		approved, err := governance.RunAction(actionCtx, 501, "events.replay_pending", request)
		if err != nil || approved == nil || approved.Result["authorized"] != 1 {
			t.Fatalf("standard profile %s is not governable: result=%+v err=%v", target.store, approved, err)
		}
		for _, outbox := range subsystem.Outboxes() {
			if outbox.Name != target.store {
				continue
			}
			reader := outbox.Reader.(systemgov.OutboxGovernanceReader)
			summary, err := reader.ReadOutboxGovernance(ctx, 501)
			if err != nil || summary.Authorized != 1 || summary.ManualRequired != 0 {
				t.Fatalf("standard replay %s was double-counted after authorization: summary=%+v err=%v", target.store, summary, err)
			}
			items, err := reader.ListOutboxCandidates(ctx, 501, 10)
			if err != nil || len(items) != 0 {
				t.Fatalf("standard replay %s remained actionable after authorization: items=%+v err=%v", target.store, items, err)
			}
		}
		prior, err := governance.RunAction(actionCtx, 501, "events.replay_pending", request)
		if err != nil || prior == nil || prior.RequestID != approved.RequestID {
			t.Fatalf("standard replay %s did not retain idempotent result: result=%+v err=%v", target.store, prior, err)
		}
		changed := request
		changed.Input = map[string]interface{}{
			"store": target.store, "reason": "changed approval",
			"targets": []interface{}{map[string]interface{}{"event_id": target.eventID, "expected_attempt_count": 30}},
		}
		if _, err := governance.RunAction(actionCtx, 501, "events.replay_pending", changed); err == nil {
			t.Fatalf("standard replay %s accepted changed approval input", target.store)
		}
		var auditStatus string
		if err := db.QueryRowContext(ctx, `SELECT status FROM system_governance_action_runs WHERE org_id=501 AND request_id=?`,
			request.RequestID).Scan(&auditStatus); err != nil || auditStatus != "ok" {
			t.Fatalf("standard replay %s lacks completed audit: status=%s err=%v", target.store, auditStatus, err)
		}
	}
	if err := db.QueryRowContext(ctx, `SELECT version FROM rm_outbox WHERE message_id='m4-process-probe'`).Scan(&mysqlVersion); err != nil || mysqlVersion != originalMySQLVersion+1 {
		t.Fatalf("MySQL standard replay advanced unexpected number of times: version=%d err=%v", mysqlVersion, err)
	}
	if err := mongoDB.Collection("rm_outbox").FindOne(ctx, bson.M{"message_id": "m4-process-mongo-probe"}).Decode(&mongoVersion); err != nil || mongoVersion.Version != originalMongoVersion.Version+1 {
		t.Fatalf("Mongo standard replay advanced unexpected number of times: version=%d err=%v", mongoVersion.Version, err)
	}
	// Simulate a process exit after authorization committed but before the
	// MySQL action audit was completed. The facade must resolve, not reissue.
	if _, err := db.ExecContext(ctx, `UPDATE rm_outbox SET state='quarantined',last_error_code='publish_unknown',failure_count=31 WHERE message_id='m4-process-probe'`); err != nil {
		t.Fatal(err)
	}
	crashInput := map[string]interface{}{
		"store": "assessment-mysql-outbox", "reason": "recover original approval",
		"targets": []interface{}{map[string]interface{}{"event_id": "m4-process-probe", "expected_attempt_count": 31}},
	}
	crashAudit := systemgov.ActionAuditRecord{
		OrgID: 501, RequestID: "approve-after-crash", ActionID: "events.replay_pending", ActorUserID: 110004,
		Input: crashInput, StartedAt: time.Now(), Status: "running",
	}
	auditStore := governanceinfra.NewActionAuditStore(gormDB)
	if prior, claimed, err := auditStore.Claim(actionCtx, crashAudit); err != nil || prior != nil || !claimed {
		t.Fatalf("seed running governance audit: prior=%+v claimed=%t err=%v", prior, claimed, err)
	}
	var durable outboxport.DurableManualReplayAuthorizer
	for _, outbox := range subsystem.Outboxes() {
		if outbox.Name == "assessment-mysql-outbox" {
			durable, _ = outbox.Reader.(outboxport.DurableManualReplayAuthorizer)
		}
	}
	if durable == nil {
		t.Fatal("selected MySQL profile did not export its durable replay owner")
	}
	if _, err := durable.AuthorizeManualReplayWithReason(actionCtx, 501, crashAudit.RequestID, "recover original approval",
		[]outboxport.ManualReplayTarget{{EventID: "m4-process-probe", ExpectedAttemptCount: 31}}); err != nil {
		t.Fatal(err)
	}
	recovered, err := governance.RunAction(actionCtx, 501, "events.replay_pending", systemgov.ActionRunRequest{
		RequestID: crashAudit.RequestID, Confirm: true, Input: crashInput,
	})
	if err != nil || recovered == nil || recovered.Result["authorized"] != 1 {
		t.Fatalf("committed authorization did not recover through facade: result=%+v err=%v", recovered, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT version FROM rm_outbox WHERE message_id='m4-process-probe'`).Scan(&mysqlVersion); err != nil || mysqlVersion != originalMySQLVersion+2 {
		t.Fatalf("recovery reauthorized or lost existing grant: version=%d err=%v", mysqlVersion, err)
	}
	if err := subsystem.Close(); err != nil {
		t.Fatal(err)
	}
	// A missing ledger result is explicit pending review, not permission to
	// execute the action again. A later committed result closes the same audit.
	for _, target := range []struct {
		store, eventID string
		quarantine     func() error
	}{
		{"assessment-mysql-outbox", "m4-process-probe", func() error {
			_, err := db.ExecContext(ctx, `UPDATE rm_outbox SET state='quarantined',last_error_code='publish_unknown',failure_count=31 WHERE message_id='m4-process-probe'`)
			return err
		}},
		{"mongo-domain-events", "m4-process-mongo-probe", func() error {
			_, err := mongoDB.Collection("rm_outbox").UpdateOne(ctx, bson.M{"message_id": "m4-process-mongo-probe"}, bson.M{"$set": bson.M{
				"state": "quarantined", "last_error_code": "publish_unknown", "failure_count": int64(31),
			}})
			return err
		}},
	} {
		if err := target.quarantine(); err != nil {
			t.Fatal(err)
		}
		input := map[string]interface{}{
			"store": target.store, "reason": "late committed approval",
			"targets": []interface{}{map[string]interface{}{"event_id": target.eventID, "expected_attempt_count": 31}},
		}
		audit := systemgov.ActionAuditRecord{
			OrgID: 501, RequestID: "pending-" + target.eventID, ActionID: "events.replay_pending",
			ActorUserID: 110004, Input: input, StartedAt: time.Now(), Status: "running",
		}
		if prior, claimed, err := auditStore.Claim(actionCtx, audit); err != nil || prior != nil || !claimed {
			t.Fatalf("seed unresolved %s audit: prior=%+v claimed=%t err=%v", target.store, prior, claimed, err)
		}
		request := systemgov.ActionRunRequest{RequestID: audit.RequestID, Confirm: true, Input: input}
		if _, err := governance.RunAction(actionCtx, 501, "events.replay_pending", request); err == nil {
			t.Fatalf("unresolved %s action was accepted without a ledger", target.store)
		}
		var status string
		var finished sql.NullTime
		if err := db.QueryRowContext(ctx, `SELECT status,finished_at FROM system_governance_action_runs WHERE org_id=501 AND request_id=?`, audit.RequestID).
			Scan(&status, &finished); err != nil || status != systemgov.ActionAuditStatusPendingReconciliation || finished.Valid {
			t.Fatalf("unresolved %s audit is not pending: status=%s finished=%v err=%v", target.store, status, finished, err)
		}
		var authorizer outboxport.DurableManualReplayAuthorizer
		for _, outbox := range subsystem.Outboxes() {
			if outbox.Name == target.store {
				authorizer, _ = outbox.Reader.(outboxport.DurableManualReplayAuthorizer)
			}
		}
		if authorizer == nil {
			t.Fatalf("missing durable %s authorizer", target.store)
		}
		items, err := authorizer.AuthorizeManualReplayWithReason(actionCtx, 501, audit.RequestID, "late committed approval",
			[]outboxport.ManualReplayTarget{{EventID: target.eventID, ExpectedAttemptCount: 31}})
		if err != nil || len(items) != 1 || !items[0].Authorized {
			t.Fatalf("late %s authorization failed: items=%+v err=%v", target.store, items, err)
		}
		result, err := governance.RunAction(actionCtx, 501, "events.replay_pending", request)
		if err != nil || result == nil || result.Result["authorized"] != 1 {
			t.Fatalf("pending %s audit did not converge: result=%+v err=%v", target.store, result, err)
		}
		if err := db.QueryRowContext(ctx, `SELECT status FROM system_governance_action_runs WHERE org_id=501 AND request_id=?`, audit.RequestID).
			Scan(&status); err != nil || status != "ok" {
			t.Fatalf("pending %s audit did not finish: status=%s err=%v", target.store, status, err)
		}
	}
	// A later SDK delivery failure starts a new automatic cycle. The prior
	// approval remains in the ledger but must not hide that candidate.
	mySQLStore, err := sdkmysql.New(db)
	if err != nil {
		t.Fatal(err)
	}
	mongoStore, err := sdkmongo.New(mongoDB.Collection("rm_outbox"))
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		store string
		claim func() error
	}{
		{"assessment-mysql-outbox", func() error {
			claims, err := mySQLStore.ClaimDue(ctx, 1, time.Minute)
			if err != nil || len(claims) != 1 {
				return fmt.Errorf("claim MySQL approved message: count=%d: %w", len(claims), err)
			}
			return mySQLStore.Retry(ctx, claims[0], time.Minute, "publish_unknown")
		}},
		{"mongo-domain-events", func() error {
			claims, err := mongoStore.ClaimDue(ctx, 1, time.Minute)
			if err != nil || len(claims) != 1 {
				return fmt.Errorf("claim Mongo approved message: count=%d: %w", len(claims), err)
			}
			return mongoStore.Retry(ctx, claims[0], time.Minute, "publish_unknown")
		}},
	} {
		if err := target.claim(); err != nil {
			t.Fatal(err)
		}
		for _, outbox := range subsystem.Outboxes() {
			if outbox.Name != target.store {
				continue
			}
			reader := outbox.Reader.(systemgov.OutboxGovernanceReader)
			summary, err := reader.ReadOutboxGovernance(ctx, 501)
			if err != nil || summary.Automatic != 1 || summary.Authorized != 0 || summary.ManualRequired != 0 {
				t.Fatalf("new automatic cycle %s was hidden by old authorization: summary=%+v err=%v", target.store, summary, err)
			}
			items, err := reader.ListOutboxCandidates(ctx, 501, 10)
			if err != nil || len(items) != 1 || items[0].Disposition != "automatic" || items[0].ActionRequestID != "" {
				t.Fatalf("new automatic cycle %s candidate is wrong: items=%+v err=%v", target.store, items, err)
			}
		}
	}
	if _, err := db.ExecContext(ctx, "DROP TABLE system_governance_action_runs"); err != nil {
		t.Fatal(err)
	}
	if _, err := buildResourceEventSubsystem(gormDB, mongoDB, nil, catalog, &fakePublisher{}, eventruntime.PublishModeMQ, nil, deps); err == nil || !strings.Contains(err.Error(), "M4 governance audit schema is incomplete") {
		t.Fatalf("missing governance audit did not fail before candidate startup: %v", err)
	}
	if _, err := db.ExecContext(ctx, string(auditDDL)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "DROP TABLE rm_outbox"); err != nil {
		t.Fatal(err)
	}
	if _, err := buildResourceEventSubsystem(gormDB, mongoDB, nil, catalog, &fakePublisher{}, eventruntime.PublishModeMQ, nil, deps); err == nil || !strings.Contains(err.Error(), "M4 MySQL standard schema is incomplete") {
		t.Fatalf("missing standard table did not fail before candidate startup: %v", err)
	}
}
