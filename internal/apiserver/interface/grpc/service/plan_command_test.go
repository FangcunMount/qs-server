package service

import (
	"context"
	"testing"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakePlanCommandService struct {
	createPlanFn           func(ctx context.Context, dto planApp.CreatePlanDTO) (*planApp.PlanResult, error)
	cancelPlanFn           func(ctx context.Context, orgID int64, planID string) (*planApp.PlanMutationResult, error)
	schedulePendingTasksFn func(ctx context.Context, orgID int64, before string) (*planApp.TaskScheduleResult, error)
}

func (f *fakePlanCommandService) CreatePlan(ctx context.Context, dto planApp.CreatePlanDTO) (*planApp.PlanResult, error) {
	return f.createPlanFn(ctx, dto)
}

func (f *fakePlanCommandService) PausePlan(context.Context, int64, string) (*planApp.PlanResult, error) {
	panic("unexpected call")
}

func (f *fakePlanCommandService) ResumePlan(context.Context, int64, string, map[string]string) (*planApp.PlanResult, error) {
	panic("unexpected call")
}

func (f *fakePlanCommandService) CancelPlan(ctx context.Context, orgID int64, planID string) (*planApp.PlanMutationResult, error) {
	return f.cancelPlanFn(ctx, orgID, planID)
}

func (f *fakePlanCommandService) EnrollTestee(context.Context, planApp.EnrollTesteeDTO) (*planApp.EnrollmentResult, error) {
	panic("unexpected call")
}

func (f *fakePlanCommandService) TerminateEnrollment(context.Context, int64, string, string) (*planApp.EnrollmentTerminationResult, error) {
	panic("unexpected call")
}

func (f *fakePlanCommandService) SchedulePendingTasks(ctx context.Context, orgID int64, before string) (*planApp.TaskScheduleResult, error) {
	return f.schedulePendingTasksFn(ctx, orgID, before)
}

func (f *fakePlanCommandService) OpenTask(context.Context, int64, string, planApp.OpenTaskDTO) (*planApp.TaskResult, error) {
	panic("unexpected call")
}

func (f *fakePlanCommandService) CompleteTask(context.Context, int64, string, string) (*planApp.TaskResult, error) {
	panic("unexpected call")
}

func (f *fakePlanCommandService) ExpireTask(context.Context, int64, string) (*planApp.TaskResult, error) {
	panic("unexpected call")
}

func (f *fakePlanCommandService) CancelTask(context.Context, int64, string) (*planApp.TaskMutationResult, error) {
	panic("unexpected call")
}

func TestPlanCommandServiceCreatePlanMapsRequestAndResponse(t *testing.T) {
	svc := NewPlanCommandService(&fakePlanCommandService{
		createPlanFn: func(ctx context.Context, dto planApp.CreatePlanDTO) (*planApp.PlanResult, error) {
			if dto.OrgID != 9 {
				t.Fatalf("unexpected org id: %d", dto.OrgID)
			}
			if dto.ScaleCode != "scale-code" || dto.ScheduleType != "custom" {
				t.Fatalf("unexpected dto: %#v", dto)
			}
			if len(dto.RelativeWeeks) != 2 || dto.RelativeWeeks[0] != 2 || dto.RelativeWeeks[1] != 4 {
				t.Fatalf("unexpected relative weeks: %#v", dto.RelativeWeeks)
			}
			return &planApp.PlanResult{
				ID:            "plan-1",
				OrgID:         dto.OrgID,
				ScaleCode:     dto.ScaleCode,
				ScheduleType:  dto.ScheduleType,
				RelativeWeeks: dto.RelativeWeeks,
				Status:        "active",
			}, nil
		},
		cancelPlanFn: func(context.Context, int64, string) (*planApp.PlanMutationResult, error) {
			panic("unexpected call")
		},
		schedulePendingTasksFn: func(context.Context, int64, string) (*planApp.TaskScheduleResult, error) {
			panic("unexpected call")
		},
	})

	resp, err := svc.CreatePlan(context.Background(), &pb.CreatePlanRequest{
		OrgId:         9,
		ScaleCode:     "scale-code",
		ScheduleType:  "custom",
		RelativeWeeks: []int32{2, 4},
	})
	if err != nil {
		t.Fatalf("CreatePlan returned error: %v", err)
	}
	if resp.GetPlan().GetId() != "plan-1" {
		t.Fatalf("unexpected plan id: %s", resp.GetPlan().GetId())
	}
	if got := resp.GetPlan().GetRelativeWeeks(); len(got) != 2 || got[0] != 2 || got[1] != 4 {
		t.Fatalf("unexpected relative weeks response: %#v", got)
	}
}

func TestPlanCommandServiceCancelPlanMapsPermissionDenied(t *testing.T) {
	svc := NewPlanCommandService(&fakePlanCommandService{
		createPlanFn: func(context.Context, planApp.CreatePlanDTO) (*planApp.PlanResult, error) {
			panic("unexpected call")
		},
		cancelPlanFn: func(ctx context.Context, orgID int64, planID string) (*planApp.PlanMutationResult, error) {
			return nil, pkgerrors.WithCode(errorCode.ErrPermissionDenied, "计划不属于当前机构")
		},
		schedulePendingTasksFn: func(context.Context, int64, string) (*planApp.TaskScheduleResult, error) {
			panic("unexpected call")
		},
	})

	_, err := svc.CancelPlan(context.Background(), &pb.CancelPlanRequest{
		OrgId:  1,
		PlanId: "plan-1",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected grpc status error")
	}
	if st.Code() != codes.PermissionDenied {
		t.Fatalf("unexpected grpc code: %s", st.Code())
	}
}
