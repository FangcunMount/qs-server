//go:build integration

package interpretation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	appeventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	execution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	mongoeventoutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventoutbox"
	mongointerpretation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestInterpretationSuccessRollsBackEveryPersistedBoundary(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	fixture := newInterpretationMongoFixture(t, db)

	for _, fault := range []string{"artifact", "catalog", "run", "generation", "outbox", "commit"} {
		fault := fault
		t.Run(fault, func(t *testing.T) {
			generation, run := fixture.start(t)
			artifact := integrationArtifact(t, generation, run, fixture.now)
			postCommit := &postCommitCounter{}
			runner := fixture.runner
			if fault == "commit" {
				runner = cancelInterpretationCommitRunner{inner: runner}
			}
			committer, err := execution.NewInterpretationCommitter(
				runner,
				faultGenerationRepo{Repository: fixture.generations, fault: fault},
				faultRunRepo{Repository: fixture.runs, fault: fault},
				faultReportRepo{ReportRepository: fixture.reports, fault: fault},
				faultEventStager{inner: fixture.outbox, fault: fault},
				postCommit,
				faultCatalogProjector{inner: fixture.catalog, fault: fault},
			)
			if err != nil {
				t.Fatal(err)
			}
			_, err = committer.CommitSuccess(t.Context(), execution.CommitSuccessRequest{
				Generation: generation, Run: run, InterpretReport: artifact,
				BuilderIdentity:      domainreport.BuilderIdentityFactorScoring,
				ContentSchemaVersion: domainreport.ContentSchemaVersionV1,
				CompletedAt:          fixture.now,
			})
			if err == nil {
				t.Fatalf("success commit fault %s returned nil error", fault)
			}
			if postCommit.calls != 0 {
				t.Fatalf("post-commit calls = %d, want 0", postCommit.calls)
			}
			fixture.assertRunningAndNoTerminalWrites(t, generation, run, artifact)
			if generation.Status() != domaingeneration.StatusGenerating || run.Status() != interpretationrun.StatusRunning {
				t.Fatalf("caller state polluted: generation=%s run=%s", generation.Status(), run.Status())
			}
		})
	}
}

func TestInterpretationFailureAndScheduledRetryRollBackTogether(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	fixture := newInterpretationMongoFixture(t, db)

	for _, fault := range []string{"run", "generation", "outbox", "commit"} {
		fault := fault
		t.Run(fault, func(t *testing.T) {
			generation, run := fixture.start(t)
			runner := fixture.runner
			if fault == "commit" {
				runner = cancelInterpretationCommitRunner{inner: runner}
			}
			committer, err := execution.NewInterpretationCommitter(
				runner,
				faultGenerationRepo{Repository: fixture.generations, fault: fault},
				faultRunRepo{Repository: fixture.runs, fault: fault},
				fixture.reports,
				faultEventStager{inner: fixture.outbox, fault: fault},
				nil,
				fixture.catalog,
			)
			if err != nil {
				t.Fatal(err)
			}
			_, err = committer.CommitFailure(t.Context(), execution.CommitFailureRequest{
				Generation: generation, Run: run, OutcomeID: generation.Key().OutcomeID,
				Association: domainreport.Association{OrgID: 1, AssessmentID: meta.New(), TesteeID: 8},
				Failure: interpretationrun.Failure{
					Kind: interpretationrun.FailureKindBuild, Code: "build_failed", SafeMessage: "failed", Retryable: true,
				},
				FailedAt: fixture.now,
			})
			if err == nil {
				t.Fatalf("failure commit fault %s returned nil error", fault)
			}
			fixture.assertRunningAndNoOutbox(t, generation, run)
			if generation.Status() != domaingeneration.StatusGenerating || run.Status() != interpretationrun.StatusRunning {
				t.Fatalf("caller failure state polluted: generation=%s run=%s", generation.Status(), run.Status())
			}
		})
	}
}

