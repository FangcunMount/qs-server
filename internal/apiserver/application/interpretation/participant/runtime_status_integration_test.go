//go:build integration && reliable_messaging_m4

package participant_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	basegrpc "github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	automation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
	execution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	frozeninput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/input"
	participant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
	transaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	outcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	generation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	irun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	mongointerpretation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	mysqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	evaluation "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	grpcclient "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
	sqlDriver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// This proof uses the actual MySQL Outcome adapter, Mongo lifecycle transaction,
// signed gRPC server/client and Collection BFF. Only the authenticated workload
// identity is supplied by the isolated transport fixture; no production token.
type sqlRuntimeFacts struct{ source outcome.Repository }

func (r sqlRuntimeFacts) FindByID(ctx context.Context, id meta.ID) (*evaluationfact.Record, error) {
	v, e := r.source.FindByID(ctx, id)
	return evaloutcome.FactRecord(v), e
}
func (r sqlRuntimeFacts) FindByAssessmentID(ctx context.Context, id meta.ID) (*evaluationfact.Record, error) {
	v, e := r.source.FindByAssessmentID(ctx, id)
	return evaloutcome.FactRecord(v), e
}

type sqlRuntimeOwnership struct{ facts sqlRuntimeFacts }

func (a sqlRuntimeOwnership) AuthorizeParticipant(context.Context, participant.Actor) error {
	return nil
}
func (a sqlRuntimeOwnership) AuthorizeOwnAssessment(ctx context.Context, testee, assessment uint64) error {
	v, e := a.facts.FindByAssessmentID(ctx, meta.FromUint64(assessment))
	if e != nil {
		return e
	}
	if v == nil || v.TesteeID() != testee {
		return status.Error(codes.PermissionDenied, "not owned")
	}
	return nil
}

type runtimeWaitRead struct {
	facts  sqlRuntimeFacts
	bff    *grpcbridge.EvaluationBFFReader
	access sqlRuntimeOwnership
}

func (r runtimeWaitRead) AuthorizeAssessment(ctx context.Context, testee, id uint64) error {
	return r.access.AuthorizeOwnAssessment(ctx, testee, id)
}
func (r runtimeWaitRead) GetMyAssessment(ctx context.Context, _, id uint64) (*evaluation.AssessmentDetailResponse, error) {
	v, e := r.facts.FindByAssessmentID(ctx, meta.FromUint64(id))
	if e != nil || v == nil {
		return nil, e
	}
	return &evaluation.AssessmentDetailResponse{ID: v.AssessmentID().String(), Status: "evaluated"}, nil
}
func (r runtimeWaitRead) GetMyAssessmentRunStatus(context.Context, uint64, uint64) (*evaluation.AssessmentRuntimeStatusResponse, error) {
	return nil, fmt.Errorf("Evaluation run read is outside report phase")
}
func (r runtimeWaitRead) GetAssessmentReport(ctx context.Context, testee, id uint64) (*evaluation.AssessmentReportResponse, error) {
	return r.bff.GetAssessmentReport(ctx, testee, id)
}
func (r runtimeWaitRead) GetAssessmentReportStatus(ctx context.Context, testee, id uint64) (*evaluation.ReportRuntimeStatusResponse, error) {
	return r.bff.GetAssessmentReportStatus(ctx, testee, id)
}

type persistentRuntimeHint struct{ snapshot *reportstatus.Snapshot }

func (c *persistentRuntimeHint) Get(context.Context, string) (*reportstatus.Snapshot, error) {
	return c.snapshot, nil
}
func (c *persistentRuntimeHint) Set(_ context.Context, s *reportstatus.Snapshot, _ time.Duration) error {
	c.snapshot = s
	return nil
}
func (c *persistentRuntimeHint) SetIfHigherPriority(ctx context.Context, s *reportstatus.Snapshot, ttl time.Duration) error {
	return c.Set(ctx, s, ttl)
}

