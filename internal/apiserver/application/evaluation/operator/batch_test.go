package operator

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type assessmentRepoStub struct {
	assessment.Repository
	items map[uint64]*assessment.Assessment
}

func (s *assessmentRepoStub) FindByID(_ context.Context, id assessment.ID) (*assessment.Assessment, error) {
	value := s.items[id.Uint64()]
	if value == nil {
		return nil, errors.New("not found")
	}
	return value, nil
}

type workerStub struct {
	calls []uint64
	fails map[uint64]error
}

func (s *workerStub) Evaluate(_ context.Context, assessmentID uint64) error {
	s.calls = append(s.calls, assessmentID)
	return s.fails[assessmentID]
}

func TestEvaluateBatchValidatesEntireOrganizationBeforeExecuting(t *testing.T) {
	repo := &assessmentRepoStub{items: map[uint64]*assessment.Assessment{
		1: newAssessment(t, 1, 1),
		2: newAssessment(t, 2, 2),
	}}
	worker := &workerStub{}

	if _, err := NewBatchExecutionService(repo, worker).EvaluateBatch(context.Background(), Actor{OrgID: 1}, []uint64{1, 2}); err == nil {
		t.Fatal("EvaluateBatch() error = nil, want organization mismatch")
	}
	if len(worker.calls) != 0 {
		t.Fatalf("worker calls = %v, want none before authorization completes", worker.calls)
	}
}

func TestEvaluateBatchPreservesSynchronousAggregateResult(t *testing.T) {
	repo := &assessmentRepoStub{items: map[uint64]*assessment.Assessment{
		1: newAssessment(t, 1, 1),
		2: newAssessment(t, 1, 2),
		3: newAssessment(t, 1, 3),
	}}
	worker := &workerStub{fails: map[uint64]error{2: errors.New("failed")}}

	result, err := NewBatchExecutionService(repo, worker).EvaluateBatch(context.Background(), Actor{OrgID: 1}, []uint64{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalCount != 3 || result.SuccessCount != 2 || result.FailedCount != 1 || !reflect.DeepEqual(result.FailedIDs, []uint64{2}) {
		t.Fatalf("result = %#v", result)
	}
	if !reflect.DeepEqual(worker.calls, []uint64{1, 2, 3}) {
		t.Fatalf("worker calls = %v", worker.calls)
	}
}

func newAssessment(t *testing.T, orgID int64, id uint64) *assessment.Assessment {
	t.Helper()
	value, err := assessment.NewAssessment(
		orgID,
		testee.NewID(100+id),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"),
		assessment.NewAnswerSheetRef(meta.FromUint64(200+id)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(id)),
	)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

var _ execute.WorkerExecutionService = (*workerStub)(nil)
