//go:build integration && m3_host_runtime

package runtimeclosure

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	answersheetpb "github.com/FangcunMount/qs-server/api/grpc/gen/answersheet"
	evaluationpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	internalpb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	testeeapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	intakejourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/assessmentintake"
	apphot "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/hotrank"
	notification "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	govapp "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	operatorDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/answersheetgap"
	redishot "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog/hotrank"
	mysqlActor "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	mysqlboundary "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	ledger "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/notification"
	planrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	gov "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/taskopen"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	wechat "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
	resiliencesubsystem "github.com/FangcunMount/qs-server/internal/apiserver/resilience/subsystem"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	migrationpkg "github.com/FangcunMount/qs-server/internal/pkg/migration"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	generic "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"
	workeroptions "github.com/FangcunMount/qs-server/internal/worker/options"
	workerprocess "github.com/FangcunMount/qs-server/internal/worker/process"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The child enters the actual production Worker process root. Its companion
// uses the production hot-rank handler on the real API consumer group. No
// custom business dispatcher or fake MQ is substituted for the Worker.
type m3cChildConfig struct {
	DSN, MongoURI, MongoDB, Redis, Namespace, Endpoint, Cert, Key, Observer string
	Marker                                                                  string
	Hold                                                                    bool
}

type m3cDeliveryMarker struct{ EventID, WireSHA string }

func m3cLoadMQ(t *testing.T, path string) generic.MessagingOptions {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var o generic.MessagingOptions
	require.NoError(t, json.Unmarshal(raw, &o))
	require.NotNil(t, o.RocketMQ)
	host, _, err := net.SplitHostPort(o.RocketMQ.Endpoint)
	require.NoError(t, err)
	require.True(t, net.ParseIP(host).IsLoopback())
	return o
}

func TestM3RecoveryWorkerProcess(t *testing.T) {
	path := os.Getenv("QS_M3C_WORKER_CHILD")
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var cfg m3cChildConfig
	require.NoError(t, json.Unmarshal(raw, &cfg))
	dsn, err := driver.ParseDSN(cfg.DSN)
	require.NoError(t, err)
	host, _, err := net.SplitHostPort(dsn.Addr)
	require.NoError(t, err)
	require.True(t, net.ParseIP(host).IsLoopback())
	rh, rp, err := net.SplitHostPort(cfg.Redis)
	require.NoError(t, err)
	require.True(t, net.ParseIP(rh).IsLoopback())
	port, err := strconv.Atoi(rp)
	require.NoError(t, err)
	opts := workeroptions.NewOptions()
	opts.Metrics.Enable = false
	opts.Worker.Concurrency = 4
	opts.Worker.ServiceName = "qs-worker"
	opts.Worker.EventConfigPath = "../../../../configs/events.yaml"
	opts.MySQL.Host = dsn.Addr
	opts.MySQL.Username = dsn.User
	opts.MySQL.Password = dsn.Passwd
	opts.MySQL.Database = dsn.DBName
	opts.MySQL.Location = "Asia/Shanghai"
	opts.MySQL.SessionTimeZone = "+08:00"
	opts.MongoDB.URL = cfg.MongoURI
	opts.MongoDB.Database = cfg.MongoDB
	opts.MongoDB.MaxPoolSize = 4
	opts.Redis.Host = rh
	opts.Redis.Port = port
	opts.Redis.Database = 1
	opts.RedisProfiles = map[string]*generic.RedisOptions{}
	for name, db := range map[string]int{"lock_cache": 1, "ops_runtime": 2} {
		o := *opts.Redis
		o.Database = db
		opts.RedisProfiles[name] = &o
	}
	mq := m3cLoadMQ(t, cfg.Observer)
	opts.Messaging.Provider = "rocketmq"
	opts.Messaging.RocketMQ = mq.RocketMQ
	opts.RetryGovernance.AutomaticRetryEnabled = true
	opts.GRPC.ApiserverAddr = cfg.Endpoint
	opts.GRPC.Insecure = false
	opts.GRPC.TLSCAFile = cfg.Cert
	opts.GRPC.TLSCertFile = cfg.Cert
	opts.GRPC.TLSKeyFile = cfg.Key
	opts.GRPC.TLSServerName = "127.0.0.1"
	opts.GRPC.RequestTimeout = 2 * time.Minute
	client := redis.NewClient(&redis.Options{Addr: cfg.Redis})
	t.Cleanup(func() { _ = client.Close() })
	projection := redishot.NewRedisScaleHotRankProjection(client, keyspace.NewBuilderWithNamespace(cfg.Namespace))
	consume := apphot.NewEventConsumer(projection)
	sc, err := mq.RocketMQ.Load([]string{"qs.evaluation.lifecycle"}, 2, 5*time.Second)
	require.NoError(t, err)
	sub, err := rocketmq.NewSubscriber(rocketmq.SubscriberConfig{Config: sc, MaxAttempts: 16, InvisibleDuration: 30 * time.Second, HandlerTimeout: time.Minute, RetryDelay: time.Second})
	require.NoError(t, err)
	fmt.Println("M3C_WORKER_PHASE companion_subscribe_start")
	require.NoError(t, sub.Subscribe(t.Context(), "qs.evaluation.lifecycle", "qs-apiserver-modelcatalog-hot-rank-v1", func(ctx context.Context, d transport.Delivery) error {
		m := d.Message()
		envelope, err := domainwire.DecodeEnvelope(m.Payload)
		if err != nil {
			return err
		}
		if envelope.EventType != eventcatalog.AnswerSheetSubmitted {
			return d.Ack()
		}
		if e := consume(ctx, envelope.EventType, m.Payload); e != nil {
			return e
		}
		hash := sha256.Sum256(m.Wire)
		marker := m3cDeliveryMarker{m.ID, hex.EncodeToString(hash[:])}
		b, e := json.Marshal(marker)
		if e != nil {
			return e
		}
		if e = os.WriteFile(cfg.Marker, b, 0600); e != nil {
			return e
		}
		if cfg.Hold {
			<-ctx.Done()
			return ctx.Err()
		}
		return d.Ack()
	}, func(context.Context, transport.Failure) error { return errors.New("owned hot-rank delivery failed") }))
	fmt.Println("M3C_WORKER_PHASE production_worker_start")
	require.NoError(t, workerprocess.Run(&workerconfig.Config{Options: opts}))
	fmt.Println("M3C_WORKER_PHASE production_worker_stopped")
	cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, sub.Close(cleanup))
}

// This is the same narrow trusted read-model mapping used by the production
// reminder consumer; an operator-facing query requires an operator snapshot.
type m3cTesteeLookup struct {
	get func(context.Context, uint64) (*actorreadmodel.TesteeRow, error)
}

