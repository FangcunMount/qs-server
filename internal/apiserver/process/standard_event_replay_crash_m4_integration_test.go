//go:build reliable_messaging_m4 && reliable_messaging_m4_integration

package process

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	platformmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/platform"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	mysqldriver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type m4CrashBeforeAuditComplete struct {
	systemgov.ActionAuditStore
	marker string
}

func (s m4CrashBeforeAuditComplete) Complete(_ context.Context, _ systemgov.ActionAuditRecord) error {
	if err := os.WriteFile(s.marker, []byte("authorization committed"), 0o600); err != nil {
		return err
	}
	select {} // The parent kills this child after observing the committed grant.
}

// The parent invokes this test in an independent OS process against the same
// disposable databases. In crash mode it executes the real action until the
// Outbox authorization commits, then waits before the MySQL audit completes.
func TestM4ReplayCrashChild(t *testing.T) {
	mode := os.Getenv("RM_QS_REPLAY_CHILD_MODE")
	if mode == "" {
		t.Skip("invoked only by the disposable M4 process integration test")
	}
	if mode != "crash" && mode != "recover" {
		t.Fatalf("unknown replay child mode %q", mode)
	}
	dsn := os.Getenv("RM_QS_BOOTSTRAP_MYSQL_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Addr != "mysql:3306" || parsed.DBName != "m4_qs_bootstrap" {
		t.Fatal("disposable MySQL is required for replay crash child")
	}
	uri := os.Getenv("RM_QS_BOOTSTRAP_MONGO_URI")
	if !strings.HasPrefix(uri, "mongodb://mongo:27017/") || !strings.Contains(uri, "replicaSet=rm-test") {
		t.Fatal("disposable Mongo replica set is required for replay crash child")
	}
	store := os.Getenv("RM_QS_REPLAY_CHILD_STORE")
	eventID := os.Getenv("RM_QS_REPLAY_CHILD_EVENT_ID")
	requestID := os.Getenv("RM_QS_REPLAY_CHILD_REQUEST_ID")
	if (store != "assessment-mysql-outbox" && store != "mongo-domain-events") || eventID == "" || requestID == "" {
		t.Fatal("replay crash child requires a selected standard profile and stable identities")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
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
	wire, err := eventcatalog.Load(os.Getenv("RM_QS_BOOTSTRAP_CATALOG"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Options: options.NewOptions()}
	cfg.MessagingOptions.Enabled = true
	cfg.MessagingOptions.Provider = "nsq"
	cfg.MessagingOptions.NSQAddr = "nsqd:4150"
	cfg.Eventing.StandardOutbox.Mongo = true
	cfg.Eventing.StandardOutbox.Assessment = true
	deps := (&server{config: cfg}).buildEventSubsystemResourceDeps()
	deps.buildSubscriberFactory = nil
	deps.consumers = map[string]eventsubsystem.ConsumerOptions{"modelcatalog.hot_rank_projection": {Enabled: false}}
	subsystem, err := buildResourceEventSubsystem(gormDB, mongoDB, nil, eventcatalog.NewCatalog(wire),
		&fakePublisher{}, eventruntime.PublishModeMQ, nil, deps)
	if err != nil {
		t.Fatal(err)
	}
	defer subsystem.Close()
	auditStore, _ := buildActionAuditRuntime(gormDB, nil)
	if mode == "crash" {
		marker := os.Getenv("RM_QS_REPLAY_CHILD_MARKER")
		if !strings.HasPrefix(marker, "/tmp/m4-qs-bootstrap/") {
			t.Fatal("replay crash marker must be invocation-owned")
		}
		auditStore = m4CrashBeforeAuditComplete{ActionAuditStore: auditStore, marker: marker}
	}
	governance := platformmod.BuildRESTSystemGovernanceFacade(platformmod.RESTSystemGovernanceInput{
		EventOutboxes: subsystem.Outboxes(), MySQLDB: gormDB, MongoDB: mongoDB, ActionAuditStore: auditStore,
	})
	actionCtx := actorctx.WithGrantingUserID(ctx, 110004)
	result, err := governance.RunAction(actionCtx, 501, "events.replay_pending", systemgov.ActionRunRequest{
		RequestID: requestID, Confirm: true,
		Input: map[string]interface{}{
			"store": store, "reason": "recover after real process kill",
			"targets": []interface{}{map[string]interface{}{"event_id": eventID, "expected_attempt_count": 31}},
		},
	})
	if mode == "crash" {
		t.Fatalf("crash child unexpectedly returned from audit completion: result=%+v err=%v", result, err)
	}
	if err != nil || result == nil || fmt.Sprint(result.Result["authorized"]) != "1" {
		t.Fatalf("new process did not recover committed authorization: result=%+v err=%v", result, err)
	}
}

func runM4ReplayCrashChild(t *testing.T, ctx context.Context, store, eventID, requestID string) {
	t.Helper()
	marker := "/tmp/m4-qs-bootstrap/replay-crash-" + eventID
	_ = os.Remove(marker)
	defer os.Remove(marker)
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestM4ReplayCrashChild$", "-test.v")
	child.Env = append(os.Environ(),
		"RM_QS_REPLAY_CHILD_MODE=crash", "RM_QS_REPLAY_CHILD_STORE="+store,
		"RM_QS_REPLAY_CHILD_EVENT_ID="+eventID, "RM_QS_REPLAY_CHILD_REQUEST_ID="+requestID,
		"RM_QS_REPLAY_CHILD_MARKER="+marker)
	var output bytes.Buffer
	child.Stdout, child.Stderr = &output, &output
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- child.Wait() }()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if contents, err := os.ReadFile(marker); err == nil && string(contents) == "authorization committed" {
			break
		}
		select {
		case err := <-finished:
			t.Fatalf("crash child exited before authorization barrier: %v\n%s", err, output.String())
		case <-ctx.Done():
			t.Fatalf("crash child did not reach committed authorization: %v", ctx.Err())
		case <-ticker.C:
		}
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err == nil {
		t.Fatalf("crash child exited successfully instead of being killed: %s", output.String())
	}
}

func runM4ReplayRecoveryChild(t *testing.T, ctx context.Context, store, eventID, requestID string) {
	t.Helper()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestM4ReplayCrashChild$", "-test.v")
	child.Env = append(os.Environ(),
		"RM_QS_REPLAY_CHILD_MODE=recover", "RM_QS_REPLAY_CHILD_STORE="+store,
		"RM_QS_REPLAY_CHILD_EVENT_ID="+eventID, "RM_QS_REPLAY_CHILD_REQUEST_ID="+requestID)
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("new process failed to reconcile prior authorization: %v\n%s", err, output)
	}
}
