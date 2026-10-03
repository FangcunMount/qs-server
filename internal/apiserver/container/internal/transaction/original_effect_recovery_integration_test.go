//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration && reliable_messaging_m5

package transaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	appexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	appintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	appoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomecommit "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/commit"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	descriptor "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	evalworker "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/worker"
	automation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
	execution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	mongoreport "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	evaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	sqlreport "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/interpretation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/originaleffect"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/recoveryjournal"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	servergrpc "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/qs-server/internal/testutil/tlsfixture"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	workereventing "github.com/FangcunMount/qs-server/internal/worker/integration/eventing"
	workermessaging "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"github.com/FangcunMount/reliable-messaging/transport"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/nsqio/go-nsq"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const effectProofDB = "m6_original_effects"
const effectProofChannel = "rm-original-effect-loss"

func effectProofConnections(t *testing.T) (*gorm.DB, *mongo.Database, string) {
	t.Helper()
	dsn, uri, tcp := os.Getenv("RM_EFFECT_MYSQL_DSN"), os.Getenv("RM_EFFECT_MONGO_URI"), os.Getenv("RM_EFFECT_NSQ_TCP")
	parsed, err := driver.ParseDSN(dsn)
	if err != nil || parsed == nil || parsed.DBName != effectProofDB || parsed.Net != "tcp" || !strings.HasPrefix(parsed.Addr, "127.0.0.1:") || !strings.HasPrefix(uri, "mongodb://127.0.0.1:") || !strings.Contains(uri, "directConnection=true") || !strings.HasPrefix(tcp, "127.0.0.1:") {
		t.Fatal("explicit disposable loopback MySQL/Mongo/NSQ required; never skip")
	}
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	client, err := mongo.Connect(t.Context(), options.Client().ApplyURI(uri))
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		require.NoError(t, client.Disconnect(ctx))
	})
	require.NoError(t, client.Ping(t.Context(), nil))
	return db, client.Database(effectProofDB), tcp
}

func effectProofCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Load("../../../../../configs/events.yaml")
	require.NoError(t, err)
	return catalog.NewCatalog(c)
}

