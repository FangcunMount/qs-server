package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
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

func TestPlanProcessOptionsWithScopeForcesOneShotAndCopiesIDs(t *testing.T) {
	base := planProcessOptions{
		PlanID:         "614333603412718126",
		ScopeTesteeIDs: []string{"1001"},
		Continuous:     true,
	}

	got := base.withScope([]string{"1002", "1003"}, false)
	if got.Continuous {
		t.Fatal("expected withScope to apply the requested continuous flag")
	}
	if strings.Join(got.ScopeTesteeIDs, ",") != "1002,1003" {
		t.Fatalf("unexpected scope ids: %v", got.ScopeTesteeIDs)
	}
	got.ScopeTesteeIDs[0] = "changed"
	if strings.Join(base.ScopeTesteeIDs, ",") != "1001" {
		t.Fatalf("expected original scope ids to remain unchanged, got %v", base.ScopeTesteeIDs)
	}
}

func TestNewPlanTesteeSelector(t *testing.T) {
	tests := []struct {
		name        string
		opts        planCreateOptions
		explicitIDs []string
		wantType    string
	}{
		{
			name:        "explicit selector",
			opts:        planCreateOptions{PlanProcessExistingOnly: false},
			explicitIDs: []string{"1001"},
			wantType:    "main.explicitPlanTesteeSelector",
		},
		{
			name:     "recovery selector",
			opts:     planCreateOptions{PlanProcessExistingOnly: true},
			wantType: "main.recoveryPlanTesteeSelector",
		},
		{
			name:     "sampled selector",
			opts:     planCreateOptions{},
			wantType: "main.sampledPriorityPlanTesteeSelector",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := newPlanTesteeSelector(tt.opts, tt.explicitIDs)
			if selector == nil {
				t.Fatal("expected selector")
			}
			if got := fmt.Sprintf("%T", selector); got != tt.wantType {
				t.Fatalf("unexpected selector type: got=%s want=%s", got, tt.wantType)
			}
		})
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

func TestIsRecoveryPlanCompleted(t *testing.T) {
	tests := []struct {
		name  string
		tasks []TaskResponse
		want  bool
	}{
		{
			name: "last seq completed",
			tasks: []TaskResponse{
				{Seq: 1, Status: "completed"},
				{Seq: 2, Status: "completed"},
			},
			want: true,
		},
		{
			name: "last seq expired",
			tasks: []TaskResponse{
				{Seq: 1, Status: "completed"},
				{Seq: 2, Status: "expired"},
			},
			want: true,
		},
		{
			name: "last seq opened",
			tasks: []TaskResponse{
				{Seq: 1, Status: "completed"},
				{Seq: 2, Status: "opened"},
			},
			want: false,
		},
		{
			name: "mixed terminal statuses at last seq",
			tasks: []TaskResponse{
				{Seq: 1, Status: "completed"},
				{Seq: 2, Status: "completed"},
				{Seq: 2, Status: "expired"},
			},
			want: true,
		},
		{
			name: "last seq contains opened duplicate",
			tasks: []TaskResponse{
				{Seq: 1, Status: "completed"},
				{Seq: 2, Status: "completed"},
				{Seq: 2, Status: "opened"},
			},
			want: false,
		},
		{
			name: "last seq canceled is not treated as completed",
			tasks: []TaskResponse{
				{Seq: 1, Status: "completed"},
				{Seq: 2, Status: "canceled"},
			},
			want: false,
		},
		{
			name: "empty task list",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRecoveryPlanCompleted(tt.tasks); got != tt.want {
				t.Fatalf("isRecoveryPlanCompleted(%+v)=%v, want=%v", tt.tasks, got, tt.want)
			}
		})
	}
}

