package automation

import (
	"context"
	"testing"

	interpretationgeneration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type outcomeRepoStub struct {
	record *evaluationfact.Record
	reads  int
}

func (s *outcomeRepoStub) FindByID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	s.reads++
	return s.record, nil
}
func (s *outcomeRepoStub) FindByAssessmentID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	panic("automation must not resolve Outcome by Assessment")
}

type executorStub struct {
	input   interpinput.InterpretationInput
	traceID string
}

func (s *executorStub) Execute(_ context.Context, input interpinput.InterpretationInput, traceID string) (*interpretationgeneration.ExecuteResult, error) {
	s.input, s.traceID = input, traceID
	return &interpretationgeneration.ExecuteResult{Status: interpretationgeneration.ExecuteStatusProcessing}, nil
}

func TestGenerateRequiresTrustedActorBeforeReadingOutcome(t *testing.T) {
	repo := &outcomeRepoStub{}
	service, err := NewService(repo, &executorStub{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Generate(context.Background(), GenerateCommand{OutcomeID: meta.FromUint64(1)}); err == nil {
		t.Fatal("Generate error = nil, want actor error")
	}
	if repo.reads != 0 {
		t.Fatalf("outcome reads = %d, want 0", repo.reads)
	}
}