// Real failure/grant and success transactions create original standard intents.
// The seeded calculation result is a fixed fixture, not a model-quality proof.
func TestOriginalEffectsPostConfirmSeed(t *testing.T) {
	db, mdb, tcp := effectProofConnections(t)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	require.NoError(t, db.AutoMigrate(&evaluation.AssessmentPO{}, &evaluation.AssessmentScorePO{}, &evaluation.EvaluationOutcomePO{}, &checkpoint.RuntimeCheckpointPO{}, &sqlreport.AdmissionFailurePO{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, sdkmysql.Schema)
	require.NoError(t, err)
	require.NoError(t, createEvaluationRequestRefTable(ctx, sqlDB))
	require.NoError(t, mdb.CreateCollection(ctx, "rm_outbox"))
	_, err = mdb.Collection("rm_outbox").Indexes().CreateMany(ctx, sdkmongo.Indexes())
	require.NoError(t, err)
	_, err = mongoreport.NewGenerationRepository(mdb)
	require.NoError(t, err)
	_, err = mongoreport.NewReportRepository(mdb)
	require.NoError(t, err)
	stager, err := mysqlstandard.NewStager(effectProofCatalog(t), "qs-apiserver")
	require.NoError(t, err)
	repo := evaluation.NewAssessmentRepository(db)
	runs := checkpoint.NewRunRepository(db)
	tx := NewMySQLRunner(db)
	intake := appintake.NewService(repo, proofModelValidator{}, tx, stager)
	create := func(sheet uint64) uint64 {
		kind, algorithm, code, version := "scale", string(modelcatalog.AlgorithmScaleDefault), "SCALE-1", "v1"
		a, err := intake.CreateForAnswerSheet(ctx, appintake.CreateCommand{OrgID: 1, TesteeID: 2, AnswerSheetID: sheet, QuestionnaireCode: "Q1", QuestionnaireVersion: "v1", OriginType: "adhoc", ModelKind: &kind, ModelAlgorithm: &algorithm, ModelCode: &code, ModelVersion: &version})
		require.NoError(t, err)
		record, err := repo.FindByID(ctx, meta.FromUint64(a.ID))
		require.NoError(t, err)
		require.NoError(t, record.Submit())
		record.ClearEvents()
		require.NoError(t, repo.Save(ctx, record))
		return a.ID
	}
	retryID := create(89001)
	engine := appexecute.NewEngine(repo, m5TerminalInput{}, appexecute.WithRunRepository(runs), appexecute.WithTransactionalOutbox(tx, stager))
	require.Error(t, engine.Evaluate(ctx, retryID))
	grantCtx := appauthz.WithSnapshot(ctx, &appauthz.Snapshot{Permissions: []appauthz.Permission{{Resource: appauthz.AssessmentResource, Action: "force_retry", Mode: appauthz.AuthorizationModeUnconditional}}})
	grant, err := appoperator.NewGovernedRetryService(repo, runs, tx, stager, m5GovernedAccess{}).Authorize(grantCtx, appoperator.Actor{OrgID: 1, OperatorUserID: 9}, appoperator.GovernedRetryCommand{AssessmentID: retryID, ExpectedAttempt: 1, Origin: retrygovernance.AttemptOriginForce, RequestID: "original-force-request", Reason: "known pre-model terminal fixture", AuthorizationSubject: "9", AuthorizationAction: "force_retry"})
	require.NoError(t, err)
	require.NotEmpty(t, grant.RetryDecision().RetryEventID)
	reportID := create(89002)
	record, err := repo.FindByID(ctx, meta.FromUint64(reportID))
	require.NoError(t, err)
	now := time.Now()
	claim, err := runs.Claim(ctx, evaluationrun.ClaimRequest{AssessmentID: reportID, Token: "original-outcome", ClaimedAt: now, LeaseUntil: now.Add(time.Minute)})
	require.NoError(t, err)
	require.True(t, claim.Claimed)
	require.NoError(t, claim.Run.AttachInputSnapshot("frozen-original-input"))
	require.NoError(t, runs.SaveClaimed(ctx, claim.Run))
	result := domainoutcome.NewExecution(evaloutcome.ModelRefFromAssessment(*record.EvaluationModelRef()), domainoutcome.Summary{PrimaryLabel: "low"}, domainoutcome.Detail{Kind: modelcatalog.KindScale})
	result.Primary = &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 12}
	result.Level = &domainoutcome.ResultLevel{Code: "low", Label: "low"}
	result.Dimensions = []domainoutcome.DimensionResult{{Code: "TOTAL", Name: "total", Kind: domainoutcome.DimensionKindFactor, Role: "total", Score: result.Primary, Level: result.Level}}
	input := &evaluationinput.InputSnapshot{Model: &evaluationinput.ModelSnapshot{Kind: evaluationinput.EvaluationModelKindScale, Algorithm: string(modelcatalog.AlgorithmScaleDefault), DecisionKind: string(modelcatalog.DecisionKindScoreRange), Code: "SCALE-1", Version: "v1"}, DefinitionV2: &modeldefinition.Definition{Measure: modeldefinition.MeasureSpec{Factors: []factor.Factor{{Code: "TOTAL", Title: "total", Role: factor.FactorRoleTotal}}}, InterpretationAssets: interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "low", Title: "low", Summary: "original"}}, ReportSpec: interpretationassets.ReportSpec{Sections: []interpretationassets.ReportSection{{Code: "summary", Kind: "template", TemplateID: "factor", TemplateVersion: "frozen-v1"}}}}}}
	committer := outcomecommit.NewCommitter(tx, repo, evaluation.NewOutcomeRepository(db), runs, outcomescoring.NewAssessmentScoreProjector(evaluation.NewScoreRepository(db)), stager, nil)
	committed, err := committer.Commit(ctx, outcomecommit.CommitRequest{Assessment: record, Input: input, Execution: result, DescriptorKey: descriptor.DescriptorKey{DecisionKind: modelcatalog.DecisionKindScoreRange}, OutcomePolicy: descriptor.DefaultOutcomeCompletenessPolicy(modelcatalog.DecisionKindScoreRange), Run: &claim.Run, EvaluatedAt: time.Now()})
	require.NoError(t, err)
	require.NotNil(t, committed)
	producer, err := nsq.NewProducer(tcp, nsq.NewConfig())
	require.NoError(t, err)
	producer.SetLogger(nil, nsq.LogLevelError)
	defer producer.Stop()
	publisher, err := sdknsq.New(producer, map[string]string{"qs.evaluation.lifecycle": "qs.evaluation.lifecycle"}, 1)
	require.NoError(t, err)
	store, err := sdkmysql.New(sqlDB)
	require.NoError(t, err)
	relayEngine, err := relay.New(store, publisher, relay.Config{Concurrency: 1, PollInterval: 10 * time.Millisecond, Lease: 10 * time.Second, PublishTimeout: 2 * time.Second, WriteTimeout: time.Second, Retry: standardoutbox.SDKRetryPolicy(), Observe: func(relay.Event) {}})
	require.NoError(t, err)
	relayCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- relayEngine.Run(relayCtx) }()
	require.Eventually(t, func() bool {
		var pending int64
		return db.Table("rm_outbox").Where("state<>?", "published").Count(&pending).Error == nil && pending == 0
	}, 10*time.Second, 20*time.Millisecond)
	stop()
	require.NoError(t, <-done)
	var count int64
	require.NoError(t, db.Table("rm_outbox").Count(&count).Error)
	require.EqualValues(t, 3, count)
	t.Logf("original_grant=%s assessment=%d original_outcome=%s confirmed_intents=3", grant.RetryDecision().RetryEventID, retryID, committed.ID())
}

