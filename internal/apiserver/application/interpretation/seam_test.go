package interpretation_test

import (
	"context"
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type seamWriter struct {
	calls int
}

func (s *seamWriter) Write(context.Context, evaloutcome.Outcome) error {
	s.calls++
	return nil
}

func TestServiceDelegatesToReportingWriter(t *testing.T) {
	writer := &seamWriter{}
	svc := interpretation.NewService(writer)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(1),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-1"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(1)),
		assessment.NewAdhocOrigin(),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	outcome := evaloutcome.Outcome{
		Assessment: a,
		Execution:  &assessment.AssessmentOutcome{},
	}
	if err := svc.GenerateAndPersist(context.Background(), outcome); err != nil {
		t.Fatalf("GenerateAndPersist() error = %v", err)
	}
	if writer.calls != 1 {
		t.Fatalf("writer calls = %d, want 1", writer.calls)
	}
}

func TestReportingWriterRequiresConfiguredDependencies(t *testing.T) {
	writer, err := interpretationreporting.NewWriter(nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewWriter() error = %v", err)
	}
	if writer == nil {
		t.Fatal("NewWriter() returned nil writer")
	}
}
