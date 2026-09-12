package plan

import (
	"context"
	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"testing"
)

type internalPlanCommandStub struct {
	planapp.PlanCommandService
	calls int
}

func (s *internalPlanCommandStub) CreatePlan(context.Context, planapp.CreatePlanDTO) (*planapp.PlanResult, error) {
	s.calls++
	return &planapp.PlanResult{}, nil
}

func TestRESTCommandsCannotFallBackToInternalService(t *testing.T) {
	internal := &internalPlanCommandStub{}
	module := &Module{CommandService: internal}
	if module.ExportRESTDeps(nil).CommandService != nil {
		t.Fatal("missing Operator commands fell back to internal authority")
	}
	module.OperatorCommandService = planapp.NewOperatorCommandService(internal, nil, nil)
	rest := module.ExportRESTDeps(nil).CommandService
	if _, err := rest.CreatePlan(context.Background(), planapp.CreatePlanDTO{OrgID: 1}); err == nil || internal.calls != 0 {
		t.Fatal("backstage request reached internal command without authorization")
	}
	grpc := module.ExportGRPCDeps().CommandService
	if grpc != internal {
		t.Fatal("internal service export changed")
	}
	if _, err := grpc.CreatePlan(context.Background(), planapp.CreatePlanDTO{OrgID: 1}); err != nil || internal.calls != 1 {
		t.Fatal("internal flow was incorrectly coupled to Operator context")
	}
}
