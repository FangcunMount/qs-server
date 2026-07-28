package plan

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"testing"
	"time"

	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	stageport "github.com/FangcunMount/qs-server/internal/apiserver/port/historicalseedstage"
	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
)

type historicalTaskRepoStub struct {
	domainPlan.AssessmentTaskRepository
	task      *domainPlan.AssessmentTask
	saveCalls int
	lockCalls int
}

func (r *historicalTaskRepoStub) FindByID(context.Context, domainPlan.AssessmentTaskID) (*domainPlan.AssessmentTask, error) {
	return r.task, nil
}

func (r *historicalTaskRepoStub) FindByIDForUpdate(context.Context, domainPlan.AssessmentTaskID) (*domainPlan.AssessmentTask, error) {
	r.lockCalls++
	return r.task, nil
}

func (r *historicalTaskRepoStub) Save(context.Context, *domainPlan.AssessmentTask) error {
	r.saveCalls++
	return nil
}

type historicalEntryGeneratorStub struct {
	calls    int
	expireAt time.Time
}

func (g *historicalEntryGeneratorStub) GenerateEntry(context.Context, *domainPlan.AssessmentTask) (string, string, time.Time, error) {
	g.calls++
	return "token", "https://example.com/task", g.expireAt, nil
}

type historicalTaskStageStoreStub struct {
	records       map[string]stageport.Record
	beginAttempts int
	failed        int
	nextAttemptID uint64
}

func (s *historicalTaskStageStoreStub) Begin(_ context.Context, attempt stageport.Attempt) (stageport.AttemptHandle, error) {
	s.beginAttempts++
	s.nextAttemptID++
	return stageport.AttemptHandle{ID: s.nextAttemptID, Stage: attempt.Stage, ContextHash: "test"}, nil
}

func (s *historicalTaskStageStoreStub) Fail(_ context.Context, _ stageport.AttemptHandle, _ stageport.Failure) error {
	s.failed++
	return nil
}

func (s *historicalTaskStageStoreStub) CompleteAttempt(ctx context.Context, _ stageport.AttemptHandle, completion stageport.Completion) (*stageport.Record, error) {
	return s.Complete(ctx, completion)
}

func (s *historicalTaskStageStoreStub) Complete(_ context.Context, completion stageport.Completion) (*stageport.Record, error) {
	payload, err := json.Marshal(completion.Payload)
	if err != nil {
		return nil, err
	}
	record := stageport.Record{Stage: completion.Stage, Status: "completed", BusinessAt: completion.BusinessAt, ResourceType: completion.ResourceType, ResourceID: completion.ResourceID, PayloadJSON: payload}
	s.records[completion.Stage] = record
	return &record, nil
}

func (s *historicalTaskStageStoreStub) FindCurrent(_ context.Context, stage string) (*stageport.Record, error) {
	record, ok := s.records[stage]
	if !ok {
		return nil, nil
	}
	copy := record
	return &copy, nil
}

func TestHistoricalTaskTransitionsReplayPersistedStagesWithoutDuplicateMutation(t *testing.T) {
	openedAt := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
	completedAt := openedAt.Add(30 * time.Minute)
	task := domainPlan.NewAssessmentTask(domainPlan.NewAssessmentPlanID(), 1, 9, domainTestee.NewID(7), "MODEL", openedAt)
	repo := &historicalTaskRepoStub{task: task}
	generator := &historicalEntryGeneratorStub{expireAt: openedAt.Add(time.Hour)}
	stages := &historicalTaskStageStoreStub{records: make(map[string]stageport.Record)}
	service := WithTaskHistoricalStageRecorder(NewTaskManagementService(repo, generator, nil), stages)
	ctx := historicalseed.WithContext(context.Background(), historicalseed.Context{
		BatchID: "batch", ScenarioID: "2025-01-01/1/submit_answer/task", OrgID: 9, Version: historicalseed.Version1,
		Timeline: historicalseed.Timeline{TaskOpenedAt: &openedAt, TaskCompletedAt: &completedAt},
	})

	if _, err := service.OpenTask(ctx, 9, task.GetID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenTask(ctx, 9, task.GetID().String()); err != nil {
		t.Fatalf("replay open: %v", err)
	}
	if generator.calls != 1 || repo.saveCalls != 1 {
		t.Fatalf("open calls generator=%d save=%d", generator.calls, repo.saveCalls)
	}

	if _, err := service.CompleteTask(ctx, 9, task.GetID().String(), "91"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteTask(ctx, 9, task.GetID().String(), "91"); err != nil {
		t.Fatalf("replay complete: %v", err)
	}
	if repo.saveCalls != 2 {
		t.Fatalf("completion replay saved task again: save=%d", repo.saveCalls)
	}
	if repo.lockCalls != 4 {
		t.Fatalf("historical transitions did not lock every read: %d", repo.lockCalls)
	}
	if stages.beginAttempts != 4 || stages.failed != 0 {
		t.Fatalf("attempt lifecycle begins=%d failed=%d", stages.beginAttempts, stages.failed)
	}
}

func TestHistoricalTaskReplayRejectsChangedTimeline(t *testing.T) {
	openedAt := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
	task := domainPlan.NewAssessmentTask(domainPlan.NewAssessmentPlanID(), 1, 9, domainTestee.NewID(7), "MODEL", openedAt)
	repo := &historicalTaskRepoStub{task: task}
	generator := &historicalEntryGeneratorStub{expireAt: openedAt.Add(time.Hour)}
	stages := &historicalTaskStageStoreStub{records: make(map[string]stageport.Record)}
	service := WithTaskHistoricalStageRecorder(NewTaskManagementService(repo, generator, nil), stages)
	base := historicalseed.Context{BatchID: "batch", ScenarioID: "scenario", OrgID: 9, Version: historicalseed.Version1, Timeline: historicalseed.Timeline{TaskOpenedAt: &openedAt}}
	if _, err := service.OpenTask(historicalseed.WithContext(context.Background(), base), 9, task.GetID().String()); err != nil {
		t.Fatal(err)
	}
	driftedAt := openedAt.Add(time.Minute)
	base.Timeline.TaskOpenedAt = &driftedAt
	_, err := service.OpenTask(historicalseed.WithContext(context.Background(), base), 9, task.GetID().String())
	if !stderrors.Is(err, stageport.ErrPayloadConflict) {
		t.Fatalf("err=%v, want payload conflict", err)
	}
	if stages.beginAttempts != 2 || stages.failed != 1 {
		t.Fatalf("attempt lifecycle begins=%d failed=%d", stages.beginAttempts, stages.failed)
	}
}