type effectSQLFacts struct{ source domainoutcome.Repository }

func (r effectSQLFacts) FindByID(ctx context.Context, id meta.ID) (*evaluationfact.Record, error) {
	v, e := r.source.FindByID(ctx, id)
	return evaloutcome.FactRecord(v), e
}
func (r effectSQLFacts) FindByAssessmentID(ctx context.Context, id meta.ID) (*evaluationfact.Record, error) {
	v, e := r.source.FindByAssessmentID(ctx, id)
	return evaloutcome.FactRecord(v), e
}

type effectReportBuilder struct{ calls atomic.Int32 }

func (*effectReportBuilder) ReportType() policy.ReportType           { return policy.ReportTypeStandard }
func (*effectReportBuilder) TemplateVersion() policy.TemplateVersion { return "frozen-v1" }
func (*effectReportBuilder) BuilderIdentity() string {
	return domainreport.BuilderIdentityFactorScoring
}
func (*effectReportBuilder) ContentSchemaVersion() string { return "report-content/v1" }
func (*effectReportBuilder) MechanismKey() rendering.Key {
	return rendering.Key{DecisionKind: modelcatalog.DecisionKindScoreRange, ReportType: policy.ReportTypeStandard}
}
func (b *effectReportBuilder) Build(_ context.Context, in interpinput.InterpretationInput) (*domainreport.Draft, error) {
	b.calls.Add(1)
	if in.Model.Version != "v1" || in.Report.TemplateID != "factor" || in.Report.TemplateVersion != "frozen-v1" {
		return nil, errors.New("original frozen route changed")
	}
	return domainreport.NewDraft(domainreport.Content{Model: domainreport.ModelIdentity{Kind: "scale", Code: "SCALE-1", Version: "v1"}, PrimaryScore: domainreport.NewRawTotalScore(12, nil), Level: domainreport.LevelFromRisk(domainreport.RiskLevelLow), Conclusion: "original", Dimensions: []domainreport.DimensionInterpret{domainreport.NewDimensionInterpret(domainreport.NewFactorCode("TOTAL"), "total", 12, nil, domainreport.RiskLevelLow, "original", "original")}}), nil
}

type effectNormalSubscriber struct {
	sub        *sdknsq.Subscriber
	ctx        context.Context
	originals  map[string][]byte
	deliveries atomic.Int32
	failed     atomic.Int32
}

func (s *effectNormalSubscriber) Subscribe(topic, channel string, next transport.Handler) error {
	return s.sub.Subscribe(s.ctx, topic, channel, func(ctx context.Context, d transport.Delivery) error {
		got := d.Message()
		expected, ok := s.originals[got.ID]
		if !ok || !bytes.Equal(got.Payload, expected) {
			return errors.New("original identity changed")
		}
		s.deliveries.Add(1)
		return next(ctx, d)
	}, func(context.Context, legacy.FailedHandoff) error {
		s.failed.Add(1)
		return errors.New("unexpected failed handoff")
	})
}