func TestFilterRecoveryPlanTestees(t *testing.T) {
	testees := []*TesteeResponse{
		{ID: "1001"},
		{ID: "1002"},
		{ID: "1003"},
		{ID: "1004"},
		{ID: "1005"},
	}
	gateway := &planTaskCountProviderStub{
		taskLists: map[string][]TaskResponse{
			"1001": {
				{ID: "2001", Seq: 1, Status: "completed"},
				{ID: "2002", Seq: 2, Status: "completed"},
			},
			"1002": {
				{ID: "2003", Seq: 1, Status: "completed"},
				{ID: "2004", Seq: 2, Status: "expired"},
			},
			"1003": {
				{ID: "2005", Seq: 1, Status: "completed"},
				{ID: "2006", Seq: 2, Status: "opened"},
			},
			"1004": {},
		},
		taskErrs: map[string]error{
			"1005": errors.New("temporary timeout"),
		},
	}

	filtered, stats, err := filterRecoveryPlanTestees(context.Background(), gateway, noopSeedLogger{}, "614333603412718126", testees, 2, false)
	if err != nil {
		t.Fatalf("unexpected filter error: %v", err)
	}

	gotIDs := make([]string, 0, len(filtered))
	for _, testee := range filtered {
		gotIDs = append(gotIDs, testee.ID)
	}
	sort.Strings(gotIDs)
	wantIDs := []string{"1003", "1005"}
	if strings.Join(gotIDs, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("unexpected filtered ids: got=%v want=%v", gotIDs, wantIDs)
	}

	if stats.FilteredCompletedPlanTestees != 2 || stats.FilteredNoTaskTestees != 1 || stats.RetainedUndeterminedTestees != 1 {
		t.Fatalf("unexpected recovery filter stats: %+v", stats)
	}
	if stats.ExistingTaskStats == nil {
		t.Fatal("expected existing task stats")
	}
	if stats.ExistingTaskStats.Total != 6 || stats.ExistingTaskStats.Completed != 4 || stats.ExistingTaskStats.Expired != 1 || stats.ExistingTaskStats.Opened != 1 {
		t.Fatalf("unexpected existing task stats: %+v", stats.ExistingTaskStats)
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

func TestPrioritizePlanEnrollmentTesteesPrefersNoTasksThenNoCurrentPlanThenFewerTasks(t *testing.T) {
	testees := []*TesteeResponse{
		{ID: "1001", CreatedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "1002", CreatedAt: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
		{ID: "1003", CreatedAt: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)},
		{ID: "1004", CreatedAt: time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC)},
		{ID: "1005", CreatedAt: time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)},
	}
	gateway := &planTaskCountProviderStub{
		priorityStats: map[string]planTesteeTaskPriority{
			"1001": {TotalTaskCount: 0, CurrentPlanTaskCount: 0},
			"1002": {TotalTaskCount: 5, CurrentPlanTaskCount: 0},
			"1003": {TotalTaskCount: 2, CurrentPlanTaskCount: 1},
			"1004": {TotalTaskCount: 3, CurrentPlanTaskCount: 0},
			"1005": {TotalTaskCount: 1, CurrentPlanTaskCount: 1},
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
	wantIDs := []string{"1001", "1004", "1002", "1005", "1003"}
	if strings.Join(gotIDs, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("unexpected prioritized ids: got=%v want=%v", gotIDs, wantIDs)
	}
}

func TestPrioritizePlanEnrollmentTesteesFallsBackToListTasksByTestee(t *testing.T) {
	testees := []*TesteeResponse{
		{ID: "1001", CreatedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "1002", CreatedAt: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
		{ID: "1003", CreatedAt: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)},
	}
	gateway := &pagedPlanSeedGatewayStub{
		taskLists: map[string][]TaskResponse{
			"1001": {},
			"1002": {
				{ID: "2001", PlanID: "other-plan", TesteeID: "1002", Seq: 1, Status: "completed"},
				{ID: "2002", PlanID: "other-plan", TesteeID: "1002", Seq: 2, Status: "opened"},
			},
			"1003": {
				{ID: "2003", PlanID: "614333603412718126", TesteeID: "1003", Seq: 1, Status: "completed"},
			},
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
	wantIDs := []string{"1001", "1002", "1003"}
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

func TestStreamSamplePlanEnrollmentTesteesUsesPagedIteration(t *testing.T) {
	base := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	client := &pagedPlanSeedGatewayStub{
		pages: map[int][]*ApiserverTesteeResponse{
			1: {
				{ID: "1001", CreatedAt: base.Add(1 * time.Minute)},
				{ID: "1002", CreatedAt: base.Add(2 * time.Minute)},
				{ID: "1003", CreatedAt: base.Add(3 * time.Minute)},
			},
			2: {
				{ID: "1004", CreatedAt: base.Add(4 * time.Minute)},
				{ID: "1005", CreatedAt: base.Add(5 * time.Minute)},
				{ID: "1006", CreatedAt: base.Add(6 * time.Minute)},
			},
			3: {
				{ID: "1007", CreatedAt: base.Add(7 * time.Minute)},
			},
		},
		totalPages: 3,
	}

	selected, loadedCount, err := streamSamplePlanEnrollmentTestees(context.Background(), client, 1, 3, 0, 0, "614333603412718126")
	if err != nil {
		t.Fatalf("unexpected stream sample error: %v", err)
	}
	if loadedCount != 7 {
		t.Fatalf("expected loadedCount=7, got %d", loadedCount)
	}
	if len(selected) == 0 {
		t.Fatal("expected at least one selected testee")
	}
	for i := 1; i < len(selected); i++ {
		if selected[i-1].CreatedAt.After(selected[i].CreatedAt) {
			t.Fatalf("expected selected testees sorted by created_at, got %v before %v", selected[i-1].ID, selected[i].ID)
		}
	}
	if got := strings.Join(client.pageCalls, ","); got != "1,2,3" {
		t.Fatalf("expected paged calls 1,2,3, got %s", got)
	}
}

func TestStreamFilterRecoveryPlanTesteesUsesPagedFiltering(t *testing.T) {
	base := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	gateway := &pagedPlanSeedGatewayStub{
		pages: map[int][]*ApiserverTesteeResponse{
			1: {
				{ID: "1001", CreatedAt: base.Add(1 * time.Minute)},
				{ID: "1002", CreatedAt: base.Add(2 * time.Minute)},
			},
			2: {
				{ID: "1003", CreatedAt: base.Add(3 * time.Minute)},
				{ID: "1004", CreatedAt: base.Add(4 * time.Minute)},
			},
			3: {
				{ID: "1005", CreatedAt: base.Add(5 * time.Minute)},
				{ID: "1006", CreatedAt: base.Add(6 * time.Minute)},
			},
		},
		totalPages: 3,
		taskLists: map[string][]TaskResponse{
			"1001": {{ID: "t1", Seq: 1, Status: "completed"}},
			"1002": {{ID: "t2", Seq: 1, Status: "opened"}},
			"1003": {},
			"1004": {{ID: "t4", Seq: 1, Status: "expired"}},
			"1005": {{ID: "t5", Seq: 1, Status: "pending"}},
			"1006": {{ID: "t6", Seq: 1, Status: "completed"}},
		},
	}

	retained, stats, loadedCount, err := streamFilterRecoveryPlanTestees(
		context.Background(),
		gateway,
		noopSeedLogger{},
		1,
		2,
		0,
		0,
		"614333603412718126",
		1,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected stream filter error: %v", err)
	}
	if loadedCount != 6 {
		t.Fatalf("expected loadedCount=6, got %d", loadedCount)
	}
	if stats == nil || stats.ExistingTaskStats == nil {
		t.Fatal("expected aggregate filter stats")
	}
	gotIDs := make([]string, 0, len(retained))
	for _, testee := range retained {
		gotIDs = append(gotIDs, testee.ID)
	}
	if got := strings.Join(gotIDs, ","); got != "1002,1005" {
		t.Fatalf("unexpected retained ids: got=%s want=1002,1005", got)
	}
	if got := strings.Join(gateway.pageCalls, ","); got != "1,2,3" {
		t.Fatalf("expected paged calls 1,2,3, got %s", got)
	}
	if stats.FilteredCompletedPlanTestees != 3 || stats.FilteredNoTaskTestees != 1 || stats.RetainedUndeterminedTestees != 0 {
		t.Fatalf("unexpected filter stats: %+v", stats)
	}
}

type planTaskCountProviderStub struct {
	priorityStats map[string]planTesteeTaskPriority
	taskLists     map[string][]TaskResponse
	taskErrs      map[string]error
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

func (s *planTaskCountProviderStub) ListTasksByPlan(ctx context.Context, planID string) (*TaskListResponse, error) {
	return buildStubTaskListResponse(s.taskLists, ListTasksRequest{PlanID: planID}), nil
}

func (s *planTaskCountProviderStub) ListTasks(ctx context.Context, req ListTasksRequest) (*TaskListResponse, error) {
	if err, ok := s.taskErrs[strings.TrimSpace(req.TesteeID)]; ok {
		return nil, err
	}
	return buildStubTaskListResponse(s.taskLists, req), nil
}

func (s *planTaskCountProviderStub) ListTasksByTestee(ctx context.Context, testeeID string) (*TaskListResponse, error) {
	if err, ok := s.taskErrs[testeeID]; ok {
		return nil, err
	}
	return buildStubTaskListResponse(s.taskLists, ListTasksRequest{TesteeID: testeeID}), nil
}

func (s *planTaskCountProviderStub) ListTasksByTesteeAndPlan(ctx context.Context, testeeID, planID string) (*TaskListResponse, error) {
	if err, ok := s.taskErrs[testeeID]; ok {
		return nil, err
	}
	return buildStubTaskListResponse(s.taskLists, ListTasksRequest{PlanID: planID, TesteeID: testeeID}), nil
}

func (s *planTaskCountProviderStub) GetTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	return nil, nil
}

func (s *planTaskCountProviderStub) ExpireTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	return nil, nil
}

func (s *planTaskCountProviderStub) GetPlanTaskPriorityByTesteeIDs(ctx context.Context, planID string, testeeIDs []string) (map[string]planTesteeTaskPriority, error) {
	return s.priorityStats, nil
}

type noopSeedLogger struct{}

func (noopSeedLogger) Warnw(string, ...interface{}) {}
func (noopSeedLogger) Infow(string, ...interface{}) {}

type pagedPlanSeedGatewayStub struct {
	pages                      map[int][]*ApiserverTesteeResponse
	totalPages                 int
	pageCalls                  []string
	taskLists                  map[string][]TaskResponse
	taskErrs                   map[string]error
	listTaskCalls              []ListTasksRequest
	listTasksByPlanCallCount   int
	listTasksByTesteePlanCalls []string
}

func (s *pagedPlanSeedGatewayStub) GetPlan(ctx context.Context, planID string) (*PlanResponse, error) {
	return nil, nil
}

func (s *pagedPlanSeedGatewayStub) GetScale(ctx context.Context, code string) (*ScaleResponse, error) {
	return nil, nil
}

func (s *pagedPlanSeedGatewayStub) GetQuestionnaireDetail(ctx context.Context, code string) (*QuestionnaireDetailResponse, error) {
	return nil, nil
}

func (s *pagedPlanSeedGatewayStub) ListTesteesByOrg(ctx context.Context, orgID int64, page, pageSize int) (*ApiserverTesteeListResponse, error) {
	s.pageCalls = append(s.pageCalls, fmt.Sprintf("%d", page))
	items := append([]*ApiserverTesteeResponse(nil), s.pages[page]...)
	return &ApiserverTesteeListResponse{
		Items:      items,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: s.totalPages,
		Total:      0,
	}, nil
}

func (s *pagedPlanSeedGatewayStub) GetTesteeByID(ctx context.Context, testeeID string) (*ApiserverTesteeResponse, error) {
	return nil, nil
}

func (s *pagedPlanSeedGatewayStub) EnrollTestee(ctx context.Context, req EnrollTesteeRequest) (*EnrollmentResponse, error) {
	return nil, nil
}

func (s *pagedPlanSeedGatewayStub) SchedulePendingTasks(ctx context.Context, req SchedulePendingTasksRequest) (*TaskListResponse, error) {
	return nil, nil
}

func (s *pagedPlanSeedGatewayStub) ListTasksByPlan(ctx context.Context, planID string) (*TaskListResponse, error) {
	s.listTasksByPlanCallCount++
	return buildStubTaskListResponse(s.taskLists, ListTasksRequest{PlanID: planID}), nil
}

func (s *pagedPlanSeedGatewayStub) ListTasks(ctx context.Context, req ListTasksRequest) (*TaskListResponse, error) {
	if err, ok := s.taskErrs[strings.TrimSpace(req.TesteeID)]; ok {
		return nil, err
	}
	s.listTaskCalls = append(s.listTaskCalls, req)
	return buildStubTaskListResponse(s.taskLists, req), nil
}

func (s *pagedPlanSeedGatewayStub) ListTasksByTestee(ctx context.Context, testeeID string) (*TaskListResponse, error) {
	if err, ok := s.taskErrs[testeeID]; ok {
		return nil, err
	}
	return buildStubTaskListResponse(s.taskLists, ListTasksRequest{TesteeID: testeeID}), nil
}

func (s *pagedPlanSeedGatewayStub) ListTasksByTesteeAndPlan(ctx context.Context, testeeID, planID string) (*TaskListResponse, error) {
	if err, ok := s.taskErrs[testeeID]; ok {
		return nil, err
	}
	s.listTasksByTesteePlanCalls = append(s.listTasksByTesteePlanCalls, fmt.Sprintf("%s:%s", testeeID, planID))
	return buildStubTaskListResponse(s.taskLists, ListTasksRequest{PlanID: planID, TesteeID: testeeID}), nil
}

func (s *pagedPlanSeedGatewayStub) GetTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	return nil, nil
}

func (s *pagedPlanSeedGatewayStub) ExpireTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	return nil, nil
}

func buildStubTaskListResponse(taskLists map[string][]TaskResponse, req ListTasksRequest) *TaskListResponse {
	allTasks := make([]TaskResponse, 0)
	for testeeID, items := range taskLists {
		for _, task := range items {
			if strings.TrimSpace(task.TesteeID) == "" {
				task.TesteeID = testeeID
			}
			allTasks = append(allTasks, task)
		}
	}

	planID := strings.TrimSpace(req.PlanID)
	testeeID := strings.TrimSpace(req.TesteeID)
	status := normalizeTaskStatus(req.Status)
	filtered := make([]TaskResponse, 0, len(allTasks))
	for _, task := range allTasks {
		if planID != "" {
			taskPlanID := strings.TrimSpace(task.PlanID)
			if taskPlanID != "" && taskPlanID != planID {
				continue
			}
		}
		if testeeID != "" {
			taskTesteeID := strings.TrimSpace(task.TesteeID)
			if taskTesteeID != "" && taskTesteeID != testeeID {
				continue
			}
		}
		if status != "" && normalizeTaskStatus(task.Status) != status {
			continue
		}
		filtered = append(filtered, task)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		left := strings.TrimSpace(filtered[i].TesteeID)
		right := strings.TrimSpace(filtered[j].TesteeID)
		if left != right {
			return left < right
		}
		if filtered[i].Seq != filtered[j].Seq {
			return filtered[i].Seq < filtered[j].Seq
		}
		return filtered[i].ID < filtered[j].ID
	})

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = len(filtered)
		if pageSize == 0 {
			pageSize = 1
		}
	}

	start := (page - 1) * pageSize
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}

	return &TaskListResponse{
		Tasks:      append([]TaskResponse(nil), filtered[start:end]...),
		TotalCount: int64(len(filtered)),
		Page:       page,
		PageSize:   pageSize,
	}
}