func TestPersistentReportRuntimeReadbackWithoutFailureNotification(t *testing.T) {
	dsn := os.Getenv("QS_M6_REPORT_RUNTIME_MYSQL_DSN")
	parsed, err := sqlDriver.ParseDSN(dsn)
	if err != nil || parsed == nil || parsed.DBName != "qs_m6_report_runtime_test_isolated" || parsed.User != "root" || parsed.Passwd != "" {
		t.Fatal("explicit isolated MySQL DSN required; no SKIP or application database fallback")
	}
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := sqlDB.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&mysqlevaluation.EvaluationOutcomePO{}); err != nil {
		t.Fatal(err)
	}
	_, mdb := mongodbtest.ReplicaSetDatabase(t)
	gens, err := mongointerpretation.NewGenerationRepository(mdb)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := mongointerpretation.NewRunRepository(mdb)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := mongointerpretation.NewReportRepository(mdb)
	if err != nil {
		t.Fatal(err)
	}
	reportCatalog, err := mongointerpretation.NewReportCatalogProjector(mdb)
	if err != nil {
		t.Fatal(err)
	}
	if err := mdb.CreateCollection(t.Context(), "rm_outbox"); err != nil {
		t.Fatal(err)
	}
	wire, err := catalog.Load("../../../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stager, err := mongostandard.NewStager(mdb.Collection("rm_outbox"), catalog.NewCatalog(wire), "qs-apiserver")
	if err != nil {
		t.Fatal(err)
	}
	tx := transaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		s, e := mdb.Client().StartSession()
		if e != nil {
			return e
		}
		defer s.EndSession(ctx)
		_, e = s.WithTransaction(ctx, func(sc mongo.SessionContext) (any, error) { return nil, fn(sc) })
		return e
	})
	starter, err := execution.NewStarter(tx, gens, runs, reports, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	committer, err := execution.NewInterpretationCommitter(tx, gens, runs, reports, stager, nil, reportCatalog)
	if err != nil {
		t.Fatal(err)
	}
	facts := sqlRuntimeFacts{source: mysqlevaluation.NewOutcomeRepository(db)}
	access := sqlRuntimeOwnership{facts: facts}
	options := &delegatedsubject.Options{Enabled: true, CurrentKey: "isolated-runtime-key", TTL: time.Minute}
	verifier, err := delegatedsubject.NewVerifierFromOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := delegatedsubject.NewSignerFromOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(basegrpc.ContextWithServiceIdentity(ctx, &basegrpc.ServiceIdentity{ServiceName: serviceidentity.CollectionServerCertificateCommonName, CommonName: serviceidentity.CollectionServerCertificateCommonName}), req)
	}))
	grpcservice.NewParticipantReportService(participant.NewService(mongointerpretation.NewReportReadModel(mdb), access), verifier, participant.NewRuntimeStatusReader(access, facts, gens, runs)).RegisterService(server)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	base, err := grpcclient.NewClient(&grpcclient.ClientConfig{Endpoint: "passthrough:///runtime-proof", Timeout: 2 * time.Second, Insecure: true}, grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = base.Close() })
	client := grpcclient.NewParticipantReportClient(base, signer)
	bff := grpcbridge.NewEvaluationBFFReader(nil, client, nil)
	ctx := context.WithValue(t.Context(), middleware.UserClaimsContextKey{}, &middleware.UserClaims{UserID: "proof-user", OrgID: "1"})
	original, outboxPolicy := retrygovernance.BusinessPolicy(), retrygovernance.OutboxPolicy()
	proof := original
	proof.MaxAutomaticAttempts = 1
	proof.Version = "bounded-report-readback-proof"
	if err := retrygovernance.ConfigurePolicies(proof, outboxPolicy); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := retrygovernance.ConfigurePolicies(original, outboxPolicy); e != nil {
			t.Error(e)
		}
	})
	for _, tc := range []struct {
		name      string
		retryable bool
		want      string
	}{{"manual", true, "temporarily_unavailable"}, {"terminal", false, "failed"}} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{Assets: &interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "INTJ", Title: "INTJ", Summary: "frozen"}}, Profiles: []interpretationassets.TypeProfilePresentation{{OutcomeCode: "INTJ", Commentary: "frozen"}}, ReportSpec: interpretationassets.ReportSpec{Sections: []interpretationassets.ReportSection{{Code: "personality", Kind: "template", AdapterKey: "personality_type", TemplateID: "personality", TemplateVersion: "frozen-v3"}}}}, ModelRef: evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindTypology, Algorithm: string(modelcatalog.AlgorithmPersonalityTypology), Code: "PERSONALITY", Version: "1.0.0"}, DecisionKind: modelcatalog.DecisionKindPoleComposition, TypologyRouting: &evaluationinput.TypologyRoutingFreeze{DecisionKind: string(modelcatalog.DecisionKindPoleComposition), ReportKind: "template", AdapterKey: "personality_type", TemplateID: "personality", TemplateVersion: "frozen-v3"}})
			if err != nil {
				t.Fatal(err)
			}
			oid, aid := meta.New(), meta.New()
			record, err := outcome.NewRecord(outcome.NewRecordInput{ID: oid, OrgID: 1, AssessmentID: aid, TesteeID: 2, RunID: aid.String() + ":1", Model: outcome.ModelIdentity{Kind: modelcatalog.KindTypology, Algorithm: modelcatalog.AlgorithmPersonalityTypology, Code: "PERSONALITY", Version: "1.0.0"}, Runtime: outcome.RuntimeIdentity{DecisionKind: modelcatalog.DecisionKindPoleComposition}, ReportInput: raw, SchemaVersion: 2, EvaluatedAt: time.Now(), Payload: []byte(`{"Detail":{"Payload":{"type_code":"INTJ","match_percent":80}},"Primary":{"Kind":"match_percent","Value":80,"Label":"INTJ"},"Profile":{"Kind":"personality_type","Code":"INTJ"}}`)})
			if err != nil {
				t.Fatal(err)
			}
			if err := facts.source.Save(ctx, record); err != nil {
				t.Fatal(err)
			}
			persisted, err := facts.FindByAssessmentID(ctx, aid)
			if err != nil {
				t.Fatal(err)
			}
			input, err := frozeninput.FromOutcomeRecord(persisted)
			if err != nil {
				t.Fatal(err)
			}
			key := generation.Key{OutcomeID: oid, ReportType: input.Report.ReportType, TemplateVersion: input.Report.TemplateVersion}
			started, err := starter.Start(ctx, execution.StartRequest{Key: key, TraceID: "runtime-proof"})
			if err != nil {
				t.Fatal(err)
			}
			failed, err := committer.CommitFailure(ctx, execution.CommitFailureRequest{Generation: started.Generation, Run: started.Run, OutcomeID: oid, Association: domainreport.Association{OrgID: 1, AssessmentID: aid, TesteeID: 2}, Failure: irun.Failure{Kind: irun.FailureKindBuild, Code: "controlled_failure", SafeMessage: "do not expose", Retryable: tc.retryable}, FailedAt: time.Now()})
			if err != nil {
				t.Fatal(err)
			}
			// No Relay, subscriber, failure notification or Redis state is supplied.
			count, err := mdb.Collection("rm_outbox").CountDocuments(ctx, bson.M{"event_type": catalog.InterpretationReportFailed})
			if err != nil || count < 1 {
				t.Fatalf("durable failure intent missing count=%d err=%v", count, err)
			}
			hint := &persistentRuntimeHint{}
			wait := reportwait.NewService(runtimeWaitRead{facts: facts, bff: bff, access: access}, hint, nil, nil, reportwait.DefaultConfig())
			got, err := wait.GetStatus(ctx, 2, aid.Uint64())
			if err != nil || got == nil || got.Status != tc.want {
				t.Fatalf("without notification got=%+v err=%v", got, err)
			}
			if _, err := client.GetAssessmentReportStatus(ctx, 3, aid.Uint64()); status.Code(err) != codes.PermissionDenied {
				t.Fatalf("unowned runtime read allowed err=%v", err)
			}
			if tc.retryable {
				authorized, err := automation.NewGovernedRetryService(gens, runs, facts, tx, stager).Authorize(ctx, automation.GovernedRetryCommand{OrgID: 1, GenerationID: failed.Generation.ID(), ExpectedAttempt: 1, Origin: retrygovernance.AttemptOriginManual, RequestID: "readback-" + aid.String(), Reason: "controlled original-task authorization"})
				if err != nil {
					t.Fatal(err)
				}
				decision := authorized.RetryDecision()
				authCtx := retrygovernance.WithAuthorization(ctx, retrygovernance.Authorization{ExpectedAttempt: 1, EventID: decision.RetryEventID, ActionRequestID: decision.ActionRequestID, Origin: retrygovernance.AttemptOriginManual})
				next, err := starter.Start(authCtx, execution.StartRequest{Key: key, TraceID: "original-frozen-retry"})
				if err != nil || next.Status != execution.StartStatusStarted || next.Run.Attempt() != 2 {
					t.Fatalf("governed original retry=%+v err=%v", next, err)
				}
				got, err = wait.GetStatus(ctx, 2, aid.Uint64())
				if err != nil || got == nil || got.Status != "processing" || got.Stage != "interpreting" {
					t.Fatalf("old manual hint hid latest running attempt got=%+v err=%v", got, err)
				}
				latest, err := runs.FindByID(ctx, next.Run.ID())
				if err != nil || latest.Status() != irun.StatusRunning {
					t.Fatalf("query mutated latest run=%+v err=%v", latest, err)
				}
			}
			if key.TemplateVersion.String() != "frozen-v3" {
				t.Fatal("live template substituted")
			}
		})
	}
}