func (r m3cTesteeLookup) GetByID(ctx context.Context, id uint64) (*testeeapp.TesteeResult, error) {
	row, err := r.get(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil || row.ID != id {
		return nil, errors.New("original testee unavailable")
	}
	return &testeeapp.TesteeResult{ID: row.ID, OrgID: row.OrgID, ProfileID: row.ProfileID, Source: row.Source}, nil
}

type m3cIdentity struct{ changed atomic.Bool }

func (r *m3cIdentity) ListLinkedUsers(context.Context, string) ([]iambridge.MiniProgramLinkedUser, error) {
	users := []iambridge.MiniProgramLinkedUser{{UserID: "owned-user", Relations: []string{"self"}}}
	if r.changed.Load() {
		users = append(users, iambridge.MiniProgramLinkedUser{UserID: "new-current-user", Relations: []string{"self"}})
	}
	return users, nil
}
func (*m3cIdentity) ReadCandidates(_ context.Context, _, app string, users []string) ([]iambridge.MiniProgramRecipientCandidate, error) {
	out := []iambridge.MiniProgramRecipientCandidate{}
	for _, id := range users {
		out = append(out, iambridge.MiniProgramRecipientCandidate{UserID: id, LoginIdentityID: id + "-identity", AppID: app, OpenID: id + "-openid", Relations: []string{"self"}})
	}
	return out, nil
}

type m3cProvider struct{ url string }

func (p m3cProvider) ListTemplates(context.Context, string, string) ([]wechat.SubscribeTemplate, error) {
	return []wechat.SubscribeTemplate{{ID: "owned-template", Content: "{{thing5.DATA}}{{date1.DATA}}{{character_string2.DATA}}{{thing3.DATA}}"}}, nil
}
func (p m3cProvider) SendSubscribeMessage(ctx context.Context, a, b string, m wechat.SubscribeMessage) error {
	_, e := p.SendSubscribeMessageWithReceipt(ctx, a, b, m)
	return e
}
func (p m3cProvider) SendSubscribeMessageWithReceipt(ctx context.Context, _, _ string, m wechat.SubscribeMessage) (wechat.SubscribeSendReceipt, error) {
	raw, e := json.Marshal(m)
	if e != nil {
		return wechat.SubscribeSendReceipt{}, e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(raw))
	if e != nil {
		return wechat.SubscribeSendReceipt{}, e
	}
	response, e := http.DefaultClient.Do(req)
	if e != nil {
		return wechat.SubscribeSendReceipt{}, e
	}
	defer func() { _ = response.Body.Close() }()
	var result wechat.SubscribeSendReceipt
	e = json.NewDecoder(response.Body).Decode(&result)
	return result, e
}

type m3cFreeze struct {
	notification.ReminderBatchLedger
	fail atomic.Bool
}

func (f *m3cFreeze) FreezeRecipients(ctx context.Context, key notification.ReminderBatchKey, app, template string, recipients []notification.ReminderRecipientIdentity, at time.Time) (notification.ReminderBatch, error) {
	batch, err := f.ReminderBatchLedger.FreezeRecipients(ctx, key, app, template, recipients, at)
	if err == nil && f.fail.Swap(false) {
		return batch, errors.New("owned interruption immediately after durable freeze")
	}
	return batch, err
}

type m3cReminder struct {
	mu      sync.RWMutex
	service notification.TaskOpenedReminderService
}

func (s *m3cReminder) set(v notification.TaskOpenedReminderService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.service = v
}
func (s *m3cReminder) ProcessTaskOpenedReminder(ctx context.Context, r notification.TaskOpenedReminderRequest) error {
	s.mu.RLock()
	v := s.service
	s.mu.RUnlock()
	return v.ProcessTaskOpenedReminder(ctx, r)
}

func m3cPrivateJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0600))
}
func m3cEventually(t *testing.T, fn func() bool) {
	t.Helper()
	require.Eventually(t, fn, 2*time.Minute, 200*time.Millisecond)
}

func m3cBroker(t *testing.T, action string) string {
	t.Helper()
	broker, project := os.Getenv("QS_M3_HOST_BROKER"), os.Getenv("QS_M3_FIXTURE_PROJECT")
	require.True(t, strings.HasPrefix(project, "rocketmq-m2-"))
	require.True(t, strings.HasPrefix(broker, project+"-"))
	invoke := func(args ...string) string {
		ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
		defer cancel()
		raw, err := exec.CommandContext(ctx, "docker", append([]string{"--host=unix:///var/run/docker.sock"}, args...)...).Output()
		require.NoError(t, err)
		return strings.TrimSpace(string(raw))
	}
	require.Equal(t, project, invoke("inspect", "--format", `{{index .Config.Labels "com.docker.compose.project"}}`, broker))
	switch action {
	case "id":
		return invoke("inspect", "--format", "{{.Id}}", broker)
	case "exit":
		return invoke("inspect", "--format", "{{.State.ExitCode}}", broker)
	case "health":
		return invoke("inspect", "--format", "{{.State.Health.Status}}", broker)
	case "topics":
		return invoke("exec", broker, "java", "-Xms32m", "-Xmx128m", "-XX:MaxDirectMemorySize=32m", "-cp", "/opt/rocketmq/conf:/opt/rocketmq/lib/*", "org.apache.rocketmq.tools.command.MQAdminStartup", "topicList", "-n", "nameserver:9876")
	case "kill":
		return invoke("kill", "--signal=KILL", broker)
	case "start":
		return invoke("start", broker)
	}
	t.Fatal("unsupported owned action")
	return ""
}

// Export only equality/status diagnostics, never private artifacts or links.
func m3cOpeningDiagnostic(t *testing.T, db *gorm.DB, owner *mysqlboundary.Committer, org int64, id string, index int) {
	t.Helper()
	values := map[string]any{"index": index}
	var current planrepo.AssessmentTaskPO
	currentErr := db.Table("assessment_task").Where("id=?", id).Take(&current).Error
	values["current_read"] = currentErr == nil
	r, err := owner.Find(t.Context(), fmt.Sprintf("org:%d", org), "task-open/"+id+"/1")
	values["original_lookup"] = err == nil
	if err == nil {
		plan := r.Execution.Plan
		artifact, err := owner.Artifact(plan)
		values["artifact_valid"] = err == nil
		var original struct{ Task planrepo.AssessmentTaskPO }
		if err == nil && json.Unmarshal(artifact, &original) == nil && currentErr == nil {
			a, b := original.Task, current
			values["current_opened"] = b.Status == "opened"
			values["identity_equal"] = a.ID == b.ID && a.OrgID == b.OrgID && a.PlanID == b.PlanID && a.TesteeID == b.TesteeID
			values["revision_equal"] = a.ScheduleRevision == b.ScheduleRevision
			values["entry_equal"] = a.EntryURL == b.EntryURL
			equalClock := func(a, b *time.Time) bool {
				if a == nil || b == nil {
					return a == nil && b == nil
				}
				return a.Equal(*b)
			}
			values["open_clock_equal"] = equalClock(a.OpenAt, b.OpenAt)
			values["expiry_equal"] = equalClock(a.ExpireAt, b.ExpireAt)
			values["planned_equal"] = a.PlannedAt.Equal(b.PlannedAt)
			values["due_equal"] = equalClock(a.DueAt, b.DueAt)
			values["defined_equal"] = equalClock(a.ScheduleDefinedAt, b.ScheduleDefinedAt)
			values["stable_fields_equal"] = a.EnrollmentID == b.EnrollmentID && a.Seq == b.Seq && a.ScaleCode == b.ScaleCode
			values["full_current_match"] = (taskopen.RecoveryOpening{Task: a}).MatchesCurrent(&b)
		}
		branches := plan.Input().Branches
		values["single_branch"] = len(branches) == 1
		if len(branches) == 1 {
			values["logical_destination"] = branches[0].Message.Input().Destination == "qs.plan.task"
		}
		refs := r.Execution.References()
		if len(refs) == 1 {
			ref := refs[0]
			key := transaction.ExecutionKey(ref.Owner, ref.OperationID, ref.WriterEpoch, ref.Fence)
			branch := transaction.BranchKey(ref.BranchID)
			var state transaction.State
			err := db.Raw("SELECT state FROM rm_tx_execution WHERE execution_key=?", key[:]).Scan(&state).Error
			values["execution_committed"] = err == nil && state == transaction.Committed
			var witness struct {
				Reference []byte
				State     transaction.State
			}
			err = db.Raw("SELECT reference,state FROM rm_tx_witness WHERE execution_key=? AND branch_key=?", key[:], branch[:]).Take(&witness).Error
			expected, e := transaction.EncodeReference(ref)
			values["witness_exact"] = err == nil && e == nil && bytes.Equal(expected, witness.Reference)
			values["resolves_commit"] = transaction.Resolve(ref, &transaction.Evidence{Reference: ref, ExecutionState: state, BranchState: witness.State}) == transaction.Commit
		}
	}
	raw, _ := json.Marshal(values)
	fmt.Println("M3C_OPENING_DIAGNOSTIC " + string(raw))
}