func TestCollectPlanTaskJobsByPlanUsesPlanTaskListing(t *testing.T) {
	gateway := &planTaskCountProviderStub{
		taskLists: map[string][]TaskResponse{
			"1001": {
				{ID: "2001", TesteeID: "1001", Seq: 1, Status: "opened"},
				{ID: "2002", TesteeID: "1001", Seq: 2, Status: "completed"},
			},
			"1002": {
				{ID: "2003", TesteeID: "1002", Seq: 1, Status: "expired"},
				{ID: "2004", TesteeID: "1002", Seq: 2, Status: "opened"},
			},
		},
	}
	deps := &dependencies{Logger: newSeeddataLogger(false)}
	var skipped atomic.Int64
	var failed atomic.Int64

	jobs, err := collectPlanTaskJobsByPlan(context.Background(), gateway, deps, "614333603412718126", false, &skipped, &failed)
	if err != nil {
		t.Fatalf("unexpected collect error: %v", err)
	}
	if failed.Load() != 0 {
		t.Fatalf("expected no failed task list loads, got %d", failed.Load())
	}
	if skipped.Load() != 2 {
		t.Fatalf("expected 2 skipped tasks, got %d", skipped.Load())
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 opened jobs, got %d", len(jobs))
	}
	if jobs[0].testeeID != "1001" || jobs[0].task.ID != "2001" {
		t.Fatalf("unexpected first job: %+v", jobs[0])
	}
	if jobs[1].testeeID != "1002" || jobs[1].task.ID != "2004" {
		t.Fatalf("unexpected second job: %+v", jobs[1])
	}
}

