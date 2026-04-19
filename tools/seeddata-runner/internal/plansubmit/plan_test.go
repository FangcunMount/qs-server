package plansubmit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seedconfig"
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

func TestNormalizePlanWorkers(t *testing.T) {
	tests := []struct {
		name      string
		workers   int
		testeeCnt int
		expected  int
	}{
		{name: "default to one", workers: 0, testeeCnt: 10, expected: 1},
		{name: "cap by task count", workers: 8, testeeCnt: 3, expected: 3},
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

func TestOptionsFromConfigUsesPlanIDs(t *testing.T) {
	cfg := &seedconfig.Config{
		PlanSubmit: seedconfig.PlanSubmitConfig{
			PlanIDs: []seedconfig.FlexibleID{"614333603412718126", "614187067651404334", "614333603412718126"},
			Workers: 2,
		},
	}

	opts := optionsFromConfig(cfg)
	if len(opts.PlanIDs) != 2 {
		t.Fatalf("expected 2 normalized plan ids, got %#v", opts.PlanIDs)
	}
	if opts.PlanIDs[0] != "614333603412718126" || opts.PlanIDs[1] != "614187067651404334" {
		t.Fatalf("unexpected plan ids: %#v", opts.PlanIDs)
	}
	if opts.Workers != 2 {
		t.Fatalf("unexpected workers: %d", opts.Workers)
	}
}

func TestBuildPlanTaskSubmitRequestIncludesTaskID(t *testing.T) {
	detail := &QuestionnaireDetailResponse{
		Code:    "QNR-001",
		Title:   "Test Questionnaire",
		Version: "1.0.0",
		Questions: []QuestionResponse{
			{
				Code:  "Q1",
				Type:  questionTypeRadio,
				Title: "Question 1",
				Options: []OptionResponse{
					{Code: "A", Content: "A", Score: 1},
					{Code: "B", Content: "B", Score: 2},
				},
			},
		},
	}

	req, err := buildPlanTaskSubmitRequest(
		detail,
		"1.0.0",
		TaskResponse{
			ID:       "2001",
			TesteeID: "1001",
			Status:   "opened",
		},
		false,
		newSeeddataLogger(false),
	)
	if err != nil {
		t.Fatalf("buildPlanTaskSubmitRequest returned error: %v", err)
	}
	if req == nil {
		t.Fatal("expected non-nil request")
	}
	if req.TaskID != "2001" {
		t.Fatalf("expected task_id=2001, got %q", req.TaskID)
	}
	if req.TesteeID != 1001 {
		t.Fatalf("expected testee_id=1001, got %d", req.TesteeID)
	}
}

func TestOpenPlanTaskSubmitSessionDoesNotRequireLocalRuntimeConfig(t *testing.T) {
	const planID = "614333603412718126"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/testees":
			_ = json.NewEncoder(w).Encode(Response{Code: 0, Message: "ok", Data: map[string]any{
				"items":     []any{},
				"page":      1,
				"page_size": 1,
			}})
		case "/api/v1/plans/" + planID:
			_ = json.NewEncoder(w).Encode(Response{Code: 0, Message: "ok", Data: map[string]any{
				"id":         planID,
				"org_id":     1,
				"scale_code": "SAS-TEST",
				"status":     "active",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	logger := newSeeddataLogger(false)
	deps := &dependencies{
		Logger: logger,
		Config: &SeedConfig{
			Global: GlobalConfig{OrgID: 1},
		},
		APIClient: NewAPIClient(server.URL, "test-token", logger),
	}

	session, err := openPlanTaskSubmitSession(context.Background(), deps, planID, false)
	if err != nil {
		t.Fatalf("openPlanTaskSubmitSession returned error: %v", err)
	}
	if session == nil || session.plan == nil {
		t.Fatalf("expected non-nil submit session and plan")
	}
	if session.plan.ID != planID || session.plan.ScaleCode != "SAS-TEST" {
		t.Fatalf("unexpected loaded plan: %#v", session.plan)
	}
}

func TestLoadPlanTaskSubmitQuestionnaireUsesAPIGateway(t *testing.T) {
	const planID = "614333603412718126"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/testees":
			_ = json.NewEncoder(w).Encode(Response{Code: 0, Message: "ok", Data: map[string]any{
				"items":     []any{},
				"page":      1,
				"page_size": 1,
			}})
		case "/api/v1/plans/" + planID:
			_ = json.NewEncoder(w).Encode(Response{Code: 0, Message: "ok", Data: map[string]any{
				"id":         planID,
				"org_id":     1,
				"scale_code": "SAS-TEST",
				"status":     "active",
			}})
		case "/api/v1/scales/SAS-TEST":
			_ = json.NewEncoder(w).Encode(Response{Code: 0, Message: "ok", Data: map[string]any{
				"code":                  "SAS-TEST",
				"questionnaire_code":    "QNR-TEST",
				"questionnaire_version": "1.0.1",
			}})
		case "/api/v1/questionnaires/QNR-TEST":
			_ = json.NewEncoder(w).Encode(Response{Code: 0, Message: "ok", Data: map[string]any{
				"code":    "QNR-TEST",
				"title":   "Questionnaire",
				"version": "1.0.1",
				"questions": []any{
					map[string]any{"code": "q1", "type": "radio", "title": "Q1", "options": []any{
						map[string]any{"code": "a", "content": "A", "score": 1},
					}},
				},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	logger := newSeeddataLogger(false)
	deps := &dependencies{
		Logger: logger,
		Config: &SeedConfig{
			Global: GlobalConfig{OrgID: 1},
		},
		APIClient: NewAPIClient(server.URL, "test-token", logger),
	}

	session, err := openPlanTaskSubmitSession(context.Background(), deps, planID, false)
	if err != nil {
		t.Fatalf("openPlanTaskSubmitSession returned error: %v", err)
	}

	scaleResp, detail, err := loadPlanTaskSubmitQuestionnaire(context.Background(), session, false)
	if err != nil {
		t.Fatalf("loadPlanTaskSubmitQuestionnaire returned error: %v", err)
	}
	if scaleResp == nil || scaleResp.QuestionnaireCode != "QNR-TEST" || scaleResp.QuestionnaireVersion != "1.0.1" {
		t.Fatalf("unexpected scale response: %#v", scaleResp)
	}
	if detail == nil || detail.Code != "QNR-TEST" || detail.Version != "1.0.1" {
		t.Fatalf("unexpected questionnaire detail: %#v", detail)
	}
}

func TestRecentPlanTaskTrackerHonorsTTL(t *testing.T) {
	tracker := newRecentPlanTaskTracker(2 * time.Minute)
	if tracker.Seen("2001") {
		t.Fatal("expected task to be unseen initially")
	}

	tracker.Remember("2001")
	if !tracker.Seen("2001") {
		t.Fatal("expected task to be tracked after remember")
	}

	tracker.mu.Lock()
	tracker.submittedAt["2001"] = time.Now().Add(-1 * time.Second)
	tracker.mu.Unlock()
	if tracker.Seen("2001") {
		t.Fatal("expected expired task tracker entry to be pruned")
	}
}

type pagedPlanTaskSubmitGatewayStub struct {
	taskLists           map[string][]TaskResponse
	listTaskWindowCalls []ListPlanTaskWindowRequest
}

func (s *pagedPlanTaskSubmitGatewayStub) GetPlan(ctx context.Context, planID string) (*PlanResponse, error) {
	return nil, nil
}

func (s *pagedPlanTaskSubmitGatewayStub) GetScale(ctx context.Context, code string) (*ScaleResponse, error) {
	return nil, nil
}

func (s *pagedPlanTaskSubmitGatewayStub) GetQuestionnaireDetail(ctx context.Context, code string) (*QuestionnaireDetailResponse, error) {
	return nil, nil
}

func (s *pagedPlanTaskSubmitGatewayStub) ListPlanTaskWindow(ctx context.Context, req ListPlanTaskWindowRequest) (*PlanTaskWindowResponse, error) {
	s.listTaskWindowCalls = append(s.listTaskWindowCalls, req)
	return buildStubTaskWindowResponse(s.taskLists, req), nil
}

func buildStubTaskWindowResponse(taskLists map[string][]TaskResponse, req ListPlanTaskWindowRequest) *PlanTaskWindowResponse {
	if taskLists == nil {
		return &PlanTaskWindowResponse{
			Page:     max(req.Page, 1),
			PageSize: max(req.PageSize, 1),
		}
	}

	allTasks := make([]TaskResponse, 0)
	for testeeID, items := range taskLists {
		for _, task := range items {
			if strings.TrimSpace(task.TesteeID) == "" {
				task.TesteeID = testeeID
			}
			if planID := strings.TrimSpace(req.PlanID); planID != "" && strings.TrimSpace(task.PlanID) != "" && strings.TrimSpace(task.PlanID) != planID {
				continue
			}
			if status := normalizeTaskStatus(req.Status); status != "" && normalizeTaskStatus(task.Status) != status {
				continue
			}
			allTasks = append(allTasks, task)
		}
	}

	sort.SliceStable(allTasks, func(i, j int) bool {
		if allTasks[i].TesteeID != allTasks[j].TesteeID {
			return allTasks[i].TesteeID < allTasks[j].TesteeID
		}
		if allTasks[i].Seq != allTasks[j].Seq {
			return allTasks[i].Seq < allTasks[j].Seq
		}
		return allTasks[i].ID < allTasks[j].ID
	})

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = len(allTasks)
		if pageSize == 0 {
			pageSize = 1
		}
	}
	start := (page - 1) * pageSize
	if start > len(allTasks) {
		start = len(allTasks)
	}
	end := start + pageSize
	if end > len(allTasks) {
		end = len(allTasks)
	}

	return &PlanTaskWindowResponse{
		Tasks:    append([]TaskResponse(nil), allTasks[start:end]...),
		Page:     page,
		PageSize: pageSize,
		HasMore:  end < len(allTasks),
	}
}

func TestListOpenPlanTaskJobsUsesPagedOpenedTasks(t *testing.T) {
	planID := "614333603412718126"
	testeeTasks := make([]TaskResponse, 0, planOpenTaskPageSize+1)
	for idx := 0; idx < planOpenTaskPageSize+1; idx++ {
		testeeTasks = append(testeeTasks, TaskResponse{
			ID:       "task-" + strconv.Itoa(idx+1),
			PlanID:   planID,
			TesteeID: "1001",
			Seq:      idx + 1,
			Status:   "opened",
		})
	}
	testeeTasks[0].ID = "2001"
	testeeTasks[len(testeeTasks)-1].ID = "2201"
	gateway := &pagedPlanTaskSubmitGatewayStub{
		taskLists: map[string][]TaskResponse{
			"1001": testeeTasks,
			"1002": {
				{ID: "2301", PlanID: planID, TesteeID: "1002", Seq: 1, Status: "completed"},
			},
		},
	}

	jobs, err := listOpenPlanTaskJobs(context.Background(), gateway, newSeeddataLogger(false), planID, false)
	if err != nil {
		t.Fatalf("unexpected collect error: %v", err)
	}
	if len(gateway.listTaskWindowCalls) != 2 {
		t.Fatalf("expected 2 paged opened-task calls, got %d", len(gateway.listTaskWindowCalls))
	}
	if len(jobs) != planOpenTaskPageSize+1 {
		t.Fatalf("expected %d opened jobs, got %d", planOpenTaskPageSize+1, len(jobs))
	}
	if jobs[0].task.ID != "2001" || jobs[len(jobs)-1].task.ID != "2201" {
		t.Fatalf("unexpected opened jobs boundary ids: first=%s last=%s", jobs[0].task.ID, jobs[len(jobs)-1].task.ID)
	}
}

func TestRunPlanSubmitOpenTasksCycleSubmitsOnlyFreshOpenedTasks(t *testing.T) {
	planID := "614333603412718126"
	gateway := &pagedPlanTaskSubmitGatewayStub{
		taskLists: map[string][]TaskResponse{
			"1001": {
				{ID: "2001", PlanID: planID, TesteeID: "1001", Seq: 1, Status: "opened"},
			},
			"1002": {
				{ID: "2002", PlanID: planID, TesteeID: "1002", Seq: 1, Status: "opened"},
			},
		},
	}
	submitClient := &adminAnswerSheetSubmitClientStub{}
	tracker := newRecentPlanTaskTracker(5 * time.Minute)
	tracker.Remember("2002")

	detail := &QuestionnaireDetailResponse{
		Code:    "QNR-001",
		Title:   "Test Questionnaire",
		Version: "1.0.0",
		Questions: []QuestionResponse{
			{
				Code:  "Q1",
				Type:  questionTypeRadio,
				Title: "Question 1",
				Options: []OptionResponse{
					{Code: "A", Content: "A", Score: 1},
				},
			},
		},
	}

	stats, err := runPlanSubmitOpenTasksCycle(
		context.Background(),
		gateway,
		submitClient,
		newSeeddataLogger(false),
		planID,
		"1.0.0",
		detail,
		2,
		tracker,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected cycle error: %v", err)
	}
	if stats.OpenedCount != 2 {
		t.Fatalf("expected opened_count=2, got %+v", stats)
	}
	if stats.SubmittedCount != 1 {
		t.Fatalf("expected submitted_count=1, got %+v", stats)
	}
	if stats.SkippedCount != 1 {
		t.Fatalf("expected skipped_count=1, got %+v", stats)
	}
	if stats.FailedTaskExecutions != 0 {
		t.Fatalf("expected no failed task executions, got %+v", stats)
	}
	if submitClient.withPolicyCalls != 1 {
		t.Fatalf("expected one submit call, got %d", submitClient.withPolicyCalls)
	}
	if submitClient.lastPolicyReq.TaskID != "2001" {
		t.Fatalf("expected task_id=2001 to be submitted, got %+v", submitClient.lastPolicyReq)
	}
}