// Actual CLI, primary SQL/Mongo, production mTLS/ACL, normal Worker mapping,
// grant Claim, report Starter/Executor and success transaction are exercised.
// The known pre-model failure resolver, seed calculation and content builder
// are fixtures; no external model/WeChat or production timing is claimed.
func TestOriginalEffectsRecoveryAfterActualBrokerLoss(t *testing.T) {
	db, mdb, tcp := effectProofConnections(t)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	binary := os.Getenv("RM_EFFECT_RECOVERY_BINARY")
	require.NotEmpty(t, binary)
	stager, err := mysqlstandard.NewStager(effectProofCatalog(t), "qs-apiserver")
	require.NoError(t, err)
	assessments := evaluation.NewAssessmentRepository(db)
	runs := checkpoint.NewRunRepository(db)
	engine := appexecute.NewEngine(assessments, m5TerminalInput{}, appexecute.WithRunRepository(runs), appexecute.WithTransactionalOutbox(NewMySQLRunner(db), stager))
	gens, err := mongoreport.NewGenerationRepository(mdb)
	require.NoError(t, err)
	iruns, err := mongoreport.NewRunRepository(mdb)
	require.NoError(t, err)
	reports, err := mongoreport.NewReportRepository(mdb)
	require.NoError(t, err)
	projection, err := mongoreport.NewReportCatalogProjector(mdb)
	require.NoError(t, err)
	mongoStager, err := mongostandard.NewStager(mdb.Collection("rm_outbox"), effectProofCatalog(t), "qs-apiserver")
	require.NoError(t, err)
	mongoTx := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		session, err := mdb.Client().StartSession()
		if err != nil {
			return err
		}
		defer session.EndSession(ctx)
		_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (any, error) { return nil, fn(sc) })
		return err
	})
	starter, err := execution.NewStarter(mongoTx, gens, iruns, reports, time.Minute)
	require.NoError(t, err)
	committer, err := execution.NewInterpretationCommitter(mongoTx, gens, iruns, reports, mongoStager, nil, projection)
	require.NoError(t, err)
	builder := &effectReportBuilder{}
	registry, err := rendering.NewRegistry(builder)
	require.NoError(t, err)
	executor, err := execution.NewExecutor(starter, registry, committer)
	require.NoError(t, err)
	reportService, err := automation.NewService(effectSQLFacts{evaluation.NewOutcomeRepository(db)}, executor)
	require.NoError(t, err)
	ca := tlsfixture.New(t)
	pair := ca.Issue(t, "server.test", false)
	worker := ca.Issue(t, "qs-worker.svc", false)
	server, err := servergrpc.NewServer(&servergrpc.Config{TLSCertFile: pair.CertFile, TLSKeyFile: pair.KeyFile, MTLS: servergrpc.MTLSConfig{Enabled: true, CAFile: ca.CAFile, RequireClientCert: true}, ACL: servergrpc.ACLConfig{Enabled: true, ConfigFile: "../../../../../configs/grpc-acl.prod.yaml", DefaultPolicy: "deny"}}, nil)
	require.NoError(t, err)
	defer server.Stop()
	grpcservice.NewEvaluationWorkerService(evalworker.NewService(engine, assessments, evaluation.NewOutcomeRepository(db), runs)).RegisterService(server.Server)
	grpcservice.NewInterpretationAutomationService(reportService).RegisterService(server.Server)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = server.Serve(lis) }()
	env := append(os.Environ(), "MYSQL_DSN="+os.Getenv("RM_EFFECT_MYSQL_DSN"), "MONGO_URI="+os.Getenv("RM_EFFECT_MONGO_URI"), "MONGO_DB="+effectProofDB, "M6_EFFECT_GRPC_ENDPOINT="+lis.Addr().String(), "M6_EFFECT_CA_FILE="+ca.CAFile, "M6_EFFECT_CERT_FILE="+worker.CertFile, "M6_EFFECT_KEY_FILE="+worker.KeyFile, "M6_EFFECT_SERVER_NAME=server.test")
	runCLI := func(want int, args ...string) []byte {
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = env
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit, stderr.String())
			code = exit.ExitCode()
		}
		require.Equal(t, want, code, stderr.String())
		return out.Bytes()
	}
	var originalRows []struct {
		MessageID, EventType  string
		Payload               []byte
		Version, AttemptCount uint64
	}
	require.NoError(t, db.Table("rm_outbox").Where("event_type IN ?", []string{originaleffect.EvaluationRetry, originaleffect.ReportInitial}).Order("event_type").Find(&originalRows).Error)
	require.Len(t, originalRows, 2)
	originals := map[string][]byte{}
	var reportPlan originaleffect.Plan
	journal := t.TempDir()
	require.NoError(t, os.Chmod(journal, 0700))
	for _, row := range originalRows {
		wire, recognized, err := legacy.Decode(row.Payload)
		require.NoError(t, err)
		require.True(t, recognized)
		originals[row.MessageID] = wire.Payload
		var identity struct {
			Data struct {
				AssessmentID uint64 `json:"assessment_id"`
			}
		}
		require.NoError(t, json.Unmarshal(wire.Payload, &identity))
		scope := []string{"--event-id=" + row.MessageID, "--assessment-id=" + strconv.FormatUint(identity.Data.AssessmentID, 10), "--org-id=1", "--accepted-before=" + time.Now().Format(time.RFC3339Nano)}
		var plan originaleffect.Plan
		require.NoError(t, json.Unmarshal(runCLI(0, scope...), &plan))
		require.Equal(t, row.MessageID, plan.EventID)
		runCLI(1, append(scope, "--org-id=2")...)
		if row.EventType == originaleffect.ReportInitial {
			reportPlan = plan
			require.Equal(t, "factor", plan.TemplateID)
			require.Equal(t, "frozen-v1", plan.TemplateVersion)
			oid, err := strconv.ParseUint(plan.OutcomeID, 10, 64)
			require.NoError(t, err)
			for _, collection := range []string{(mongoreport.ReportGenerationPO{}).CollectionName(), (mongoreport.InterpretReportPO{}).CollectionName()} {
				marker := bson.M{"_id": "owned-soft-deleted-acceptance", "outcome_id": oid, "deleted_at": time.Now()}
				_, err := mdb.Collection(collection).InsertOne(ctx, marker)
				require.NoError(t, err)
				runCLI(1, scope...)
				_, err = mdb.Collection(collection).DeleteOne(ctx, bson.M{"_id": marker["_id"]})
				require.NoError(t, err)
			}
			require.NoError(t, db.Create(&sqlreport.AdmissionFailurePO{ID: 9991, OutcomeID: oid, EventID: row.MessageID, Fingerprint: "event:" + row.MessageID, OccurredAt: time.Now(), FirstFailedAt: time.Now(), LastFailedAt: time.Now()}).Error)
			runCLI(1, scope...)
			require.NoError(t, db.Delete(&sqlreport.AdmissionFailurePO{}, 9991).Error)
		}
		id := uuid.NewString()
		apply := append(append([]string{}, scope...), "--mode=apply", "--source-fingerprint="+plan.SourceFingerprint, "--audit-dir="+journal, "--request-id="+id, "--operator=isolated-proof", "--reason=original confirmation loss and external result reviewed", "--external-result-reviewed")
		var receipt recoveryjournal.Receipt
		require.NoError(t, json.Unmarshal(runCLI(0, apply...), &receipt))
		require.Equal(t, "accepted", receipt.EffectOutcome)
		require.Equal(t, "not_sent", receipt.TransportOutcome)
		require.False(t, receipt.BusinessCompletionProven)
		require.NotEmpty(t, receipt.RunID)
		runCLI(0, "--mode=reconcile", "--audit-dir="+journal, "--request-id="+id)
		runCLI(1, append(apply, "--request-id="+uuid.NewString())...)
		runCLI(1, scope...)
	}
	require.EqualValues(t, 1, builder.calls.Load())
	effectAssertBroker(t, 0, 0)
	manager, err := grpcclient.NewManager(&grpcclient.ManagerConfig{Endpoint: lis.Addr().String(), Timeout: 10 * time.Second, TLS: grpcclient.TLSConfig{CAFile: ca.CAFile, CertFile: worker.CertFile, KeyFile: worker.KeyFile, ServerName: "server.test"}})
	require.NoError(t, err)
	defer manager.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dispatcher := workereventing.NewDispatcher(logger, &workereventing.HandlerDependencies{Logger: logger, EvaluationWorkerClient: grpcclient.NewEvaluationWorkerClient(manager), InterpretationAutomationClient: grpcclient.NewInterpretationAutomationClient(manager)}, handlers.NewRegistry())
	cfg, err := catalog.Parse([]byte("version: '1'\ntopics:\n  evaluation:\n    name: qs.evaluation.lifecycle\nevents:\n  evaluation.retry.requested:\n    topic: evaluation\n    delivery: durable_outbox\n    aggregate: Evaluation\n    domain: evaluation\n    handler: evaluation_requested_handler\n  evaluation.outcome.committed:\n    topic: evaluation\n    delivery: durable_outbox\n    aggregate: Evaluation\n    domain: evaluation\n    handler: evaluation_outcome_committed_handler\n"))
	require.NoError(t, err)
	require.NoError(t, dispatcher.Initialize(catalog.NewCatalog(cfg)))
	nsqCfg := nsq.NewConfig()
	nsqCfg.HeartbeatInterval = time.Second
	nsqCfg.ReadTimeout = 3 * time.Second
	nsqCfg.WriteTimeout = time.Second
	sub, err := sdknsq.NewSubscriber(sdknsq.SubscriberConfig{NSQDAddresses: []string{tcp}, Driver: nsqCfg, MaxInFlight: 1, MaxAttempts: 3, DeliveryContext: ctx})
	require.NoError(t, err)
	defer func() {
		shutdown, stop := context.WithTimeout(context.Background(), 6*time.Second)
		defer stop()
		require.NoError(t, sub.Close(shutdown))
	}()
	adapter := &effectNormalSubscriber{sub: sub, ctx: ctx, originals: originals}
	require.NoError(t, workermessaging.SubscribeSDKHandlersWithOptions(workermessaging.SubscribeSDKHandlersOptions{ServiceName: effectProofChannel, Logger: logger, Runtime: dispatcher, Subscriber: adapter, UnknownRecorder: func(context.Context, transport.Received, string) error { return errors.New("unexpected unknown event") }}))
	producer, err := nsq.NewProducer(tcp, nsqCfg)
	require.NoError(t, err)
	producer.SetLogger(nil, nsq.LogLevelError)
	defer producer.Stop()
	for _, row := range originalRows {
		for range 2 {
			require.NoError(t, producer.Publish("qs.evaluation.lifecycle", row.Payload))
		}
	}
	effectAssertBroker(t, 4, 4)
	require.EqualValues(t, 4, adapter.deliveries.Load())
	require.Zero(t, adapter.failed.Load())
	require.EqualValues(t, 1, builder.calls.Load())
	var after []struct {
		MessageID, EventType  string
		Payload               []byte
		Version, AttemptCount uint64
	}
	require.NoError(t, db.Table("rm_outbox").Where("event_type IN ?", []string{originaleffect.EvaluationRetry, originaleffect.ReportInitial}).Order("event_type").Find(&after).Error)
	require.Equal(t, originalRows, after)
	oid, err := strconv.ParseUint(reportPlan.OutcomeID, 10, 64)
	require.NoError(t, err)
	for _, collection := range []string{(mongoreport.ReportGenerationPO{}).CollectionName(), (mongoreport.InterpretReportPO{}).CollectionName()} {
		count, err := mdb.Collection(collection).CountDocuments(ctx, bson.M{"outcome_id": oid})
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
	}
	for _, row := range originalRows {
		if row.EventType == originaleffect.EvaluationRetry {
			wire, _, err := legacy.Decode(row.Payload)
			require.NoError(t, err)
			var e struct {
				Data struct {
					AssessmentID uint64 `json:"assessment_id"`
				}
			}
			require.NoError(t, json.Unmarshal(wire.Payload, &e))
			var count int64
			require.NoError(t, db.Table("runtime_checkpoint").Where("scope=? AND assessment_id=?", "evaluation_run", e.Data.AssessmentID).Count(&count).Error)
			require.EqualValues(t, 2, count)
		}
	}
	t.Logf("recovery_pub=0 original_retry_grant=unchanged successor_runs=1 report_builder=1 generations=1 artifacts=1 late_originals=4 original_published_rows=unchanged template=%s@%s", reportPlan.TemplateID, reportPlan.TemplateVersion)
}

