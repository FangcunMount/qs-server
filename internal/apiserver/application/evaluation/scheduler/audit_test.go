package scheduler

import (
	"context"
	"reflect"
	"testing"

	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

type candidateReaderStub struct {
	ids   []uint64
	after []uint64
}

func (r *candidateReaderStub) ListSubmittedAssessmentIDsAfter(_ context.Context, after uint64, limit int) ([]uint64, error) {
	r.after = append(r.after, after)
	result := make([]uint64, 0, limit)
	for _, id := range r.ids {
		if id > after && len(result) < limit {
			result = append(result, id)
		}
	}
	return result, nil
}

type assessmentRepoStub struct{}

func (assessmentRepoStub) Save(context.Context, *domainassessment.Assessment) error { return nil }
func (assessmentRepoStub) FindByID(context.Context, domainassessment.ID) (*domainassessment.Assessment, error) {
	return nil, nil
}
func (assessmentRepoStub) Delete(context.Context, domainassessment.ID) error { return nil }
func (assessmentRepoStub) FindByAnswerSheetID(context.Context, domainassessment.AnswerSheetRef) (*domainassessment.Assessment, error) {
	return nil, nil
}

type outcomeRepoStub struct{}

func (outcomeRepoStub) Save(context.Context, *domainoutcome.Record) error { return nil }
func (outcomeRepoStub) FindByID(context.Context, domainoutcome.ID) (*domainoutcome.Record, error) {
	return nil, nil
}
func (outcomeRepoStub) FindByAssessmentID(context.Context, domainoutcome.ID) (*domainoutcome.Record, error) {
	return nil, nil
}

func TestAuditOnceTraversesAllBatchesAndRestartsAfterEnd(t *testing.T) {
	reader := &candidateReaderStub{ids: []uint64{1, 2, 3, 4, 5}}
	service := NewService(assessmentRepoStub{}, outcomeRepoStub{}, reader)
	for range 4 {
		if _, err := service.AuditOnce(context.Background(), 2); err != nil {
			t.Fatal(err)
		}
	}
	if want := []uint64{0, 2, 4, 5}; !reflect.DeepEqual(reader.after, want) {
		t.Fatalf("keyset cursors = %v, want %v", reader.after, want)
	}
	if _, err := service.AuditOnce(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	if reader.after[len(reader.after)-1] != 0 {
		t.Fatalf("scan did not restart: %v", reader.after)
	}
}

func TestAuditOnceRejectsMissingDependencies(t *testing.T) {
	reader := &candidateReaderStub{}
	for _, service := range []Service{
		NewService(nil, outcomeRepoStub{}, reader),
		NewService(assessmentRepoStub{}, nil, reader),
		NewService(assessmentRepoStub{}, outcomeRepoStub{}, nil),
	} {
		if _, err := service.AuditOnce(context.Background(), 10); err == nil {
			t.Fatal("expected module configuration error")
		}
	}
}