func TestM3HostRetirementRecovery(t *testing.T) {
	require.True(t, strings.HasPrefix(os.Getenv("QS_M3_FIXTURE_PROJECT"), "rocketmq-m2-"))
	for _, key := range []string{"QS_M3_HOST_DSN", "QS_M3_HOST_MONGO_URI", "QS_M3_RECOVERY_REDIS", "QS_M3_RECOVERY_CERT", "QS_M3_RECOVERY_KEY"} {
		require.NotEmpty(t, os.Getenv(key), key)
	}
	cfg, err := driver.ParseDSN(os.Getenv("QS_M3_HOST_DSN"))
	require.NoError(t, err)
	host, _, err := net.SplitHostPort(cfg.Addr)
	require.NoError(t, err)
	require.True(t, net.ParseIP(host).IsLoopback())
	require.Empty(t, cfg.DBName)
	cfg.Loc = time.FixedZone("Asia/Shanghai", 8*3600)
	cfg.ParseTime = true
	cfg.MultiStatements = true
	cfg.Params = map[string]string{"time_zone": "'+08:00'"}
	sqlDB, dbName := openRuntimeDatabase(t, cfg.FormatDSN())
	cfg.DBName = dbName
	t.Setenv("QS_SERVER_TEST_MONGO_URI", os.Getenv("QS_M3_HOST_MONGO_URI"))
	t.Setenv("QS_SERVER_TEST_MONGO_DB_PREFIX", "qs_m3c_recovery_test")
	mongoClient, mongoDB := mongodbtest.ReplicaSetDatabase(t)
	pair := runtimeClosurePristinePair(t, sqlDB, dbName, mongoClient, mongoDB.Name())
	_, _, _, err = migrationpkg.EmbeddedMigrationResources()
	require.NoError(t, err)
	_, _, err = migrationpkg.NewMigrator(sqlDB, pair.MySQLConfig(false)).Run()
	require.NoError(t, err)
	_, _, err = migrationpkg.NewMongoMigrator(mongoClient, pair.MongoConfig(false)).Run()
	require.NoError(t, err)
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	addr := os.Getenv("QS_M3_RECOVERY_REDIS")
	rh, _, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	require.True(t, net.ParseIP(rh).IsLoopback())
	redisClient := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = redisClient.Close() })
	require.NoError(t, redisClient.Ping(t.Context()).Err())
	namespace := "m3c-" + uuid.NewString()
	keys := keyspace.NewBuilderWithNamespace(namespace)
	eventsConfig, err := eventcatalog.Load("../../../../configs/events.yaml")
	require.NoError(t, err)
	routes := eventcatalog.NewCatalog(eventsConfig)
	mq := m3cLoadMQ(t, os.Getenv("QS_M3_HOST_OPTIONS"))
	normal, err := messagingruntime.NewNormalWirePublisher(&mq, []string{"qs.survey.lifecycle", "qs.plan.task"}, 4)
	require.NoError(t, err)
	registry, err := eventcatalog.NewEffectiveRegistry(routes, eventcatalog.DefaultSpecs())
	require.NoError(t, err)
	consumers := map[string]eventsubsystem.ConsumerOptions{}
	for _, spec := range registry.Snapshot() {
		for _, c := range spec.AdditionalConsumers {
			consumers[c.ID] = eventsubsystem.ConsumerOptions{Enabled: false}
		}
	}
	events, err := eventsubsystem.NewWithTransactionOwners(eventsubsystem.Options{MySQLDB: db, MongoDB: mongoDB, OpsRedis: redisClient, Catalog: routes, WirePublisher: normal, PublisherMode: eventruntime.PublishModeMQ, Consumers: consumers}, &mq)
	require.NoError(t, err)
	my, mg := events.MySQLTransactionOwner().Binding(), events.MongoTransactionOwner().Binding()
	_, err = sqlDB.ExecContext(t.Context(), "INSERT INTO rm_tx_binding VALUES(?,?,?,1)", []byte(my.Owner), []byte(my.StoreID), []byte(my.WriterEpoch))
	require.NoError(t, err)
	_, err = mongoDB.Collection("rm_tx_binding").InsertOne(t.Context(), bson.M{"_id": mg.Owner, "store_id": mg.StoreID, "writer_epoch": mg.WriterEpoch, "schema_version": 1})
	require.NoError(t, err)
	locks := newTestLocks(redisClient, "apiserver", namespace+":locks")
	resilience, err := resiliencesubsystem.New(resiliencesubsystem.Options{InstanceID: "owned-m3c-api", Backpressure: apiserveroptions.NewBackpressureOptions(), Locks: locks})
	require.NoError(t, err)
	c := container.NewContainerWithOptions(db, mongoDB, redisClient, container.ContainerOptions{TaskOpenedReminderEnabled: true, EventSubsystem: events, Transactions: &container.EventTransactionBindings{MySQL: events.MySQLTransactionOwner(), Mongo: events.MongoTransactionOwner(), Routes: routes}, CacheSubsystem: newRuntimeCacheSubsystem(redisClient), LockSubsystem: locks, Resilience: resilience, PlanEntryBaseURL: "https://collection.example.test/entry", Silent: true})
	require.NoError(t, normal.Start())
	require.NoError(t, c.Initialize())
	require.NoError(t, c.StartEventSubsystem(t.Context()))
	t.Cleanup(func() {
		require.NoError(t, c.Cleanup())
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		require.NoError(t, normal.Close(ctx))
	})
	seedRuntimeCatalog(t, mongoDB)
	orgID := uint64(830000)
	require.NoError(t, mysqlActor.NewOperatorRepository(db).Save(t.Context(), operatorDomain.NewOperator(int64(orgID), 1, "owned headquarters")))
	deps := c.BuildGRPCDeps(nil)
	testeeID, _ := createRuntimeActorAndEntry(t, c, deps, orgID)
	taskIDs := []string{}
	for range 8 {
		plan, err := c.PlanModule.CommandService.CreatePlan(t.Context(), planapp.CreatePlanDTO{OrgID: int64(orgID), ScaleCode: runtimeModelCode, ScheduleType: "by_day", TriggerTime: "00:00:00", Interval: 1, TotalTimes: 1})
		require.NoError(t, err)
		enrollment, err := c.PlanModule.CommandService.EnrollTestee(t.Context(), planapp.EnrollTesteeDTO{OrgID: int64(orgID), PlanID: plan.ID, TesteeID: strconv.FormatUint(testeeID, 10), StartDate: time.Now().In(cfg.Loc).Format("2006-01-02")})
		require.NoError(t, err)
		require.Len(t, enrollment.Tasks, 1)
		taskID := enrollment.Tasks[0].ID
		_, err = c.PlanModule.CommandService.OpenTask(t.Context(), int64(orgID), taskID)
		require.NoError(t, err)
		taskIDs = append(taskIDs, taskID)
	}
	var providerCalls atomic.Int64
	var unexpectedRecipient atomic.Bool
	var unknown atomic.Bool
	providerHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var message wechat.SubscribeMessage
		if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
			w.WriteHeader(500)
			return
		}
		if message.ToUser != "owned-user-openid" {
			unexpectedRecipient.Store(true)
		}
		n := providerCalls.Add(1)
		if unknown.Load() {
			conn, _, e := w.(http.Hijacker).Hijack()
			if e == nil {
				_ = conn.Close()
			}
			return
		}
		_ = json.NewEncoder(w).Encode(wechat.SubscribeSendReceipt{Accepted: true, PlatformMessageID: fmt.Sprintf("owned-%d", n)})
	}))
	t.Cleanup(providerHTTP.Close)
	identity := &m3cIdentity{}
	freeze := &m3cFreeze{ReminderBatchLedger: ledger.NewReminderBatchLedger(db)}
	provider := m3cProvider{providerHTTP.URL}
	reminder := &m3cReminder{}
	newReminder := func() notification.TaskOpenedReminderService {
		return notification.NewTaskOpenedReminderService(c.PlanModule.TaskReminderStateReader, m3cTesteeLookup{get: c.ActorModule.ReadModel.GetTestee}, identity, freeze, ledger.NewReminderDeliveryLedger(db), nil, provider, provider, c.PlanModule.TaskNotificationContextReader, c.PublishedModelTitleResolver(), &notification.Config{AppID: "owned-app", AppSecret: "owned-fixture", PagePath: "pages/task/index", TaskOpenedTemplateID: "owned-template"})
	}
	reminder.set(newReminder())
	journey := intakejourney.NewService(deps.Survey.AnswerSheetScoringService, deps.Plan.TaskAssessmentResolver, deps.Plan.CommandService, deps.Evaluation.IntakeService, deps.Interpretation.ReportStatusReporter, deps.Survey.SubmissionReader)
	assessmentService := grpcservice.NewAssessmentIntakeService(journey, deps.Evaluation.IntakeService, deps.Survey.AnswerSheetManagementService)
	var phase atomic.Int32
	phase.Store(1)
	var ensureCalls atomic.Int64
	var reminderVisits sync.Map
	var ensureMu sync.Mutex
	var ensureInputs []*evaluationpb.EnsureAssessmentRequest
	held := make(chan *evaluationpb.EnsureAssessmentRequest, 1)
	var heldOnce sync.Once
	certPath, keyPath := os.Getenv("QS_M3_RECOVERY_CERT"), os.Getenv("QS_M3_RECOVERY_KEY")
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	require.NoError(t, err)
	ca, err := os.ReadFile(certPath)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(ca))
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots})), grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		response, e := next(ctx, req)
		if r, ok := req.(*internalpb.ProcessTaskOpenedReminderRequest); ok && e == nil {
			reminderVisits.Store(r.OpeningEventId, true)
		}
		if v, ok := req.(*evaluationpb.EnsureAssessmentRequest); ok && e == nil {
			ensureCalls.Add(1)
			ensureMu.Lock()
			ensureInputs = append(ensureInputs, proto.Clone(v).(*evaluationpb.EnsureAssessmentRequest))
			ensureMu.Unlock()
			if phase.CompareAndSwap(1, 2) {
				return nil, status.Error(codes.Unavailable, "owned response loss after original intake commit")
			}
			if phase.Load() == 3 {
				heldOnce.Do(func() { held <- v })
				<-ctx.Done()
				return nil, status.Error(codes.Canceled, "owned worker exited before response/ACK")
			}
		}
		return response, e
	}))
	assessmentService.RegisterService(server)
	grpcservice.NewEvaluationWorkerService(deps.Evaluation.WorkerService).RegisterService(server)
	grpcservice.NewInterpretationAutomationService(deps.Interpretation.AutomationService).RegisterService(server)
	grpcservice.NewInternalService(deps.Actor.TesteeAssessmentAttentionService, nil, nil, nil, nil, nil, nil, nil, reminder).RegisterService(server)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); require.NoError(t, <-serveDone) })
	answerService := grpcservice.NewAnswerSheetService(deps.Survey.AnswerSheetSubmissionService)
	request := &answersheetpb.SaveAnswerSheetRequest{QuestionnaireCode: runtimeQuestionCode, QuestionnaireVersion: runtimeVersion, IdempotencyKey: "m3c-original-answer", WriterId: testeeID, TesteeId: testeeID, OrgId: orgID, TaskId: taskIDs[7], OriginRef: &answersheetpb.OriginRef{Type: "plan_task", Id: taskIDs[7]}, Answers: []*answersheetpb.Answer{{QuestionCode: "Q1", QuestionType: "Radio", Value: "A"}}}
	answer, err := answerService.SaveAnswerSheet(t.Context(), request)
	require.NoError(t, err)
	require.NotZero(t, answer.GetId())
	// All fixture tasks/answer are now before this immutable cutoff. Wait the
	// existing operator policy's ten-second safety margin rather than weakening it.
	cutoff := time.Now().UTC()
	remaining := time.Until(cutoff.Add(11 * time.Second))
	select {
	case <-time.After(remaining):
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	cases := map[string]bool{}
	cases["original_transaction_roots_without_relay"] = true
	day := time.Now().In(cfg.Loc).Format("20060102")
	audit := t.TempDir()
	require.NoError(t, os.Chmod(audit, 0700))
	toolInput, _ := json.Marshal(map[string]any{"mongo_uri": os.Getenv("QS_M3_HOST_MONGO_URI"), "mongo_db": mongoDB.Name(), "redis_address": addr, "redis_db": 0, "redis_namespace": namespace})
	runTool := func(path string, expected int, env []string, input []byte, args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.Env = append(os.Environ(), env...)
		cmd.Stdin = bytes.NewReader(input)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		raw, e := cmd.Output()
		code := 0
		if e != nil {
			var exit *exec.ExitError
			require.ErrorAs(t, e, &exit, "owned operator executable unavailable")
			code = exit.ExitCode()
		}
		require.Equal(t, expected, code, "operator diagnostic: %s", stderr.String())
		return raw
	}
	hotTool := os.Getenv("QS_M3_RECOVERY_HOTRANK")
	indexBytes, err := os.ReadFile("../../../../scripts/oneoff/hotrank_day_rebuild/index.up.json")
	require.NoError(t, err)
	indexHash := sha256.Sum256(indexBytes)
	runTool(hotTool, 0, nil, toolInput, "--mode", "prepare-index", "--day", day, "--request-id", uuid.NewString(), "--operator", "owned-m3c", "--expected-fingerprint", hex.EncodeToString(indexHash[:]), "--audit-dir", audit)
	inspected := runTool(hotTool, 0, nil, toolInput, "--mode", "inspect", "--day", day)
	var inspect struct {
		Fingerprint string `json:"fingerprint"`
	}
	require.NoError(t, json.Unmarshal(inspected, &inspect))
	require.NotEmpty(t, inspect.Fingerprint)
	hotArgs := []string{"--mode", "apply", "--day", day, "--request-id", uuid.NewString(), "--operator", "owned-m3c", "--expected-fingerprint", inspect.Fingerprint, "--audit-dir", audit}
	runTool(hotTool, 0, nil, toolInput, hotArgs...)
	runTool(hotTool, 0, nil, toolInput, hotArgs...)
	rank := func() float64 {
		v, e := redisClient.ZScore(t.Context(), keys.BuildScaleHotDailyKey(day), runtimeQuestionCode).Result()
		require.NoError(t, e)
		return v
	}
	require.Equal(t, float64(1), rank())
	cases["real_redis_current_day_cli_rebuild_and_repeat"] = true

	recovery := gov.NewCurrentReminderRecovery(db, reminder, events.MySQLTransactionOwner().Artifact)
	reminderRequests := []govapp.ReminderGapRecoveryRequest{}
	for i, taskIndex := range []int{0, 1, 2, 7} {
		req := govapp.ReminderGapRecoveryRequest{TaskID: taskIDs[taskIndex], SourceProtocol: govapp.TransactionReminderSource, ExpectedScheduleRevision: 1, OpenedBefore: cutoff}
		inspection, err := recovery.InspectReminderGap(t.Context(), int64(orgID), 1, req)
		if err != nil {
			m3cOpeningDiagnostic(t, db, events.MySQLTransactionOwner(), int64(orgID), req.TaskID, i)
		}
		require.NoError(t, err, "original reminder index=%d", i)
		req.RequestID = uuid.NewString()
		req.OpeningEventID = inspection.OpeningEventID
		req.SourceFingerprint = inspection.SourceFingerprint
		req.EvidenceReference = "owned-m3c"
		req.Reason = "original operation only"
		req.Confirm = true
		unknown.Store(i == 1)
		freeze.fail.Store(i == 2)
		result, err := recovery.AuthorizeReminderGap(t.Context(), int64(orgID), 1, req)
		require.NoError(t, err)
		if i == 0 || i == 3 {
			require.Equal(t, "succeeded", result.Status)
		} else {
			require.Equal(t, govapp.ActionAuditStatusPendingReconciliation, result.Status)
		}
		reminderRequests = append(reminderRequests, req)
	}
	unknown.Store(false)
	require.EqualValues(t, 3, providerCalls.Load())
	reminder.set(newReminder())
	identity.changed.Store(true)
	recovered := gov.NewCurrentReminderRecovery(db, reminder, events.MySQLTransactionOwner().Artifact)
	_, err = recovered.ResumeReminderGap(t.Context(), int64(orgID), 1, reminderRequests[0])
	require.NoError(t, err)
	require.EqualValues(t, 3, providerCalls.Load())
	cases["known_reminder_original_receipt_no_repeat"] = true
	_, err = recovered.ResumeReminderGap(t.Context(), int64(orgID), 1, reminderRequests[1])
	require.NoError(t, err)
	require.EqualValues(t, 3, providerCalls.Load())
	cases["unknown_provider_response_never_resends"] = true
	_, err = recovered.ResumeReminderGap(t.Context(), int64(orgID), 1, reminderRequests[2])
	require.NoError(t, err)
	require.EqualValues(t, 4, providerCalls.Load())
	require.False(t, unexpectedRecipient.Load(), "recovery expanded the original frozen recipient set")
	var recipient string
	require.NoError(t, db.Table("task_opened_reminder_delivery").Select("user_id").Where("task_id=?", taskIDs[2]).Scan(&recipient).Error)
	require.Equal(t, "owned-user", recipient)
	cases["frozen_reminder_recipients_survive_recovery"] = true
	// The three remaining original tasks exercise refusal at the real SQL source.
	for i, change := range []string{"status='canceled'", "status='completed'", "planned_at=DATE_ADD(planned_at,INTERVAL 1 SECOND)", "expire_at=DATE_SUB(NOW(3),INTERVAL 1 SECOND)"} {
		require.NoError(t, db.Exec("UPDATE assessment_task SET "+change+" WHERE id=?", taskIDs[i+3]).Error)
		_, err = recovered.InspectReminderGap(t.Context(), int64(orgID), 1, govapp.ReminderGapRecoveryRequest{TaskID: taskIDs[i+3], SourceProtocol: govapp.TransactionReminderSource, ExpectedScheduleRevision: 1, OpenedBefore: cutoff})
		require.Error(t, err)
	}
	require.EqualValues(t, 4, providerCalls.Load())
	require.False(t, unexpectedRecipient.Load(), "recovery expanded the original frozen recipient set")
	// End the isolated corrupt-schedule counterfactual before normal deliveries.
	require.NoError(t, db.Exec("UPDATE assessment_task SET status='canceled' WHERE id=?", taskIDs[5]).Error)
	cases["closed_or_changed_task_refuses_new_send"] = true
	facade := govapp.NewFacade(govapp.FacadeDeps{CurrentReminderGapRecovery: gov.NewCurrentReminderRecovery(db, nil, events.MySQLTransactionOwner().Artifact), CurrentReminderGapRecoveryEnabled: false})
	_, found, err := facade.(govapp.ReminderGapRecovery).ResolveReminderGap(t.Context(), int64(orgID), 1, reminderRequests[0])
	require.NoError(t, err)
	require.True(t, found)
	cases["receipts_readable_with_writes_disabled"] = true
	gapTool := os.Getenv("QS_M3_RECOVERY_ANSWERSHEET")
	gapEnv := []string{"MYSQL_DSN=" + cfg.FormatDSN(), "MONGO_URI=" + os.Getenv("QS_M3_HOST_MONGO_URI"), "MONGO_DB=" + mongoDB.Name(), "M6_QS03_GRPC_ENDPOINT=" + listener.Addr().String(), "M6_QS03_CA_FILE=" + certPath, "M6_QS03_CERT_FILE=" + certPath, "M6_QS03_KEY_FILE=" + keyPath, "M6_QS03_SERVER_NAME=127.0.0.1"}
	gapArgs := []string{"--answersheet-id", strconv.FormatUint(answer.GetId(), 10), "--org-id", strconv.FormatUint(orgID, 10), "--accepted-before", cutoff.Format(time.RFC3339Nano)}
	gapInspect := runTool(gapTool, 0, gapEnv, nil, append([]string{"--mode", "inspect"}, gapArgs...)...)
	var gap struct {
		SourceFingerprint string `json:"source_fingerprint"`
		EventID           string `json:"event_id"`
	}
	require.NoError(t, json.Unmarshal(gapInspect, &gap))
	require.NotEmpty(t, gap.SourceFingerprint)
	gapSource, err := answersheetgap.New(mongoDB, db)
	require.NoError(t, err)
	originalPlan, err := gapSource.CaptureOriginalRecovery(t.Context(), answer.GetId(), orgID, cutoff, time.Now())
	require.NoError(t, err)
	require.Equal(t, gap.SourceFingerprint, originalPlan.SourceFingerprint)
	originalWireHash := sha256.Sum256(originalPlan.Message().Input().Payload)
	gapRequest := uuid.NewString()
	applyArgs := append(append([]string{}, gapArgs...), "--mode", "apply", "--source-fingerprint", gap.SourceFingerprint, "--audit-dir", audit, "--request-id", gapRequest, "--operator", "owned-m3c", "--reason", "owned original transaction recovery", "--external-result-reviewed")
	unknownReceipt := runTool(gapTool, 2, gapEnv, nil, applyArgs...)
	require.Contains(t, string(unknownReceipt), `"effect_outcome":"unknown"`)
	require.EqualValues(t, 1, ensureCalls.Load())
	reconciled := runTool(gapTool, 2, nil, nil, "--mode", "reconcile", "--audit-dir", audit, "--request-id", gapRequest)
	require.Contains(t, string(reconciled), `"no_repeat_effect":true`)
	require.Contains(t, string(reconciled), `"effect_outcome":"unknown"`)
	require.EqualValues(t, 1, ensureCalls.Load())
	cases["answersheet_operator_original_request_response_loss"] = true

	// Serialize only fresh owned connection selectors in a private child file.
	private := t.TempDir()
	require.NoError(t, os.Chmod(private, 0700))
	child := m3cChildConfig{DSN: cfg.FormatDSN(), MongoURI: os.Getenv("QS_M3_HOST_MONGO_URI"), MongoDB: mongoDB.Name(), Redis: addr, Namespace: namespace, Endpoint: listener.Addr().String(), Cert: certPath, Key: keyPath, Observer: os.Getenv("QS_M3_HOST_OBSERVER_OPTIONS"), Marker: filepath.Join(private, "first-delivery.json"), Hold: true}
	executable, err := os.Executable()
	require.NoError(t, err)
	type worker struct {
		cmd      *exec.Cmd
		done     chan error
		log      *os.File
		doneFlag atomic.Bool
	}
	workers := []*worker{}
	start := func(conf m3cChildConfig) *worker {
		t.Helper()
		path := filepath.Join(private, uuid.NewString()+".json")
		m3cPrivateJSON(t, path, conf)
		log, err := os.OpenFile(path+".log", os.O_CREATE|os.O_WRONLY, 0600)
		require.NoError(t, err)
		cmd := exec.Command(executable, "-test.run=^TestM3RecoveryWorkerProcess$", "-test.timeout=10m", "-test.v")
		cmd.Env = append(os.Environ(), "QS_M3C_WORKER_CHILD="+path)
		cmd.Stdout = log
		cmd.Stderr = log
		require.NoError(t, cmd.Start())
		w := &worker{cmd: cmd, done: make(chan error, 1), log: log}
		go func() { err := cmd.Wait(); w.doneFlag.Store(true); w.done <- err }()
		workers = append(workers, w)
		return w
	}
	t.Cleanup(func() {
		for _, w := range workers {
			if !w.doneFlag.Load() {
				if t.Failed() {
					_ = w.cmd.Process.Signal(syscall.SIGQUIT)
				} else {
					_ = w.cmd.Process.Signal(syscall.SIGTERM)
				}
				select {
				case <-w.done:
				case <-time.After(20 * time.Second):
					_ = w.cmd.Process.Kill()
					<-w.done
				}
			}
			if t.Failed() {
				// The harness withholds raw Go output and exports only its hash.
				// Preserve owned-child failure diagnostics in that private output.
				if raw, e := os.ReadFile(w.log.Name()); e == nil {
					fmt.Printf("M3C_PRIVATE_CHILD_DIAGNOSTIC\n%s\n", raw)
				}
			}
			_ = w.log.Close()
		}
	})
	phase.Store(3)
	first := start(child)
	select {
	case request := <-held:
		require.Equal(t, answer.GetId(), request.AnswerSheetId)
	case e := <-first.done:
		t.Fatalf("actual Worker exited before original effect: %v", e)
	case <-time.After(2 * time.Minute):
		t.Fatal("actual Worker did not reach original intake")
	}
	var firstMarker m3cDeliveryMarker
	m3cEventually(t, func() bool {
		raw, e := os.ReadFile(child.Marker)
		return e == nil && json.Unmarshal(raw, &firstMarker) == nil
	})
	require.Equal(t, gap.EventID, firstMarker.EventID)
	require.Equal(t, hex.EncodeToString(originalWireHash[:]), firstMarker.WireSHA)
	require.NoError(t, first.cmd.Process.Kill())
	exit := <-first.done
	require.Error(t, exit)
	require.Equal(t, syscall.SIGKILL, first.cmd.ProcessState.Sys().(syscall.WaitStatus).Signal())
	// A real killed Worker cannot release its production five-minute Redis
	// suppression lease. Observe the actual remaining TTL and wait for natural
	// expiry; never shorten the production policy or delete the key to pass.
	workerRedis := redis.NewClient(&redis.Options{Addr: addr, DB: 1})
	t.Cleanup(func() { _ = workerRedis.Close() })
	leaseKeys := []string{}
	leaseScan := workerRedis.Scan(t.Context(), 0, "*answersheet:processing:"+strconv.FormatUint(answer.GetId(), 10), 100).Iterator()
	for leaseScan.Next(t.Context()) {
		leaseKeys = append(leaseKeys, leaseScan.Val())
	}
	require.NoError(t, leaseScan.Err())
	require.Len(t, leaseKeys, 1, "actual killed Worker processing lease is required")
	processingLease, err := workerRedis.PTTL(t.Context(), leaseKeys[0]).Result()
	require.NoError(t, err)
	capability, ok := locklease.Lookup(locklease.WorkloadAnswersheetProcessing)
	require.True(t, ok)
	require.Positive(t, processingLease)
	require.LessOrEqual(t, processingLease, capability.Spec.DefaultTTL)
	processingExpiresAt := time.Now().Add(processingLease)
	// Evaluation claims last two minutes; interpretation and suppression
	// leases use five. All existing leases may survive the same SIGKILL.
	// Use one absolute observed window for the whole recovery, not a shorter
	// independent report timer that can cut a valid reclaimed attempt short.
	recoveryDeadline := processingExpiresAt.Add(2 * time.Minute)
	fmt.Printf("M3C_REAL_PROCESSING_LEASE_MS %d\n", processingLease.Milliseconds())
	cases["worker_sigkill_after_effect_before_ack"] = true
	brokerID := m3cBroker(t, "id")
	m3cBroker(t, "kill")
	require.Equal(t, "137", m3cBroker(t, "exit"))
	m3cBroker(t, "start")
	require.Equal(t, brokerID, m3cBroker(t, "id"))
	// A container start is not Proxy readiness. Restart the replacement Worker
	// after the real Broker health check and verified TLS handshake succeed.
	// Original delivery/effect assertions below remain mandatory.
	connection, err := mq.RocketMQ.Load([]string{"qs.evaluation.lifecycle"}, 2, 5*time.Second)
	require.NoError(t, err)
	m3cEventually(t, func() bool {
		if m3cBroker(t, "health") != "healthy" {
			return false
		}
		conn, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", connection.Endpoint, connection.TLS.Clone())
		if e != nil {
			return false
		}
		_ = conn.Close()
		return true
	})
	// NameServer registration is a separate readiness fact. Read the original
	// manifest and wait until every provisioned physical topic reappears; do
	// not update/recreate topics after the crash or substitute a new Broker.
	routeBytes, err := os.ReadFile(mq.RocketMQ.RoutesFile)
	require.NoError(t, err)
	manifest, err := transport.ParseRouteManifest(routeBytes)
	require.NoError(t, err)
	m3cEventually(t, func() bool {
		present := map[string]bool{}
		for _, topic := range strings.Fields(m3cBroker(t, "topics")) {
			present[topic] = true
		}
		for _, route := range manifest.Routes {
			if !present[route.PhysicalTopic] {
				return false
			}
		}
		return true
	})
	// A NameServer topic list can be populated while the Proxy still returns
	// TOPIC_NOT_FOUND. Reuse the existing authenticated read-only route probe
	// for every actual Worker/hotrank physical input before starting a client.
	var routeCommand []string
	require.NoError(t, json.Unmarshal([]byte(os.Getenv("QS_M3_RECOVERY_ROUTE_PROBE")), &routeCommand))
	require.Len(t, routeCommand, 2)
	workerMQ := m3cLoadMQ(t, os.Getenv("QS_M3_HOST_OBSERVER_OPTIONS"))
	workerConnection, err := workerMQ.RocketMQ.Load(nil, 2, 5*time.Second)
	require.NoError(t, err)
	readyTopics := map[string]bool{}
	criticalInput := false
	for _, route := range manifest.Routes {
		if route.Subscription == "qs-worker" || route.Subscription == "qs-apiserver-modelcatalog-hot-rank-v1" {
			readyTopics[route.PhysicalTopic] = true
			criticalInput = criticalInput || (route.Topic == "qs.evaluation.lifecycle" && route.Owner == "qs.mongo")
		}
	}
	require.True(t, criticalInput)
	require.NotEmpty(t, readyTopics)
	proxyDeadline := time.Now().Add(2 * time.Minute)
	for topic := range readyTopics {
		probePath := filepath.Join(private, uuid.NewString()+"-route.json")
		m3cPrivateJSON(t, probePath, map[string]string{"endpoint": workerConnection.Endpoint, "accesskey": workerConnection.AccessKey, "secretkey": workerConnection.SecretKey, "topic": topic, "ca": workerMQ.RocketMQ.CAFile})
		for {
			remaining := time.Until(proxyDeadline)
			require.Positive(t, remaining, "actual authenticated Proxy routes did not become ready")
			ctx, cancel := context.WithTimeout(t.Context(), remaining)
			cmd := exec.CommandContext(ctx, routeCommand[0], routeCommand[1], "--config", probePath, "--mode", "route-read", "--require-ready", "--id", "owned-restarted-route")
			raw, e := cmd.Output()
			cancel()
			if e == nil {
				require.Contains(t, string(raw), `"proxy_route"`)
				break
			}
			select {
			case <-time.After(time.Second):
			case <-t.Context().Done():
				t.Fatal(t.Context().Err())
			}
		}
	}
	fmt.Printf("M3C_ACTUAL_PROXY_ROUTES_READY %d\n", len(readyTopics))
	cases["same_owned_broker_sigkill_restart"] = true
	phase.Store(4)
	child.Hold = false
	child.Marker = filepath.Join(private, "second-delivery.json")
	second := start(child)
	require.NotEqual(t, first.cmd.Process.Pid, second.cmd.Process.Pid)
	var secondMarker m3cDeliveryMarker
	redeliveryDeadline := time.NewTimer(2 * time.Minute)
	defer redeliveryDeadline.Stop()
	redeliveryPoll := time.NewTicker(200 * time.Millisecond)
	defer redeliveryPoll.Stop()
redelivery:
	for {
		raw, e := os.ReadFile(child.Marker)
		if e == nil && json.Unmarshal(raw, &secondMarker) == nil {
			break redelivery
		}
		select {
		case e := <-second.done:
			t.Fatalf("replacement Worker exited before original redelivery: %v", e)
		case <-redeliveryDeadline.C:
			t.Fatal("replacement Worker did not observe original redelivery")
		case <-redeliveryPoll.C:
		}
	}
	require.Equal(t, firstMarker, secondMarker)
	cases["actual_original_wire_redelivery_after_restart"] = true
	reportBudget := time.Until(recoveryDeadline)
	require.Positive(t, reportBudget)
	require.Eventually(t, func() bool {
		result, e := assessmentService.ResolveAssessmentByAnswerSheetID(t.Context(), &evaluationpb.ResolveAssessmentByAnswerSheetIDRequest{AnswerSheetId: answer.GetId()})
		if e != nil || result.GetAssessmentStatus() != "evaluated" {
			return false
		}
		count, e := mongoDB.Collection("interpret_report_artifacts").CountDocuments(t.Context(), bson.M{})
		return e == nil && count == 1
	}, reportBudget, 200*time.Millisecond)
	evaluated, err := assessmentService.ResolveAssessmentByAnswerSheetID(t.Context(), &evaluationpb.ResolveAssessmentByAnswerSheetIDRequest{AnswerSheetId: answer.GetId()})
	require.NoError(t, err)
	assertRowCount(t, db, "assessment", "answer_sheet_id=?", 1, answer.GetId())
	assertRowCount(t, db, "runtime_checkpoint", "scope=? AND assessment_id=?", 1, "evaluation_run", evaluated.GetAssessmentId())
	assertRowCount(t, db, "evaluation_outcome", "assessment_id=?", 1, evaluated.GetAssessmentId())
	for _, name := range []string{"report_generations", "interpretation_runs", "interpret_report_artifacts"} {
		count, err := mongoDB.Collection(name).CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
	}
	var originalOutcome struct {
		ID              string
		EvaluationRunID string
	}
	require.NoError(t, db.Table("evaluation_outcome").Select("id,evaluation_run_id").Where("assessment_id=?", evaluated.GetAssessmentId()).Take(&originalOutcome).Error)
	require.NotEmpty(t, originalOutcome.ID)
	require.NotEmpty(t, originalOutcome.EvaluationRunID)
	originalReports := map[string]any{}
	for _, name := range []string{"report_generations", "interpretation_runs", "interpret_report_artifacts"} {
		var document bson.M
		require.NoError(t, mongoDB.Collection(name).FindOne(t.Context(), bson.M{}).Decode(&document))
		originalReports[name] = document["_id"]
		require.NotNil(t, originalReports[name])
	}
	// Keep the production lease and existing redelivery/backoff policy. The
	// deadline follows the observed expiry plus a bounded delivery margin.
	lateDeliveryBudget := time.Until(recoveryDeadline)
	require.Positive(t, lateDeliveryBudget)
	defer func() {
		if !t.Failed() {
			return
		}
		// Read only this test's original event and owned Redis key. Emit counts
		// and technical states, never fixture credentials, payloads or DSNs.
		visited := make([]bool, len(reminderRequests))
		for i, req := range reminderRequests {
			_, visited[i] = reminderVisits.Load(req.OpeningEventID)
		}
		var deadLetters []struct {
			DeliveryAttempts int
			RetryDisposition string
			LastError        string
		}
		deadErr := db.Table("event_delivery_dead_letter").Select("delivery_attempts,retry_disposition,last_error").Where("event_id=?", gap.EventID).Limit(10).Find(&deadLetters).Error
		var held []struct {
			Status             string
			RetryDisposition   string
			ReplayAttemptCount int
		}
		holdErr := db.Table("retry_event_hold").Select("status,retry_disposition,replay_attempt_count").Where("event_id=?", gap.EventID).Limit(10).Find(&held).Error
		ttl, redisErr := workerRedis.PTTL(context.Background(), leaseKeys[0]).Result()
		encoded, _ := json.Marshal(map[string]any{"ensure_calls": ensureCalls.Load(), "reminder_visits": visited,
			"dead_letters": deadLetters, "dead_letter_read_ok": deadErr == nil, "held_deliveries": held, "hold_read_ok": holdErr == nil,
			"worker_lock_pttl_ms": ttl.Milliseconds(), "redis_read_ok": redisErr == nil, "provider_calls": providerCalls.Load()})
		fmt.Println("M3C_COMPLETION_DIAGNOSTIC " + string(encoded))
	}()
	require.Eventually(t, func() bool {
		for _, req := range reminderRequests {
			if _, ok := reminderVisits.Load(req.OpeningEventID); !ok {
				return false
			}
		}
		return ensureCalls.Load() >= 3
	}, lateDeliveryBudget, 200*time.Millisecond)
	m3cEventually(t, func() bool {
		remaining, e := workerRedis.Exists(t.Context(), leaseKeys[0]).Result()
		return e == nil && remaining == 0
	})
	require.GreaterOrEqual(t, ensureCalls.Load(), int64(3))
	ensureMu.Lock()
	inputs := append([]*evaluationpb.EnsureAssessmentRequest(nil), ensureInputs...)
	ensureMu.Unlock()
	for _, input := range inputs {
		require.True(t, proto.Equal(inputs[0], input), "original EnsureAssessment request changed on late delivery")
	}
	require.Equal(t, float64(1), rank())
	require.EqualValues(t, 4, providerCalls.Load())
	require.False(t, unexpectedRecipient.Load(), "recovery expanded the original frozen recipient set")
	cases["late_original_delivery_preserves_rank_and_notification_counts"] = true
	// Conflicting original evidence must fail the whole day without replacing it.
	_, err = mongoDB.Collection("rm_tx_execution").UpdateMany(t.Context(), bson.M{"owner": "qs.mongo"}, bson.M{"$set": bson.M{"plan_digest": make([]byte, 32)}})
	require.NoError(t, err)
	source, err := redishot.NewMongoDaySource(mongoDB)
	require.NoError(t, err)
	_, err = source.Capture(t.Context(), day)
	require.Error(t, err)
	require.Equal(t, float64(1), rank())
	corruptArgs := append([]string(nil), hotArgs...)
	for i := range corruptArgs {
		if i > 0 && corruptArgs[i-1] == "--request-id" {
			corruptArgs[i] = uuid.NewString()
		}
	}
	runTool(hotTool, 1, nil, toolInput, corruptArgs...)
	require.Equal(t, float64(1), rank())
	cases["conflicting_source_refuses_whole_rebuild"] = true
	for _, table := range []string{"rm_outbox", "ai_messaging_outbox"} {
		var count int64
		require.NoError(t, db.Table(table).Count(&count).Error)
		require.Zero(t, count)
	}
	count, err := mongoDB.Collection("rm_outbox").CountDocuments(t.Context(), bson.M{})
	require.NoError(t, err)
	require.Zero(t, count)
	require.NoError(t, second.cmd.Process.Signal(syscall.SIGTERM))
	select {
	case e := <-second.done:
		require.NoError(t, e)
	case <-time.After(30 * time.Second):
		t.Fatal("actual Worker failed to drain")
	}
	// Recheck persistent identities and counts AFTER the late original request
	// finished and the real Worker drained. Earlier pre-replay counts alone
	// cannot establish no duplicate business effect.
	finalAssessment, err := assessmentService.ResolveAssessmentByAnswerSheetID(t.Context(), &evaluationpb.ResolveAssessmentByAnswerSheetIDRequest{AnswerSheetId: answer.GetId()})
	require.NoError(t, err)
	require.Equal(t, evaluated.GetAssessmentId(), finalAssessment.GetAssessmentId())
	require.Equal(t, "evaluated", finalAssessment.GetAssessmentStatus())
	assertRowCount(t, db, "assessment", "answer_sheet_id=?", 1, answer.GetId())
	assertRowCount(t, db, "runtime_checkpoint", "scope=? AND assessment_id=?", 1, "evaluation_run", evaluated.GetAssessmentId())
	assertRowCount(t, db, "evaluation_outcome", "assessment_id=?", 1, evaluated.GetAssessmentId())
	finalOutcome := originalOutcome
	require.NoError(t, db.Table("evaluation_outcome").Select("id,evaluation_run_id").Where("assessment_id=?", evaluated.GetAssessmentId()).Take(&finalOutcome).Error)
	require.Equal(t, originalOutcome, finalOutcome)
	for name, originalID := range originalReports {
		count, e := mongoDB.Collection(name).CountDocuments(t.Context(), bson.M{})
		require.NoError(t, e)
		require.EqualValues(t, 1, count)
		var document bson.M
		require.NoError(t, mongoDB.Collection(name).FindOne(t.Context(), bson.M{}).Decode(&document))
		require.Equal(t, originalID, document["_id"], "late delivery replaced the original report identity")
	}
	require.Equal(t, float64(1), rank())
	require.EqualValues(t, 4, providerCalls.Load())
	require.False(t, unexpectedRecipient.Load())
	cases["one_assessment_run_outcome_and_report"] = true
	// Cleanup assertions run before the exported receipt; t.Cleanup still owns
	// safety cleanup on every earlier failure. The outer fixture removes resources.
	require.NoError(t, c.Cleanup())
	require.NoError(t, normal.Close(t.Context()))
	require.NoError(t, mongoDB.Drop(t.Context()))
	_, err = sqlDB.ExecContext(t.Context(), "DROP DATABASE `"+dbName+"`")
	require.NoError(t, err)
	sha, err := exec.CommandContext(t.Context(), "git", "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	require.Len(t, cases, 14)
	receipt := map[string]any{"source_sha": strings.TrimSpace(string(sha)), "passed": len(cases), "failed": 0, "skipped": 0, "cases": cases, "real_redis": true, "proxy_routes_ready": true, "proxy_routes_checked": len(readyTopics), "processing_lease_observed": true, "processing_lease_ttl_ms": processingLease.Milliseconds(), "broker_sigkill": true, "worker_sigkill": true, "controlled_provider": true, "worker_pids_distinct": true, "owned_databases_dropped": true, "no_existing_storage": true, "no_production": true}
	rawReceipt, err := json.Marshal(receipt)
	require.NoError(t, err)
	fmt.Println("M3C_QS_RECOVERY_RECEIPT " + string(rawReceipt))
}
