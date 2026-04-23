package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
	"github.com/gin-gonic/gin"
)

type stubPlanCommandService struct {
	lastCreatePlanDTO planApp.CreatePlanDTO
	createPlanResult  *planApp.PlanResult
	createPlanErr     error
	createPlanCalls   int
}

func (s *stubPlanCommandService) CreatePlan(_ context.Context, dto planApp.CreatePlanDTO) (*planApp.PlanResult, error) {
	s.createPlanCalls++
	s.lastCreatePlanDTO = dto
	return s.createPlanResult, s.createPlanErr
}
func (*stubPlanCommandService) PausePlan(context.Context, int64, string) (*planApp.PlanResult, error) {
	return nil, nil
}
func (*stubPlanCommandService) ResumePlan(context.Context, int64, string, map[string]string) (*planApp.PlanResult, error) {
	return nil, nil
}
func (*stubPlanCommandService) FinishPlan(context.Context, int64, string) (*planApp.PlanResult, error) {
	return nil, nil
}
func (*stubPlanCommandService) CancelPlan(context.Context, int64, string) (*planApp.PlanMutationResult, error) {
	return nil, nil
}
func (*stubPlanCommandService) EnrollTestee(context.Context, planApp.EnrollTesteeDTO) (*planApp.EnrollmentResult, error) {
	return nil, nil
}
func (*stubPlanCommandService) TerminateEnrollment(context.Context, int64, string, string) (*planApp.EnrollmentTerminationResult, error) {
	return nil, nil
}
func (*stubPlanCommandService) SchedulePendingTasks(context.Context, int64, string) (*planApp.TaskScheduleResult, error) {
	return nil, nil
}
func (*stubPlanCommandService) OpenTask(context.Context, int64, string, planApp.OpenTaskDTO) (*planApp.TaskResult, error) {
	return nil, nil
}
func (*stubPlanCommandService) CompleteTask(context.Context, int64, string, string) (*planApp.TaskResult, error) {
	return nil, nil
}
func (*stubPlanCommandService) ExpireTask(context.Context, int64, string) (*planApp.TaskResult, error) {
	return nil, nil
}
func (*stubPlanCommandService) CancelTask(context.Context, int64, string) (*planApp.TaskMutationResult, error) {
	return nil, nil
}

type stubPlanQueryService struct{}

func (stubPlanQueryService) GetPlan(context.Context, int64, string) (*planApp.PlanResult, error) {
	return nil, nil
}
func (stubPlanQueryService) ListPlans(context.Context, planApp.ListPlansDTO) (*planApp.PlanListResult, error) {
	return nil, nil
}
func (stubPlanQueryService) GetTask(context.Context, int64, string) (*planApp.TaskResult, error) {
	return nil, nil
}
func (stubPlanQueryService) ListTasks(context.Context, planApp.ListTasksDTO) (*planApp.TaskListResult, error) {
	return nil, nil
}
func (stubPlanQueryService) ListTaskWindow(context.Context, planApp.ListTaskWindowDTO) (*planApp.TaskWindowResult, error) {
	return nil, nil
}
func (stubPlanQueryService) ListTasksByPlan(context.Context, int64, string) ([]*planApp.TaskResult, error) {
	return nil, nil
}
func (stubPlanQueryService) ListTasksByPlanInScope(context.Context, int64, string, []string) ([]*planApp.TaskResult, error) {
	return nil, nil
}
func (stubPlanQueryService) ListTasksByTestee(context.Context, string) ([]*planApp.TaskResult, error) {
	return nil, nil
}
func (stubPlanQueryService) ListPlansByTestee(context.Context, string) ([]*planApp.PlanResult, error) {
	return nil, nil
}
func (stubPlanQueryService) ListTasksByTesteeAndPlan(context.Context, string, string) ([]*planApp.TaskResult, error) {
	return nil, nil
}

type planTesteeAccessService struct{}

func (*planTesteeAccessService) ResolveAccessScope(context.Context, int64, int64) (*actorAccessApp.TesteeAccessScope, error) {
	return &actorAccessApp.TesteeAccessScope{IsAdmin: true}, nil
}
func (*planTesteeAccessService) ValidateTesteeAccess(context.Context, int64, int64, uint64) error {
	return nil
}
func (*planTesteeAccessService) ListAccessibleTesteeIDs(context.Context, int64, int64) ([]uint64, error) {
	return nil, nil
}

func newPlanHandlerForTest(command planApp.PlanCommandService) *PlanHandler {
	handler := NewPlanHandler(command, stubPlanQueryService{})
	handler.BaseHandler = *NewBaseHandler()
	handler.SetTesteeAccessService(&planTesteeAccessService{})
	return handler
}

func newPlanTestContext(method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	return c, rec
}

func TestPlanHandlerCreatePlanSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	command := &stubPlanCommandService{
		createPlanResult: &planApp.PlanResult{
			ID:           "plan-1",
			OrgID:        88,
			ScaleCode:    "SCL-1",
			ScheduleType: "by_week",
			TriggerTime:  "19:00:00",
			Interval:     2,
			TotalTimes:   6,
			Status:       "active",
		},
	}
	handler := newPlanHandlerForTest(command)
	c, rec := newPlanTestContext(http.MethodPost, "/api/v1/plans", []byte(`{"scale_code":"SCL-1","schedule_type":"by_week","trigger_time":"19:00:00","interval":2,"total_times":6}`))
	c.Set(restmiddleware.OrgIDKey, uint64(88))

	handler.CreatePlan(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if command.createPlanCalls != 1 {
		t.Fatalf("CreatePlan calls = %d, want 1", command.createPlanCalls)
	}
	if command.lastCreatePlanDTO.OrgID != 88 || command.lastCreatePlanDTO.ScaleCode != "SCL-1" {
		t.Fatalf("unexpected dto: %+v", command.lastCreatePlanDTO)
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			ID        string `json:"id"`
			OrgID     int64  `json:"org_id"`
			ScaleCode string `json:"scale_code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Code != 0 || payload.Data.ID != "plan-1" || payload.Data.OrgID != 88 || payload.Data.ScaleCode != "SCL-1" {
		t.Fatalf("unexpected response payload: %+v", payload)
	}
}

func TestPlanHandlerCreatePlanRejectsMissingProtectedOrgScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	command := &stubPlanCommandService{}
	handler := newPlanHandlerForTest(command)
	c, rec := newPlanTestContext(http.MethodPost, "/api/v1/plans", []byte(`{"scale_code":"SCL-1","schedule_type":"by_week"}`))

	handler.CreatePlan(c)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if command.createPlanCalls != 0 {
		t.Fatalf("CreatePlan calls = %d, want 0", command.createPlanCalls)
	}
}

func TestPlanHandlerCreatePlanRejectsInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	command := &stubPlanCommandService{}
	handler := newPlanHandlerForTest(command)
	c, rec := newPlanTestContext(http.MethodPost, "/api/v1/plans", []byte(`{"scale_code":`))
	c.Set(restmiddleware.OrgIDKey, uint64(88))

	handler.CreatePlan(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if command.createPlanCalls != 0 {
		t.Fatalf("CreatePlan calls = %d, want 0", command.createPlanCalls)
	}
}

var _ actorAccessApp.TesteeAccessService = (*planTesteeAccessService)(nil)