type interpretationMongoFixture struct {
	db          *mongo.Database
	runner      apptransaction.Runner
	generations *mongointerpretation.GenerationRepository
	runs        *mongointerpretation.RunRepository
	reports     *mongointerpretation.ReportRepository
	catalog     *mongointerpretation.ReportCatalogProjector
	outbox      *mongoeventoutbox.Store
	now         time.Time
}

func newInterpretationMongoFixture(t *testing.T, db *mongo.Database) interpretationMongoFixture {
	t.Helper()
	generations, err := mongointerpretation.NewGenerationRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := mongointerpretation.NewRunRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := mongointerpretation.NewReportRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := mongointerpretation.NewReportCatalogProjector(db)
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := mongoeventoutbox.NewStoreWithTopicResolver(db, interpretationTopicResolver{})
	if err != nil {
		t.Fatal(err)
	}
	return interpretationMongoFixture{
		db: db, runner: mongoInterpretationRunner(db), generations: generations, runs: runs,
		reports: reports, catalog: catalog, outbox: outbox,
		now: time.Date(2026, 8, 15, 13, 0, 0, 0, time.UTC),
	}
}

func (f interpretationMongoFixture) start(t *testing.T) (*domaingeneration.ReportGeneration, *interpretationrun.InterpretationRun) {
	t.Helper()
	starter, err := execution.NewStarter(f.runner, f.generations, f.runs, f.reports, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	result, err := starter.Start(t.Context(), execution.StartRequest{
		Key:     domaingeneration.Key{OutcomeID: meta.New(), ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersion("v1")},
		TraceID: "integration-trace",
	})
	if err != nil {
		t.Fatal(err)
	}
	return result.Generation, result.Run
}

func (f interpretationMongoFixture) assertRunningAndNoTerminalWrites(t *testing.T, generation *domaingeneration.ReportGeneration, run *interpretationrun.InterpretationRun, artifact *domainreport.InterpretReport) {
	t.Helper()
	f.assertRunningAndNoOutbox(t, generation, run)
	assertMongoDocumentCount(t, f.db.Collection("interpret_report_artifacts"), bson.M{"domain_id": artifact.ID().Uint64()}, 0)
	assertMongoDocumentCount(t, f.db.Collection("report_query_catalog"), bson.M{"generation_id": generation.ID().Uint64()}, 0)
}

func (f interpretationMongoFixture) assertRunningAndNoOutbox(t *testing.T, generation *domaingeneration.ReportGeneration, run *interpretationrun.InterpretationRun) {
	t.Helper()
	persistedGeneration, err := f.generations.FindByID(t.Context(), generation.ID())
	if err != nil {
		t.Fatal(err)
	}
	persistedRun, err := f.runs.FindByID(t.Context(), run.ID())
	if err != nil {
		t.Fatal(err)
	}
	if persistedGeneration.Status() != domaingeneration.StatusGenerating || persistedRun.Status() != interpretationrun.StatusRunning {
		t.Fatalf("persisted state = generation:%s run:%s, want generating/running", persistedGeneration.Status(), persistedRun.Status())
	}
	assertMongoDocumentCount(t, f.db.Collection("domain_event_outbox"), bson.M{"aggregate_id": generation.ID().String()}, 0)
}

type faultGenerationRepo struct {
	domaingeneration.Repository
	fault string
}

func (r faultGenerationRepo) Save(ctx context.Context, generation *domaingeneration.ReportGeneration, expected uint64) error {
	if r.fault == "generation" {
		return errors.New("injected generation failure")
	}
	return r.Repository.Save(ctx, generation, expected)
}

type faultRunRepo struct {
	interpretationrun.Repository
	fault string
}

func (r faultRunRepo) Save(ctx context.Context, run *interpretationrun.InterpretationRun) error {
	if r.fault == "run" {
		return errors.New("injected run failure")
	}
	return r.Repository.Save(ctx, run)
}

type faultReportRepo struct {
	domainreport.ReportRepository
	fault string
}

func (r faultReportRepo) Insert(ctx context.Context, report *domainreport.InterpretReport) error {
	if r.fault == "artifact" {
		return errors.New("injected artifact failure")
	}
	return r.ReportRepository.Insert(ctx, report)
}

type faultCatalogProjector struct {
	inner execution.ReportCatalogProjector
	fault string
}

func (p faultCatalogProjector) ProjectCurrent(ctx context.Context, report *domainreport.InterpretReport) error {
	if p.fault == "catalog" {
		return errors.New("injected catalog failure")
	}
	return p.inner.ProjectCurrent(ctx, report)
}

type faultEventStager struct {
	inner *mongoeventoutbox.Store
	fault string
}

func (s faultEventStager) Stage(ctx context.Context, events ...event.DomainEvent) error {
	if s.fault == "outbox" {
		return errors.New("injected outbox failure")
	}
	return s.inner.Stage(ctx, events...)
}

func (s faultEventStager) StageAt(ctx context.Context, readyAt time.Time, events ...event.DomainEvent) error {
	if s.fault == "outbox" {
		return errors.New("injected scheduled outbox failure")
	}
	return s.inner.StageAt(ctx, readyAt, events...)
}

type postCommitCounter struct{ calls int }

var _ appeventing.PostCommitDispatcher = (*postCommitCounter)(nil)

func (c *postCommitCounter) AfterCommit(context.Context, []event.DomainEvent, time.Time) { c.calls++ }

type interpretationTopicResolver struct{}

func (interpretationTopicResolver) GetTopicForEvent(string) (string, bool) {
	return "integration.interpretation", true
}

type cancelInterpretationCommitRunner struct{ inner apptransaction.Runner }

func (r cancelInterpretationCommitRunner) WithinTransaction(ctx context.Context, callback func(context.Context) error) error {
	txCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	return r.inner.WithinTransaction(txCtx, func(sessionCtx context.Context) error {
		if err := callback(sessionCtx); err != nil {
			return err
		}
		cancel()
		return nil
	})
}

func mongoInterpretationRunner(db *mongo.Database) apptransaction.Runner {
	return apptransaction.RunnerFunc(func(ctx context.Context, callback func(context.Context) error) error {
		session, err := db.Client().StartSession()
		if err != nil {
			return err
		}
		defer session.EndSession(ctx)
		_, err = session.WithTransaction(ctx, func(sessionCtx mongo.SessionContext) (any, error) {
			return nil, callback(sessionCtx)
		})
		return err
	})
}

func integrationArtifact(t *testing.T, generation *domaingeneration.ReportGeneration, run *interpretationrun.InterpretationRun, at time.Time) *domainreport.InterpretReport {
	t.Helper()
	artifact, err := domainreport.NewInterpretReport(domainreport.InterpretReportInput{
		ID: meta.New(), GenerationID: generation.ID(), OutcomeID: generation.Key().OutcomeID, InterpretationRunID: run.ID(),
		Association: domainreport.Association{OrgID: 1, AssessmentID: meta.New(), TesteeID: 8},
		ReportType:  policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersion("v1"),
		BuilderIdentity: domainreport.BuilderIdentityFactorScoring, ContentSchemaVersion: domainreport.ContentSchemaVersionV1,
		Content: domainreport.Content{
			Model:        domainreport.ModelIdentity{Kind: "scale", Code: "CONTRACT", Version: "v1"},
			PrimaryScore: domainreport.NewRawTotalScore(8, nil),
			Dimensions: []domainreport.DimensionInterpret{
				domainreport.NewDimensionInterpret(domainreport.NewFactorCode("TOTAL"), "total", 8, nil, domainreport.RiskLevelLow, "ok", "ok"),
			},
		},
		GeneratedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func assertMongoDocumentCount(t *testing.T, collection *mongo.Collection, filter bson.M, want int64) {
	t.Helper()
	got, err := collection.CountDocuments(t.Context(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d (filter=%v)", collection.Name(), got, want, filter)
	}
}
