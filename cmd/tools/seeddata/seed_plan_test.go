package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewPlanQuestionnaireVersionMismatchError(t *testing.T) {
	err := newPlanQuestionnaireVersionMismatchError("SAS-TEST", "QNR-001", "1.0.1", "6.0.1")
	if err == nil {
		t.Fatal("expected mismatch error")
	}

	msg := err.Error()
	for _, expected := range []string{
		"scale_code=SAS-TEST",
		"questionnaire_code=QNR-001",
		"scale_questionnaire_version=1.0.1",
		"loaded_questionnaire_version=6.0.1",
		"scale:sas-test",
		"<cache.namespace>:scale:sas-test",
	} {
		if !strings.Contains(msg, expected) {
			t.Fatalf("expected error message to contain %q, got %q", expected, msg)
		}
	}
}

func TestNewExplicitPlanZeroCreatedAtError(t *testing.T) {
	err := newExplicitPlanZeroCreatedAtError("614210295354634798")
	if err == nil {
		t.Fatal("expected explicit created_at error")
	}

	msg := err.Error()
	for _, expected := range []string{
		"explicit plan backfill requires non-zero created_at",
		"testee_id=614210295354634798",
		"--plan-testee-ids",
		"/api/v1/testees/614210295354634798",
		"testee:info:614210295354634798",
		"<cache.namespace>:testee:info:614210295354634798",
	} {
		if !strings.Contains(msg, expected) {
			t.Fatalf("expected error message to contain %q, got %q", expected, msg)
		}
	}
}

func TestPlanStartDateFromAuditTimes(t *testing.T) {
	now := time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 4, 5, 9, 0, 0, 0, time.UTC)

	date, source, err := planStartDateFromAuditTimes(createdAt, updatedAt, now)
	if err != nil {
		t.Fatalf("unexpected error for created_at: %v", err)
	}
	if source != "created_at" || date != "2026-04-01" {
		t.Fatalf("unexpected created_at fallback result: date=%s source=%s", date, source)
	}

	date, source, err = planStartDateFromAuditTimes(time.Time{}, updatedAt, now)
	if err != nil {
		t.Fatalf("unexpected error for updated_at fallback: %v", err)
	}
	if source != "updated_at" || date != "2026-04-05" {
		t.Fatalf("unexpected updated_at fallback result: date=%s source=%s", date, source)
	}

	date, source, err = planStartDateFromAuditTimes(time.Time{}, time.Time{}, now)
	if err != nil {
		t.Fatalf("unexpected error for now fallback: %v", err)
	}
	if source != "now" || date != "2026-04-08" {
		t.Fatalf("unexpected now fallback result: date=%s source=%s", date, source)
	}
}

func TestNormalizePlanWorkers(t *testing.T) {
	tests := []struct {
		name      string
		workers   int
		testeeCnt int
		expected  int
	}{
		{name: "default to one", workers: 0, testeeCnt: 10, expected: 1},
		{name: "cap by testee count", workers: 8, testeeCnt: 3, expected: 3},
		{name: "keep explicit worker count", workers: 4, testeeCnt: 10, expected: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePlanWorkers(tt.workers, tt.testeeCnt); got != tt.expected {
				t.Fatalf("normalizePlanWorkers(%d, %d)=%d, want=%d", tt.workers, tt.testeeCnt, got, tt.expected)
			}
		})
	}
}

func TestNormalizePlanTaskExecutionConcurrency(t *testing.T) {
	tests := []struct {
		name                string
		workers             int
		submitWorkers       int
		waitWorkers         int
		maxInFlight         int
		expectedSubmit      int
		expectedWait        int
		expectedMaxInFlight int
	}{
		{
			name:                "defaults follow plan workers",
			workers:             4,
			expectedSubmit:      4,
			expectedWait:        4,
			expectedMaxInFlight: 32,
		},
		{
			name:                "explicit values respected",
			workers:             2,
			submitWorkers:       8,
			waitWorkers:         3,
			maxInFlight:         50,
			expectedSubmit:      8,
			expectedWait:        3,
			expectedMaxInFlight: 50,
		},
		{
			name:                "max inflight raised to worker counts",
			workers:             2,
			submitWorkers:       6,
			waitWorkers:         5,
			maxInFlight:         4,
			expectedSubmit:      6,
			expectedWait:        5,
			expectedMaxInFlight: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSubmit, gotWait, gotMaxInFlight := normalizePlanTaskExecutionConcurrency(tt.workers, tt.submitWorkers, tt.waitWorkers, tt.maxInFlight)
			if gotSubmit != tt.expectedSubmit || gotWait != tt.expectedWait || gotMaxInFlight != tt.expectedMaxInFlight {
				t.Fatalf(
					"normalizePlanTaskExecutionConcurrency(%d, %d, %d, %d)=(%d,%d,%d), want=(%d,%d,%d)",
					tt.workers,
					tt.submitWorkers,
					tt.waitWorkers,
					tt.maxInFlight,
					gotSubmit,
					gotWait,
					gotMaxInFlight,
					tt.expectedSubmit,
					tt.expectedWait,
					tt.expectedMaxInFlight,
				)
			}
		})
	}
}

