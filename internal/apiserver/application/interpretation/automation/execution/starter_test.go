package execution

import (
	"context"
	"sort"
	"testing"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type starterTx struct{ calls int }

func (t *starterTx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	t.calls++
	return fn(ctx)
}

var _ apptransaction.Runner = (*starterTx)(nil)

type memoryGenerationRepo struct {
	items     map[domaingeneration.Key]*domaingeneration.ReportGeneration
	versions  map[meta.ID]uint64
	conflict  func()
	findCalls int
}

func newMemoryGenerationRepo() *memoryGenerationRepo {
	return &memoryGenerationRepo{items: map[domaingeneration.Key]*domaingeneration.ReportGeneration{}, versions: map[meta.ID]uint64{}}
}

func (r *memoryGenerationRepo) put(item *domaingeneration.ReportGeneration) {
	r.items[item.Key()] = item
	r.versions[item.ID()] = item.Version()
}

func (r *memoryGenerationRepo) Create(_ context.Context, item *domaingeneration.ReportGeneration) error {
	if r.conflict != nil {
		fn := r.conflict
		r.conflict = nil
		fn()
		return domaingeneration.ErrAlreadyExists
	}
	if _, ok := r.items[item.Key()]; ok {
		return domaingeneration.ErrAlreadyExists
	}
	r.put(item)
	return nil
}

func (r *memoryGenerationRepo) FindByID(_ context.Context, id domaingeneration.ID) (*domaingeneration.ReportGeneration, error) {
	for _, item := range r.items {
		if item.ID() == id {
			return item, nil
		}
	}
	return nil, domaingeneration.ErrNotFound
}

func (r *memoryGenerationRepo) FindByKey(_ context.Context, key domaingeneration.Key) (*domaingeneration.ReportGeneration, error) {
	r.findCalls++
	if item, ok := r.items[key]; ok {
		return item, nil
	}
	return nil, domaingeneration.ErrNotFound
}

func (r *memoryGenerationRepo) ListByOutcomeID(_ context.Context, outcomeID domaingeneration.ID) ([]*domaingeneration.ReportGeneration, error) {
	items := make([]*domaingeneration.ReportGeneration, 0)
	for _, item := range r.items {
		if item.Key().OutcomeID == outcomeID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (r *memoryGenerationRepo) Save(_ context.Context, item *domaingeneration.ReportGeneration, expected uint64) error {
	if r.versions[item.ID()] != expected {
		return domaingeneration.ErrVersionConflict
	}
	r.put(item)
	return nil
}

type memoryRunRepo struct {
	items   map[meta.ID]*interpretationrun.InterpretationRun
	creates int
	saves   int
}

func newMemoryRunRepo() *memoryRunRepo {
	return &memoryRunRepo{items: map[meta.ID]*interpretationrun.InterpretationRun{}}
}

func (r *memoryRunRepo) Create(_ context.Context, item *interpretationrun.InterpretationRun) error {
	r.creates++
	if _, ok := r.items[item.ID()]; ok {
		return interpretationrun.ErrAlreadyExists
	}
	r.items[item.ID()] = item
	return nil
}

func (r *memoryRunRepo) FindByID(_ context.Context, id interpretationrun.ID) (*interpretationrun.InterpretationRun, error) {
	if item, ok := r.items[id]; ok {
		return item, nil
	}
	return nil, interpretationrun.ErrNotFound
}

func (r *memoryRunRepo) FindLatestByGenerationID(_ context.Context, id interpretationrun.ID) (*interpretationrun.InterpretationRun, error) {
	items := make([]*interpretationrun.InterpretationRun, 0)
	for _, item := range r.items {
		if item.GenerationID() == id {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return nil, interpretationrun.ErrNotFound
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Attempt() > items[j].Attempt() })
	return items[0], nil
}

func (r *memoryRunRepo) ListByGenerationID(ctx context.Context, id interpretationrun.ID) ([]*interpretationrun.InterpretationRun, error) {
	item, err := r.FindLatestByGenerationID(ctx, id)
	if err != nil {
		return nil, err
	}
	return []*interpretationrun.InterpretationRun{item}, nil
}

func (r *memoryRunRepo) Save(_ context.Context, item *interpretationrun.InterpretationRun) error {
	r.saves++
	if _, ok := r.items[item.ID()]; !ok {
		return interpretationrun.ErrNotFound
	}
	r.items[item.ID()] = item
	return nil
}

type memoryArtifactRepo struct {
	items map[meta.ID]*domainreport.InterpretReport
}

func (r *memoryArtifactRepo) Insert(_ context.Context, item *domainreport.InterpretReport) error {
	r.items[item.ID()] = item
	return nil
}
func (r *memoryArtifactRepo) FindByID(_ context.Context, id meta.ID) (*domainreport.InterpretReport, error) {
	if item, ok := r.items[id]; ok {
		return item, nil
	}
	return nil, domainreport.ErrInterpretReportNotFound
}
func (r *memoryArtifactRepo) FindByGenerationID(_ context.Context, id meta.ID) (*domainreport.InterpretReport, error) {
	for _, item := range r.items {
		if item.GenerationID() == id {
			return item, nil
		}
	}
	return nil, domainreport.ErrInterpretReportNotFound
}

func (r *memoryArtifactRepo) ListByAssessmentID(_ context.Context, assessmentID meta.ID) ([]*domainreport.InterpretReport, error) {
	items := make([]*domainreport.InterpretReport, 0)
	for _, item := range r.items {
		if item.Association().AssessmentID == assessmentID {
			items = append(items, item)
		}
	}
	return items, nil
}

func TestStarterCreatesGenerationAndRunningRunAtomically(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	service, generations, runs, tx := newStarterFixture(t, now)
	result, err := service.Start(context.Background(), StartRequest{Key: starterKey(), TraceID: "trace-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StartStatusStarted || result.Generation.Status() != domaingeneration.StatusGenerating || result.Run.Status() != interpretationrun.StatusRunning || !result.Run.HasActiveLease(now) {
		t.Fatalf("start result = %#v", result)
	}
	if tx.calls != 1 || len(generations.items) != 1 || runs.creates != 1 {
		t.Fatalf("transaction writes tx=%d generations=%d runs=%d", tx.calls, len(generations.items), runs.creates)
	}
}

func TestStarterReturnsProcessingForActiveLeaseAndRereadsUniqueConflict(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	service, generations, runs, tx := newStarterFixture(t, now)
	winnerGeneration, winnerRun := seedGenerating(t, now, time.Minute)
	runs.items[winnerRun.ID()] = winnerRun
	generations.conflict = func() { generations.put(winnerGeneration) }

	result, err := service.Start(context.Background(), StartRequest{Key: starterKey()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StartStatusProcessing || result.Generation != winnerGeneration || generations.findCalls < 2 || tx.calls != 1 || runs.creates != 0 {
		t.Fatalf("conflict/process result=%#v finds=%d tx=%d creates=%d", result, generations.findCalls, tx.calls, runs.creates)
	}
}

func TestStarterClosesStaleRunAndStartsNextAttemptInOneTransaction(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	service, generations, runs, tx := newStarterFixture(t, now)
	generationRecord, staleRun := seedGenerating(t, now.Add(-2*time.Minute), time.Minute)
	generations.put(generationRecord)
	runs.items[staleRun.ID()] = staleRun

	result, err := service.Start(context.Background(), StartRequest{Key: starterKey(), TraceID: "recovery"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StartStatusStarted || result.Run.Attempt() != 2 || result.Generation.Status() != domaingeneration.StatusGenerating || result.Generation.LatestRunID() != result.Run.ID() {
		t.Fatalf("recovery result = %#v", result)
	}
	if staleRun.Status() != interpretationrun.StatusFailed || staleRun.Failure() == nil || staleRun.Failure().Code != "lease_expired" {
		t.Fatalf("stale run not closed = %#v", staleRun)
	}
	if tx.calls != 1 || runs.saves != 1 || runs.creates != 1 || generationRecord.Version() != 4 {
		t.Fatalf("recovery writes tx=%d saves=%d creates=%d generation_version=%d", tx.calls, runs.saves, runs.creates, generationRecord.Version())
	}
}

func TestStarterReturnsReportForGeneratedGeneration(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	service, generations, runs, tx := newStarterFixture(t, now)
	generationRecord, runRecord := seedGenerating(t, now, time.Minute)
	if err := runRecord.Succeed(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := generationRecord.Succeed(runRecord.ID(), meta.FromUint64(99), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	generations.put(generationRecord)
	runs.items[runRecord.ID()] = runRecord
	artifact := testArtifact(t, generationRecord, runRecord, meta.FromUint64(99), now)
	service.(*starter).reports.(*memoryArtifactRepo).items[artifact.ID()] = artifact

	result, err := service.Start(context.Background(), StartRequest{Key: starterKey()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StartStatusGenerated || result.InterpretReport != artifact || tx.calls != 0 || runs.creates != 0 {
		t.Fatalf("generated result=%#v tx=%d runcreates=%d", result, tx.calls, runs.creates)
	}
}

func starterKey() domaingeneration.Key {
	return domaingeneration.Key{OutcomeID: meta.FromUint64(9), ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersion("v1")}
}

func newStarterFixture(t *testing.T, now time.Time) (Starter, *memoryGenerationRepo, *memoryRunRepo, *starterTx) {
	t.Helper()
	generations := newMemoryGenerationRepo()
	runs := newMemoryRunRepo()
	tx := &starterTx{}
	service, err := NewStarter(tx, generations, runs, &memoryArtifactRepo{items: map[meta.ID]*domainreport.InterpretReport{}}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	concrete := service.(*starter)
	concrete.now = func() time.Time { return now }
	next := uint64(100)
	concrete.newID = func() meta.ID { next++; return meta.FromUint64(next) }
	return concrete, generations, runs, tx
}

func seedGenerating(t *testing.T, startedAt time.Time, lease time.Duration) (*domaingeneration.ReportGeneration, *interpretationrun.InterpretationRun) {
	t.Helper()
	generationRecord, err := domaingeneration.New(meta.FromUint64(1), starterKey(), startedAt)
	if err != nil {
		t.Fatal(err)
	}
	runRecord, err := interpretationrun.NewPending(meta.FromUint64(2), generationRecord.ID(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.StartWithLease(startedAt, "existing", startedAt.Add(lease)); err != nil {
		t.Fatal(err)
	}
	if err := generationRecord.Begin(runRecord.ID(), startedAt); err != nil {
		t.Fatal(err)
	}
	return generationRecord, runRecord
}

func testArtifact(t *testing.T, generationRecord *domaingeneration.ReportGeneration, runRecord *interpretationrun.InterpretationRun, id meta.ID, at time.Time) *domainreport.InterpretReport {
	t.Helper()
	artifact, err := domainreport.NewInterpretReport(domainreport.InterpretReportInput{
		ID:                  id,
		GenerationID:        generationRecord.ID(),
		OutcomeID:           generationRecord.Key().OutcomeID,
		InterpretationRunID: runRecord.ID(),
		Association:         domainreport.Association{AssessmentID: meta.FromUint64(7), TesteeID: 8},
		ReportType:          generationRecord.Key().ReportType,
		TemplateVersion:     generationRecord.Key().TemplateVersion,
		GeneratedAt:         at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}