func TestCollectPlanTaskJobWindowByPlanUsesPagedOpenedTasks(t *testing.T) {
	planID := "614333603412718126"
	gateway := &pagedPlanSeedGatewayStub{
		taskLists: map[string][]TaskResponse{
			"1001": {
				{ID: "2001", PlanID: planID, TesteeID: "1001", Seq: 1, Status: "opened"},
				{ID: "2002", PlanID: planID, TesteeID: "1001", Seq: 2, Status: "completed"},
				{ID: "2003", PlanID: planID, TesteeID: "1001", Seq: 3, Status: "opened"},
			},
			"1002": {
				{ID: "2004", PlanID: planID, TesteeID: "1002", Seq: 1, Status: "opened"},
				{ID: "2005", PlanID: planID, TesteeID: "1002", Seq: 2, Status: "opened"},
			},
		},
	}
	deps := &dependencies{Logger: newSeeddataLogger(false)}
	var skipped atomic.Int64
	var failed atomic.Int64

	jobs, more, err := collectPlanTaskJobWindowByPlan(context.Background(), gateway, deps, planID, 2, false, &skipped, &failed)
	if err != nil {
		t.Fatalf("unexpected collect error: %v", err)
	}
	if failed.Load() != 0 {
		t.Fatalf("expected no failed task list loads, got %d", failed.Load())
	}
	if more != true {
		t.Fatal("expected more opened tasks after bounded discovery window")
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 opened jobs, got %d", len(jobs))
	}
	if gateway.listTasksByPlanCallCount != 0 {
		t.Fatalf("expected no full plan task listing calls, got %d", gateway.listTasksByPlanCallCount)
	}
	if len(gateway.listTaskCalls) != 1 {
		t.Fatalf("expected 1 paged list tasks call, got %d", len(gateway.listTaskCalls))
	}
	call := gateway.listTaskCalls[0]
	if call.PlanID != planID || call.Status != "opened" || call.Page != 1 || call.PageSize != 2 {
		t.Fatalf("unexpected list task request: %+v", call)
	}
}
