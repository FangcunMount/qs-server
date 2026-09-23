//go:build reliable_messaging_m4 && reliable_messaging_m4_integration

package process

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	appeventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	platformmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/platform"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	governanceinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	qsmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"github.com/gin-gonic/gin"
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
	auditIndexPath := os.Getenv("RM_QS_BOOTSTRAP_AUDIT_INDEX_MIGRATION")
	if auditIndexPath != "/tmp/m4-qs-bootstrap/mysql/000085_system_governance_pending_replay_index.up.sql" {
		t.Fatal("copied invocation-owned pending replay audit index migration required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
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
	auditIndexDDL, err := os.ReadFile(auditIndexPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(auditIndexDDL)); err != nil {
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
	nsqProxy := newM4NSQFaultProxy(t, os.Getenv("RM_QS_BOOTSTRAP_NSQ_ADDR"))
	cfg.MessagingOptions.NSQAddr = nsqProxy.Address()
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
	processServer := &server{config: cfg}
	resilience, err := processServer.buildResilienceSubsystem(nil)
	if err != nil {
		t.Fatal(err)
	}
	actionAuditStore, actionAuditRunner := buildActionAuditRuntime(gormDB, nil)
	hostOptions := processServer.buildContainerOptions(containerOptionsInput{
		eventSubsystem: subsystem, resilience: resilience,
		actionAuditStore: actionAuditStore, actionAuditRunner: actionAuditRunner,
	})
	hostOptions.Silent = true
	host := container.NewContainerWithOptions(gormDB, mongoDB, nil, hostOptions)
	hostClosed := false
	defer func() {
		if !hostClosed {
			_ = host.Cleanup()
		}
	}()
	initialized, err := bootstrapContainerStage(containerStageDeps{newContainer: func() *container.Container { return host }})
	if err != nil {
		t.Fatalf("initialize full apiserver container with selected standard profiles: %v", err)
	}
	if initialized.container != host || !host.IsInitialized() {
		t.Fatal("process bootstrap did not initialize the selected runtime container")
	}
	for _, name := range []string{"survey", "interpretation", "modelcatalog", "actor", "evaluation", "plan", "statistics"} {
		if !slices.Contains(host.GetLoadedModules(), name) {
			t.Fatalf("full container omitted %s module: %v", name, host.GetLoadedModules())
		}
	}
	if _, ok := subsystem.Profile(eventcatalog.OutboxProfileMongoDomain).Stager.(*mongostandard.Stager); !ok {
		t.Fatal("Mongo process profile kept the old writer")
	}
	if _, ok := subsystem.Profile(eventcatalog.OutboxProfileAssessmentMySQL).Stager.(*mysqlstandard.Stager); !ok {
		t.Fatal("MySQL process profile kept the old writer")
	}
	if err := host.StartEventSubsystem(ctx); err != nil {
		t.Fatal(err)
	}
	if err := host.HealthCheck(ctx); err != nil {
		t.Fatalf("initialized container health check: %v", err)
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
	// Force an actual TCP outage after startup: both selected relays must keep
	// their committed intents and recover by database scan when NSQ returns.
	nsqProxy.SetAvailable(false)
	outageEvents := []struct {
		id, eventType, aggregateType, aggregateID string
	}{
		{"m4-nsq-outage-mysql", "evaluation.requested", "Assessment", "102"},
		{"m4-nsq-outage-mongo", "answersheet.submitted", "AnswerSheet", "202"},
	}
	mysqlOutage := event.Event[map[string]any]{
		BaseEvent: event.BaseEvent{ID: outageEvents[0].id, EventTypeValue: outageEvents[0].eventType,
			AggregateTypeValue: outageEvents[0].aggregateType, AggregateIDValue: outageEvents[0].aggregateID, OccurredAtValue: time.Now()},
		Data: map[string]any{"org_id": 501},
	}
	if err := qsmysql.NewUnitOfWork(gormDB).WithinTransaction(ctx, func(txCtx context.Context) error {
		return binding.Stager.Stage(txCtx, mysqlOutage)
	}); err != nil {
		t.Fatal(err)
	}
	binding.PostCommit.AfterCommit(ctx, []event.DomainEvent{mysqlOutage}, time.Now())
	mongoOutage := event.Event[map[string]any]{
		BaseEvent: event.BaseEvent{ID: outageEvents[1].id, EventTypeValue: outageEvents[1].eventType,
			AggregateTypeValue: outageEvents[1].aggregateType, AggregateIDValue: outageEvents[1].aggregateID, OccurredAtValue: time.Now()},
		Data: map[string]any{"org_id": 501},
	}
	if _, err := session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (any, error) {
		return nil, mongoBinding.Stager.Stage(txCtx, mongoOutage)
	}); err != nil {
		t.Fatal(err)
	}
	mongoBinding.PostCommit.AfterCommit(ctx, []event.DomainEvent{mongoOutage}, time.Now())
	outageDeadline := time.Now().Add(25 * time.Second)
	for {
		var mysqlState string
		var mysqlFailures uint64
		if err := db.QueryRowContext(ctx, "SELECT state,failure_count FROM rm_outbox WHERE message_id=?", outageEvents[0].id).
			Scan(&mysqlState, &mysqlFailures); err != nil {
			t.Fatal(err)
		}
		var mongoState struct {
			State        string `bson:"state"`
			FailureCount uint64 `bson:"failure_count"`
		}
		if err := mongoDB.Collection("rm_outbox").FindOne(ctx, bson.M{"message_id": outageEvents[1].id}).Decode(&mongoState); err != nil {
			t.Fatal(err)
		}
		if mysqlState == "retry_wait" && mongoState.State == "retry_wait" && mysqlFailures > 0 && mongoState.FailureCount > 0 {
			break
		}
		if time.Now().After(outageDeadline) {
			t.Fatalf("both committed intents must remain retryable during NSQ outage: mysql=%s/%d mongo=%s/%d",
				mysqlState, mysqlFailures, mongoState.State, mongoState.FailureCount)
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Keep the broker unavailable while both host transactions commit a bounded
	// mixed-organization backlog. This exercises the running relays and NSQ
	// producer after recovery, rather than only the Store's direct ClaimDue API.
	const backlogPerStore = 64
	mysqlBacklog := make([]event.DomainEvent, 0, backlogPerStore)
	mongoBacklog := make([]event.DomainEvent, 0, backlogPerStore)
	for i := 0; i < backlogPerStore; i++ {
		orgID := 501
		if i%2 != 0 {
			orgID = 502
		}
		mysqlBacklog = append(mysqlBacklog, event.Event[map[string]any]{
			BaseEvent: event.BaseEvent{ID: fmt.Sprintf("m4-backlog-mysql-%03d", i),
				EventTypeValue: "evaluation.requested", AggregateTypeValue: "Assessment",
				AggregateIDValue: fmt.Sprintf("%d", 1000+i), OccurredAtValue: time.Now()},
			Data: map[string]any{"org_id": orgID},
		})
		mongoBacklog = append(mongoBacklog, event.Event[map[string]any]{
			BaseEvent: event.BaseEvent{ID: fmt.Sprintf("m4-backlog-mongo-%03d", i),
				EventTypeValue: "answersheet.submitted", AggregateTypeValue: "AnswerSheet",
				AggregateIDValue: fmt.Sprintf("%d", 2000+i), OccurredAtValue: time.Now()},
			Data: map[string]any{"org_id": orgID},
		})
	}
	if err := qsmysql.NewUnitOfWork(gormDB).WithinTransaction(ctx, func(txCtx context.Context) error {
		return binding.Stager.Stage(txCtx, mysqlBacklog...)
	}); err != nil {
		t.Fatal(err)
	}
	binding.PostCommit.AfterCommit(ctx, mysqlBacklog, time.Now())
	if _, err := session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (any, error) {
		return nil, mongoBinding.Stager.Stage(txCtx, mongoBacklog...)
	}); err != nil {
		t.Fatal(err)
	}
	mongoBinding.PostCommit.AfterCommit(ctx, mongoBacklog, time.Now())
	var mysqlCommitted, mysqlPremature int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*),SUM(state='published') FROM rm_outbox WHERE message_id LIKE 'm4-backlog-mysql-%'`).
		Scan(&mysqlCommitted, &mysqlPremature); err != nil {
		t.Fatal(err)
	}
	mongoCommitted, err := mongoDB.Collection("rm_outbox").CountDocuments(ctx, bson.M{"message_id": bson.M{"$regex": "^m4-backlog-mongo-"}})
	if err != nil {
		t.Fatal(err)
	}
	mongoPremature, err := mongoDB.Collection("rm_outbox").CountDocuments(ctx, bson.M{"message_id": bson.M{"$regex": "^m4-backlog-mongo-"}, "state": "published"})
	if err != nil {
		t.Fatal(err)
	}
	if mysqlCommitted != backlogPerStore || mongoCommitted != backlogPerStore || mysqlPremature != 0 || mongoPremature != 0 {
		t.Fatalf("backlog before NSQ recovery: mysql=%d published=%d mongo=%d published=%d",
			mysqlCommitted, mysqlPremature, mongoCommitted, mongoPremature)
	}
	recoveryStarted := time.Now()
	nsqProxy.SetAvailable(true)
	recoveryDeadline := time.Now().Add(90 * time.Second)
	for {
		var mysqlState string
		if err := db.QueryRowContext(ctx, "SELECT state FROM rm_outbox WHERE message_id=?", outageEvents[0].id).Scan(&mysqlState); err != nil {
			t.Fatal(err)
		}
		var mongoState struct {
			State string `bson:"state"`
		}
		if err := mongoDB.Collection("rm_outbox").FindOne(ctx, bson.M{"message_id": outageEvents[1].id}).Decode(&mongoState); err != nil {
			t.Fatal(err)
		}
		var mysqlPublished int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rm_outbox WHERE message_id LIKE 'm4-backlog-mysql-%' AND state='published'`).
			Scan(&mysqlPublished); err != nil {
			t.Fatal(err)
		}
		mongoPublished, err := mongoDB.Collection("rm_outbox").CountDocuments(ctx, bson.M{"message_id": bson.M{"$regex": "^m4-backlog-mongo-"}, "state": "published"})
		if err != nil {
			t.Fatal(err)
		}
		if mysqlState == "published" && mongoState.State == "published" && mysqlPublished == backlogPerStore && mongoPublished == backlogPerStore {
			t.Logf("both standard profiles recovered after NSQ TCP outage in %s; backlog published mysql=%d mongo=%d",
				time.Since(recoveryStarted), mysqlPublished, mongoPublished)
			break
		}
		if time.Now().After(recoveryDeadline) {
			t.Fatalf("standard profiles did not recover after NSQ returned: mysql=%s mongo=%s backlog published mysql=%d mongo=%d",
				mysqlState, mongoState.State, mysqlPublished, mongoPublished)
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Register the production REST router against the initialized container's
	// real status service. This isolated request supplies a trusted authorization
	// snapshot; IAM token verification itself is outside this proof.
	for _, path := range []string{"/tmp/m4-qs-bootstrap/api/rest", "/tmp/m4-qs-bootstrap/web/swagger-ui/swagger-ui-dist"} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("/tmp/m4-qs-bootstrap"); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)
	gin.SetMode(gin.TestMode)
	httpEngine := gin.New()
	httpEngine.Use(func(c *gin.Context) {
		snapshot := &appauthz.Snapshot{Permissions: []appauthz.Permission{{
			Resource: "qs:*:*:*", Action: "*", Mode: appauthz.AuthorizationModeUnconditional,
		}}}
		c.Set(restmiddleware.AuthzSnapshotKey, snapshot)
		c.Set(restmiddleware.UserIDKey, uint64(110004))
		c.Set(restmiddleware.OrgIDKey, uint64(501))
		requestCtx := appauthz.WithSnapshot(c.Request.Context(), snapshot)
		requestCtx = actorctx.WithGrantingUserID(requestCtx, 110004)
		c.Request = c.Request.WithContext(requestCtx)
		c.Next()
	})
	resttransport.NewRouter(host.BuildRESTDeps(cfg.RateLimit)).RegisterRoutes(httpEngine)
	lostReplyResults := make(chan int, 2)
	tcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-M4-Drop-Reply") != "1" {
			httpEngine.ServeHTTP(w, request)
			return
		}
		// Complete the real route and durable approval, then close the TCP
		// connection before returning any response bytes to the caller.
		captured := httptest.NewRecorder()
		httpEngine.ServeHTTP(captured, request)
		lostReplyResults <- captured.Code
		connection, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = connection.Close()
		}
	}))
	defer tcpServer.Close()
	response := httptest.NewRecorder()
	httpEngine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/internal/v1/events/status", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("container-backed event HTTP status = %d, body=%s", response.Code, response.Body.String())
	}
	var httpStatus appeventing.StatusSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &httpStatus); err != nil {
		t.Fatal(err)
	}
	sdkProfiles, liveOutboxes := 0, 0
	for _, profile := range httpStatus.Profiles {
		if profile.RelayKind == "sdk" && profile.Running && profile.ScanHealthy != nil && *profile.ScanHealthy {
			sdkProfiles++
		}
	}
	for _, outbox := range httpStatus.Outboxes {
		if !outbox.Degraded && len(outbox.Buckets) == 4 {
			liveOutboxes++
		}
	}
	if sdkProfiles != 2 || liveOutboxes != 2 {
		t.Fatalf("HTTP status did not expose both standard profiles: profiles=%+v outboxes=%+v", httpStatus.Profiles, httpStatus.Outboxes)
	}
	untrustedEngine := gin.New()
	resttransport.NewRouter(host.BuildRESTDeps(cfg.RateLimit)).RegisterRoutes(untrustedEngine)
	untrustedResponse := httptest.NewRecorder()
	untrustedEngine.ServeHTTP(untrustedResponse, httptest.NewRequest(http.MethodGet, "/internal/v1/events/status", nil))
	if untrustedResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("event status without IAM runtime or trusted identity = %d, want 503", untrustedResponse.Code)
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
		body, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		runHTTP := func(payload []byte) *httptest.ResponseRecorder {
			httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost,
				tcpServer.URL+"/internal/v1/system-governance/actions/events.replay_pending/runs", bytes.NewReader(payload))
			if err != nil {
				t.Fatal(err)
			}
			httpRequest.Header.Set("Content-Type", "application/json")
			tcpResponse, err := (&http.Client{Timeout: 5 * time.Second}).Do(httpRequest)
			if err != nil {
				t.Fatal(err)
			}
			defer tcpResponse.Body.Close()
			responseBody, err := io.ReadAll(tcpResponse.Body)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			response.Code = tcpResponse.StatusCode
			_, _ = response.Body.Write(responseBody)
			return response
		}
		lostReplyRequest, err := http.NewRequestWithContext(ctx, http.MethodPost,
			tcpServer.URL+"/internal/v1/system-governance/actions/events.replay_pending/runs", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		lostReplyRequest.Header.Set("Content-Type", "application/json")
		lostReplyRequest.Header.Set("X-M4-Drop-Reply", "1")
		lostReplyResponse, err := (&http.Client{Timeout: 5 * time.Second}).Do(lostReplyRequest)
		if lostReplyResponse != nil {
			_ = lostReplyResponse.Body.Close()
		}
		if err == nil {
			t.Fatalf("standard profile %s unexpectedly received first approval reply", target.store)
		}
		select {
		case status := <-lostReplyResults:
			if status != http.StatusOK {
				t.Fatalf("standard profile %s lost reply after failed approval status %d", target.store, status)
			}
		case <-ctx.Done():
			t.Fatalf("standard profile %s lost-reply handler did not finish: %v", target.store, ctx.Err())
		}
		httpApproval := runHTTP(body)
		if httpApproval.Code != http.StatusOK {
			t.Fatalf("standard profile %s HTTP replay = %d, body=%s", target.store, httpApproval.Code, httpApproval.Body.String())
		}
		var firstHTTP struct {
			Data systemgov.ActionRunResult `json:"data"`
		}
		if err := json.Unmarshal(httpApproval.Body.Bytes(), &firstHTTP); err != nil || firstHTTP.Data.RequestID != request.RequestID ||
			fmt.Sprint(firstHTTP.Data.Result["authorized"]) != "1" {
			t.Fatalf("standard profile %s HTTP replay returned unexpected result: result=%+v err=%v", target.store, firstHTTP.Data, err)
		}
		approved, err := governance.RunAction(actionCtx, 501, "events.replay_pending", request)
		if err != nil || approved == nil || fmt.Sprint(approved.Result["authorized"]) != "1" {
			t.Fatalf("standard profile %s is not governable: result=%+v err=%v", target.store, approved, err)
		}
		for _, outbox := range subsystem.Outboxes() {
			if outbox.Name != target.store {
				continue
			}
			reader := outbox.Reader.(systemgov.OutboxGovernanceReader)
			summary, err := reader.ReadOutboxGovernance(ctx, 501)
			// The live Relay may already have claimed or published the approved
			// message. Authorized counts only the still-waiting state.
			if err != nil || summary.Authorized > 1 || summary.Automatic != 0 || summary.ManualRequired != 0 {
				t.Fatalf("standard replay %s was misclassified after authorization: summary=%+v err=%v", target.store, summary, err)
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
		httpPrior := runHTTP(body)
		var priorHTTP struct {
			Data systemgov.ActionRunResult `json:"data"`
		}
		if err := json.Unmarshal(httpPrior.Body.Bytes(), &priorHTTP); err != nil || httpPrior.Code != http.StatusOK ||
			priorHTTP.Data.RequestID != firstHTTP.Data.RequestID ||
			fmt.Sprint(priorHTTP.Data.Result["authorized"]) != "1" {
			t.Fatalf("standard replay %s HTTP retry changed original result: first=%s second=%s err=%v", target.store, httpApproval.Body.String(), httpPrior.Body.String(), err)
		}
		changed := request
		changed.Input = map[string]interface{}{
			"store": target.store, "reason": "changed approval",
			"targets": []interface{}{map[string]interface{}{"event_id": target.eventID, "expected_attempt_count": 30}},
		}
		if _, err := governance.RunAction(actionCtx, 501, "events.replay_pending", changed); err == nil {
			t.Fatalf("standard replay %s accepted changed approval input", target.store)
		}
		changedBody, err := json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		if response := runHTTP(changedBody); response.Code == http.StatusOK {
			t.Fatalf("standard replay %s HTTP accepted changed approval input: %s", target.store, response.Body.String())
		}
		var auditStatus string
		if err := db.QueryRowContext(ctx, `SELECT status FROM system_governance_action_runs WHERE org_id=501 AND request_id=?`,
			request.RequestID).Scan(&auditStatus); err != nil || auditStatus != "ok" {
			t.Fatalf("standard replay %s lacks completed audit: status=%s err=%v", target.store, auditStatus, err)
		}
		var grantRows int64
		if target.store == "assessment-mysql-outbox" {
			err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM qs_rm_replay_requests WHERE org_id=501 AND request_id=?`, request.RequestID).Scan(&grantRows)
		} else {
			grantRows, err = mongoDB.Collection("qs_rm_replay_requests").CountDocuments(ctx,
				bson.M{"org_id": int64(501), "request_id": request.RequestID})
		}
		if err != nil || grantRows != 1 {
			t.Fatalf("standard replay %s has %d durable grants for one request: %v", target.store, grantRows, err)
		}
	}
	if err := db.QueryRowContext(ctx, `SELECT version FROM rm_outbox WHERE message_id='m4-process-probe'`).Scan(&mysqlVersion); err != nil ||
		mysqlVersion < originalMySQLVersion+1 || mysqlVersion > originalMySQLVersion+3 {
		t.Fatalf("MySQL standard replay advanced unexpected number of times: version=%d err=%v", mysqlVersion, err)
	}
	if err := mongoDB.Collection("rm_outbox").FindOne(ctx, bson.M{"message_id": "m4-process-mongo-probe"}).Decode(&mongoVersion); err != nil ||
		mongoVersion.Version < originalMongoVersion.Version+1 || mongoVersion.Version > originalMongoVersion.Version+3 {
		t.Fatalf("Mongo standard replay advanced unexpected number of times: version=%d err=%v", mongoVersion.Version, err)
	}
	if err := host.Cleanup(); err != nil {
		t.Fatal(err)
	}
	hostClosed = true
	if host.IsInitialized() {
		t.Fatal("container still reports initialized after cleanup")
	}
	stopped, err := subsystem.StatusService().GetStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range stopped.Profiles {
		if profile.RelayKind == "sdk" && (profile.Running || profile.ScanHealthy == nil || *profile.ScanHealthy) {
			t.Fatalf("stopped standard profile reports healthy: %+v", profile)
		}
	}
	// Execute the real governance action in a child OS process. Its test-only
	// audit hook blocks after durable authorization, so killing that process
	// leaves a genuine committed grant and a still-running MySQL audit.
	for _, target := range []struct {
		store, eventID, requestID string
		quarantine                func() error
		version                   func() (uint64, error)
		ledgerCount               func() (int64, error)
	}{
		{"assessment-mysql-outbox", "m4-process-probe", "crash-mysql-process",
			func() error {
				_, err := db.ExecContext(ctx, `UPDATE rm_outbox SET state='quarantined',last_error_code='publish_unknown',failure_count=31 WHERE message_id='m4-process-probe'`)
				return err
			},
			func() (uint64, error) {
				var version uint64
				err := db.QueryRowContext(ctx, `SELECT version FROM rm_outbox WHERE message_id='m4-process-probe'`).Scan(&version)
				return version, err
			},
			func() (int64, error) {
				var count int64
				err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM qs_rm_replay_requests WHERE org_id=501 AND request_id='crash-mysql-process'`).Scan(&count)
				return count, err
			}},
		{"mongo-domain-events", "m4-process-mongo-probe", "crash-mongo-process",
			func() error {
				_, err := mongoDB.Collection("rm_outbox").UpdateOne(ctx, bson.M{"message_id": "m4-process-mongo-probe"},
					bson.M{"$set": bson.M{"state": "quarantined", "last_error_code": "publish_unknown", "failure_count": int64(31)}})
				return err
			},
			func() (uint64, error) {
				var row struct {
					Version uint64 `bson:"version"`
				}
				err := mongoDB.Collection("rm_outbox").FindOne(ctx, bson.M{"message_id": "m4-process-mongo-probe"}).Decode(&row)
				return row.Version, err
			},
			func() (int64, error) {
				return mongoDB.Collection("qs_rm_replay_requests").CountDocuments(ctx, bson.M{"org_id": int64(501), "request_id": "crash-mongo-process"})
			}},
	} {
		if err := target.quarantine(); err != nil {
			t.Fatal(err)
		}
		beforeCrash, err := target.version()
		if err != nil {
			t.Fatalf("read %s version before crash: %v", target.store, err)
		}
		runM4ReplayCrashChild(t, ctx, target.store, target.eventID, target.requestID)
		var auditStatus string
		if err := db.QueryRowContext(ctx, `SELECT status FROM system_governance_action_runs WHERE org_id=501 AND request_id=?`, target.requestID).
			Scan(&auditStatus); err != nil || auditStatus != "running" {
			t.Fatalf("killed %s process did not leave running audit: status=%s err=%v", target.store, auditStatus, err)
		}
		beforeRecovery, err := target.version()
		if err != nil || beforeRecovery != beforeCrash+1 {
			t.Fatalf("killed %s process did not commit exactly one grant: before=%d after=%d err=%v", target.store, beforeCrash, beforeRecovery, err)
		}
		if count, err := target.ledgerCount(); err != nil || count != 1 {
			t.Fatalf("killed %s process lacks one durable replay request: count=%d err=%v", target.store, count, err)
		}
		runM4ReplayRecoveryChild(t, ctx, target.store, target.eventID, target.requestID)
		if err := db.QueryRowContext(ctx, `SELECT status FROM system_governance_action_runs WHERE org_id=501 AND request_id=?`, target.requestID).
			Scan(&auditStatus); err != nil || auditStatus != "ok" {
			t.Fatalf("new %s process did not reconcile original audit: status=%s err=%v", target.store, auditStatus, err)
		}
		afterRecovery, err := target.version()
		if err != nil || afterRecovery != beforeRecovery {
			t.Fatalf("new %s process authorized twice: before=%d after=%d err=%v", target.store, beforeRecovery, afterRecovery, err)
		}
	}
	auditStore := governanceinfra.NewActionAuditStore(gormDB)
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
		pendingRequest, err := http.NewRequestWithContext(ctx, http.MethodGet,
			tcpServer.URL+"/internal/v1/system-governance/actions/pending-reconciliations?limit=10", nil)
		if err != nil {
			t.Fatal(err)
		}
		pendingResponse, err := (&http.Client{Timeout: 5 * time.Second}).Do(pendingRequest)
		if err != nil {
			t.Fatal(err)
		}
		var pendingPayload struct {
			Data systemgov.PendingReplayAuditPage `json:"data"`
		}
		decodeErr := json.NewDecoder(pendingResponse.Body).Decode(&pendingPayload)
		_ = pendingResponse.Body.Close()
		if decodeErr != nil || pendingResponse.StatusCode != http.StatusOK || len(pendingPayload.Data.Items) != 1 ||
			pendingPayload.Data.Items[0].RequestID != audit.RequestID ||
			pendingPayload.Data.Items[0].Store != target.store || pendingPayload.Data.Items[0].ActorUserID != "110004" {
			t.Fatalf("pending %s audit absent from tenant-scoped HTTP list: status=%d page=%+v err=%v",
				target.store, pendingResponse.StatusCode, pendingPayload.Data, decodeErr)
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
