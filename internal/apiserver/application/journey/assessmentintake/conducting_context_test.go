package assessmentintake

import (
	"context"
	"errors"
	evaluationintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	answersheetapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	sheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"testing"
	"time"
)

type persistedSheet struct {
	value *sheet.AnswerSheet
	err   error
}

func (r persistedSheet) FindByID(context.Context, meta.ID) (*sheet.AnswerSheet, error) {
	return r.value, r.err
}
func TestConductingContextComesFromPersistedSubmission(t *testing.T) {
	c, _ := sheet.NewSubmissionContext(actor.NewFillerRef(2, actor.FillerTypeSelf), actor.NewTesteeRef(meta.FromUint64(3)), meta.FromUint64(1), "")
	store := uint64(10)
	start, _ := sheet.NewStartContext(99, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), &store, 2)
	c = c.WithStartContext(start)
	q, _ := sheet.NewQuestionnaireRef("Q", "1", "Q")
	value := sheet.ReconstructWithSubmissionContext(meta.FromUint64(9), q, c, nil, time.Now(), 0)
	s := &service{submissions: persistedSheet{value: value}}
	command := Command{OrgID: 1, FillerID: 2, TesteeID: 3, AnswerSheetID: 9, QuestionnaireCode: "Q", QuestionnaireVersion: "1"}
	captured, err := s.loadConductingContext(context.Background(), command)
	if err != nil || captured.ID() != 99 || *captured.StoreID() != 10 || captured.OwnershipVersion() != 2 {
		t.Fatalf("context: %+v %v", captured, err)
	}
	command.TesteeID = 7
	if _, err := s.loadConductingContext(context.Background(), command); err == nil {
		t.Fatal("mismatched event accepted")
	}
	s.submissions = persistedSheet{err: errors.New("corrupt data")}
	if _, err := s.loadConductingContext(context.Background(), command); err == nil {
		t.Fatal("storage corruption downgraded")
	}
	s.submissions = persistedSheet{}
	if _, err := s.loadConductingContext(context.Background(), command); err == nil {
		t.Fatal("missing sheet downgraded")
	}
}

// Intake cases provide an explicit persisted submission and frozen command.
func newIntakeService(scoring answersheetapp.AnswerSheetScoringService, plans planapp.TaskAssessmentResolver, commands planapp.PlanCommandService, intake evaluationintake.Service) Service {
	c, _ := sheet.NewSubmissionContext(actor.NewFillerRef(8, actor.FillerTypeSelf), actor.NewTesteeRef(meta.FromUint64(7)), meta.FromUint64(9), "")
	q, _ := sheet.NewQuestionnaireRef("Q", "1", "Q")
	persisted := sheet.ReconstructWithSubmissionContext(meta.FromUint64(3), q, c, nil, time.Now(), 0)
	return NewService(scoring, plans, commands, intake, nil, persistedSheet{value: persisted})
}
func TestMissingSubmissionReaderCannotDowngradeToLegacy(t *testing.T) {
	_, err := (&service{}).loadConductingContext(context.Background(), Command{})
	if err == nil {
		t.Fatal("missing reader silently treated as legacy")
	}
}