func effectAssertBroker(t *testing.T, published, finished int64) {
	t.Helper()
	address := os.Getenv("RM_EFFECT_NSQ_HTTP")
	require.True(t, strings.HasPrefix(address, "http://127.0.0.1:"))
	require.Eventually(t, func() bool {
		response, err := (&http.Client{Timeout: time.Second}).Get(address + "/stats?format=json&topic=qs.evaluation.lifecycle")
		if err != nil {
			return false
		}
		defer response.Body.Close()
		var stats struct {
			Topics []struct {
				MessageCount int64 `json:"message_count"`
				Channels     []struct {
					ChannelName     string `json:"channel_name"`
					Depth, Requeues int64
					InFlight        int64 `json:"in_flight_count"`
					FinishCount     int64 `json:"finish_count"`
				} `json:"channels"`
			} `json:"topics"`
		}
		if json.NewDecoder(response.Body).Decode(&stats) != nil || len(stats.Topics) != 1 || stats.Topics[0].MessageCount != published {
			return false
		}
		for _, c := range stats.Topics[0].Channels {
			if c.ChannelName == effectProofChannel {
				return c.Depth == 0 && c.InFlight == 0 && c.FinishCount == finished
			}
		}
		return false
	}, 10*time.Second, 30*time.Millisecond)
}
