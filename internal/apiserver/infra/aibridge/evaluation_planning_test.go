package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

func (s *evaluationRPCStub) Prepare(ctx context.Context, query *pb.EvaluationPlanQuery, _ ...grpc.CallOption) (*pb.EvaluationPlan, error) {
	s.calls++
	s.planQuery = query
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("bounded deadline required")
	}
	return s.planResponse, s.fail
}

func planFixture() app.EvaluationPlan {
	var release app.EvaluationRelease
	value := reflect.ValueOf(&release).Elem()
	for i := 0; i < value.NumField(); i++ {
		value.Field(i).Set(reflect.ValueOf(app.FrozenEvaluationRef{ID: value.Type().Field(i).Name, Version: "v1", Fingerprint: "sha256:" + strings.Repeat("a", 64)}))
	}
	execution := `{"policy_id":"ExecutionPolicy","version":"v1","slot_policy":{"required_generation_cases":7,"required_candidates_per_case":5,"required_preflight_cases":1},"generation_budget":{"max_executions_per_run":70},"semantic_budget":{"max_executions_per_run":70}}`
	gate := `{"policy_id":"GatePolicy","version":"v1"}`
	release.ExecutionPolicy.Fingerprint = digest([]byte(execution))
	release.GatePolicy.Fingerprint = digest([]byte(gate))
	raw, _ := json.Marshal(release)
	var refs map[string]app.FrozenEvaluationRef
	_ = json.Unmarshal(raw, &refs)
	return app.EvaluationPlan{Release: release, ReleaseFingerprint: releaseDigest(refs), GenerationCaseCount: 7, CandidatesPerCase: 5, CandidateCount: 35, PreflightCaseCount: 1, MaxGenerationInvocations: 70, MaxSemanticInvocations: 70, ExecutionPolicyJSON: execution, GatePolicyJSON: gate}
}

func TestPrepareVerifiesPlanAndForwardsProtectedScopeOnce(t *testing.T) {
	expected := planFixture()
	raw, _ := json.Marshal(expected)
	rpc := &evaluationRPCStub{t: t, planResponse: &pb.EvaluationPlan{SchemaVersion: "qs-ai-evaluation-plan/v1", PlanJson: string(raw)}}
	client := &EvaluationClient{RPC: rpc}
	scope := app.DraftScope{OrganizationID: 7, OperatorUserID: 42}
	query := app.EvaluationPlanQuery{Suite: expected.Release.Suite, GenerationRoute: expected.Release.GenerationRoute, SemanticRoute: expected.Release.SemanticRoute}
	actual, err := client.PrepareEvaluation(context.Background(), scope, query)
	if err != nil || !reflect.DeepEqual(expected, actual) || rpc.calls != 1 || rpc.planQuery.Scope.OrganizationId != 7 || rpc.planQuery.Scope.OperatorUserId != 42 || rpc.planQuery.SemanticRoute.Fingerprint != query.SemanticRoute.Fingerprint {
		t.Fatal(actual, err)
	}
	rpc.fail = context.DeadlineExceeded
	if _, err := client.PrepareEvaluation(context.Background(), scope, query); !errors.Is(err, context.DeadlineExceeded) || rpc.calls != 2 {
		t.Fatal("unexpected retry", err, rpc.calls)
	}
	if _, err := client.PrepareEvaluation(context.Background(), app.DraftScope{}, query); !errors.Is(err, app.ErrInvalid) || rpc.calls != 2 {
		t.Fatal(err)
	}
}

func TestPrepareRejectsMismatchedReferencesBudgetsAndPolicyDocuments(t *testing.T) {
	base := planFixture()
	query := app.EvaluationPlanQuery{Suite: base.Release.Suite, GenerationRoute: base.Release.GenerationRoute, SemanticRoute: base.Release.SemanticRoute}
	for name, mutate := range map[string]func(*app.EvaluationPlan){
		"suite":             func(p *app.EvaluationPlan) { p.Release.Suite.Version = "wrong" },
		"generation":        func(p *app.EvaluationPlan) { p.Release.GenerationRoute.Version = "wrong" },
		"semantic":          func(p *app.EvaluationPlan) { p.Release.SemanticRoute.Version = "wrong" },
		"missing component": func(p *app.EvaluationPlan) { p.Release.SemanticOutputSchema = app.FrozenEvaluationRef{} },
		"release digest":    func(p *app.EvaluationPlan) { p.ReleaseFingerprint = "sha256:" + strings.Repeat("0", 64) },
		"execution bytes":   func(p *app.EvaluationPlan) { p.ExecutionPolicyJSON += " " },
		"gate bytes":        func(p *app.EvaluationPlan) { p.GatePolicyJSON += " " },
		"budget":            func(p *app.EvaluationPlan) { p.MaxGenerationInvocations = 71 },
		"missing count":     func(p *app.EvaluationPlan) { p.CandidateCount = 0 },
		"case count":        func(p *app.EvaluationPlan) { p.GenerationCaseCount = 6 },
		"product":           func(p *app.EvaluationPlan) { p.CandidateCount = 36 },
		"unsafe integer":    func(p *app.EvaluationPlan) { p.MaxSemanticInvocations = 9007199254740992 },
	} {
		t.Run(name, func(t *testing.T) {
			value := base
			mutate(&value)
			raw, _ := json.Marshal(value)
			_, err := evaluationPlan(&pb.EvaluationPlan{SchemaVersion: "qs-ai-evaluation-plan/v1", PlanJson: string(raw)}, query)
			if !errors.Is(err, app.ErrConflict) {
				t.Fatal(err)
			}
		})
	}
	raw, _ := json.Marshal(base)
	for _, response := range []*pb.EvaluationPlan{nil, {SchemaVersion: "wrong", PlanJson: string(raw)}, {SchemaVersion: "qs-ai-evaluation-plan/v1", PlanJson: string(raw) + "{}"}, {SchemaVersion: "qs-ai-evaluation-plan/v1", PlanJson: strings.Repeat(" ", 32768) + string(raw)}, {SchemaVersion: "qs-ai-evaluation-plan/v1", PlanJson: "\xff"}} {
		if _, err := evaluationPlan(response, query); !errors.Is(err, app.ErrConflict) {
			t.Fatal(err)
		}
	}
}