func TestNormalizePlanExpireRate(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{name: "negative to zero", input: -0.2, expected: 0},
		{name: "keep middle value", input: 0.35, expected: 0.35},
		{name: "cap at one", input: 1.5, expected: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePlanExpireRate(tt.input); got != tt.expected {
				t.Fatalf("normalizePlanExpireRate(%v)=%v, want=%v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestShouldExpirePlanTask(t *testing.T) {
	task := TaskResponse{ID: "614186929759466030"}

	if shouldExpirePlanTask(task, 0) {
		t.Fatal("expected zero expire rate to never expire")
	}
	if !shouldExpirePlanTask(task, 1) {
		t.Fatal("expected full expire rate to always expire")
	}
	if got1, got2 := shouldExpirePlanTask(task, 0.2), shouldExpirePlanTask(task, 0.2); got1 != got2 {
		t.Fatalf("expected deterministic expire decision, got %v and %v", got1, got2)
	}
}

func TestApplyTesteeLimitToIDs(t *testing.T) {
	ids := []string{"1001", "1002", "1003"}

	if got := applyTesteeLimitToIDs(ids, 0); len(got) != 3 {
		t.Fatalf("expected no limit to keep all ids, got %v", got)
	}
	if got := applyTesteeLimitToIDs(ids, 2); len(got) != 2 || got[0] != "1001" || got[1] != "1002" {
		t.Fatalf("expected limit to keep first two ids, got %v", got)
	}
	if got := applyTesteeLimitToIDs(ids, 5); len(got) != 3 {
		t.Fatalf("expected large limit to keep all ids, got %v", got)
	}
}

func TestSummarizePlanTaskStatuses(t *testing.T) {
	stats := summarizePlanTaskStatuses([]TaskResponse{
		{Status: "pending"},
		{Status: "opened"},
		{Status: "completed"},
		{Status: "expired"},
		{Status: "canceled"},
		{Status: "weird"},
	})

	if stats.Total != 6 {
		t.Fatalf("expected total=6, got %d", stats.Total)
	}
	if stats.Pending != 1 || stats.Opened != 1 || stats.Completed != 1 || stats.Expired != 1 || stats.Canceled != 1 || stats.Unknown != 1 {
		t.Fatalf("unexpected task stats: %+v", stats)
	}
}

func TestMergePlanTaskStatusStats(t *testing.T) {
	dst := &planTaskStatusStats{Total: 2, Pending: 1, Opened: 1}
	src := &planTaskStatusStats{Total: 3, Completed: 2, Expired: 1}
	mergePlanTaskStatusStats(dst, src)

	if dst.Total != 5 || dst.Pending != 1 || dst.Opened != 1 || dst.Completed != 2 || dst.Expired != 1 {
		t.Fatalf("unexpected merged stats: %+v", dst)
	}
}

func TestResolvePlanMode(t *testing.T) {
	tests := []struct {
		name    string
		cli     string
		cfg     string
		want    string
		wantErr bool
	}{
		{name: "default local", cli: "", cfg: "", want: planModeLocal},
		{name: "config remote", cli: "", cfg: planModeRemote, want: planModeRemote},
		{name: "cli overrides config", cli: planModeLocal, cfg: planModeRemote, want: planModeLocal},
		{name: "invalid mode", cli: "bad", cfg: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePlanMode(tt.cli, tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolvePlanMode(%q, %q)=%q, want=%q", tt.cli, tt.cfg, got, tt.want)
			}
		})
	}
}

func TestSeedPlanPacerNextDelay(t *testing.T) {
	startedAt := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	pacer := newSeedPlanPacer(startedAt, 3*time.Minute, 15*time.Second, nil, false)
	if pacer == nil {
		t.Fatal("expected pacer to be initialized")
	}

	delay, fresh := pacer.nextDelay(startedAt.Add(2 * time.Minute))
	if delay != 0 || fresh {
		t.Fatalf("expected no pause before interval, got delay=%s fresh=%v", delay, fresh)
	}

	delay, fresh = pacer.nextDelay(startedAt.Add(3 * time.Minute))
	if delay != 15*time.Second || !fresh {
		t.Fatalf("expected fresh pause at interval boundary, got delay=%s fresh=%v", delay, fresh)
	}

	delay, fresh = pacer.nextDelay(startedAt.Add(3*time.Minute + 5*time.Second))
	if delay != 10*time.Second || fresh {
		t.Fatalf("expected remaining shared pause window, got delay=%s fresh=%v", delay, fresh)
	}

	delay, fresh = pacer.nextDelay(startedAt.Add(3*time.Minute + 16*time.Second))
	if delay != 0 || fresh {
		t.Fatalf("expected no pause immediately after pause window, got delay=%s fresh=%v", delay, fresh)
	}

	delay, fresh = pacer.nextDelay(startedAt.Add(6 * time.Minute))
	if delay != 15*time.Second || !fresh {
		t.Fatalf("expected next pause at following interval, got delay=%s fresh=%v", delay, fresh)
	}
}

func TestPrioritizePlanEnrollmentTesteesPrefersNeverJoinedThenLeastJoined(t *testing.T) {
	testees := []*TesteeResponse{
		{ID: "1001", CreatedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "1002", CreatedAt: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
		{ID: "1003", CreatedAt: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)},
		{ID: "1004", CreatedAt: time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC)},
	}
	gateway := &planTaskCountProviderStub{
		counts: map[string]int{
			"1001": 0,
			"1002": 3,
			"1003": 1,
			"1004": 0,
		},
	}

	prioritized, err := prioritizePlanEnrollmentTestees(context.Background(), gateway, noopSeedLogger{}, "614333603412718126", testees, 2, false)
	if err != nil {
		t.Fatalf("unexpected prioritize error: %v", err)
	}

	gotIDs := make([]string, 0, len(prioritized))
	for _, testee := range prioritized {
		gotIDs = append(gotIDs, testee.ID)
	}
	wantIDs := []string{"1001", "1004", "1003", "1002"}
	if strings.Join(gotIDs, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("unexpected prioritized ids: got=%v want=%v", gotIDs, wantIDs)
	}
}

func TestSelectPlanEnrollmentTesteesKeepsHighestPrioritySlice(t *testing.T) {
	prioritized := []*TesteeResponse{
		{ID: "1001"},
		{ID: "1002"},
		{ID: "1003"},
		{ID: "1004"},
		{ID: "1005"},
		{ID: "1006"},
		{ID: "1007"},
		{ID: "1008"},
		{ID: "1009"},
		{ID: "1010"},
	}

	selected := selectPlanEnrollmentTestees(prioritized)
	if len(selected) != 2 {
		t.Fatalf("expected top 2 selected testees from 10 candidates, got %d", len(selected))
	}
	if selected[0].ID != "1001" || selected[1].ID != "1002" {
		t.Fatalf("expected highest priority testees to be kept, got %v and %v", selected[0].ID, selected[1].ID)
	}
}

type planTaskCountProviderStub struct {
	counts map[string]int
}

func (s *planTaskCountProviderStub) GetPlan(ctx context.Context, planID string) (*PlanResponse, error) {
	return nil, nil
}

func (s *planTaskCountProviderStub) GetScale(ctx context.Context, code string) (*ScaleResponse, error) {
	return nil, nil
}

func (s *planTaskCountProviderStub) GetQuestionnaireDetail(ctx context.Context, code string) (*QuestionnaireDetailResponse, error) {
	return nil, nil
}

func (s *planTaskCountProviderStub) ListTesteesByOrg(ctx context.Context, orgID int64, page, pageSize int) (*ApiserverTesteeListResponse, error) {
	return nil, nil
}

func (s *planTaskCountProviderStub) GetTesteeByID(ctx context.Context, testeeID string) (*ApiserverTesteeResponse, error) {
	return nil, nil
}

func (s *planTaskCountProviderStub) EnrollTestee(ctx context.Context, req EnrollTesteeRequest) (*EnrollmentResponse, error) {
	return nil, nil
}

func (s *planTaskCountProviderStub) SchedulePendingTasks(ctx context.Context, req SchedulePendingTasksRequest) (*TaskListResponse, error) {
	return nil, nil
}

func (s *planTaskCountProviderStub) ListTasksByTesteeAndPlan(ctx context.Context, testeeID, planID string) (*TaskListResponse, error) {
	return &TaskListResponse{}, nil
}

func (s *planTaskCountProviderStub) GetTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	return nil, nil
}

func (s *planTaskCountProviderStub) ExpireTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	return nil, nil
}

func (s *planTaskCountProviderStub) GetPlanTaskCountsByTesteeIDs(ctx context.Context, planID string, testeeIDs []string) (map[string]int, error) {
	return s.counts, nil
}

type noopSeedLogger struct{}

func (noopSeedLogger) Warnw(string, ...interface{}) {}
func (noopSeedLogger) Infow(string, ...interface{}) {}
